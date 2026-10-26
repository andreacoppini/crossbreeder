package main

import (
	"context"
	"strings"
	"testing"
)

// resolverWith builds an r1Resolver whose client points at the fake, so the
// prefetch logic can be exercised without the region→host mapping.
func resolverWith(t *testing.T, f *fakeR1, user string) *r1Resolver {
	t.Helper()
	c := f.client(t)
	aps, err := c.listAPs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byIP := map[string][]r1AP{}
	for _, a := range aps {
		if a.IP != "" {
			byIP[a.IP] = append(byIP[a.IP], a)
		}
	}
	return &r1Resolver{client: c, byIP: byIP, user: user, conc: 8}
}

// A duplicate private IP across two venues must yield both passwords, so the
// engine can try each — the case the integration was asked to handle.
func TestR1ResolverTriesAllForDuplicateIP(t *testing.T) {
	f := newFakeR1(t)
	f.aps = []r1AP{
		{Serial: "SN-A", VenueID: "vA", IP: "192.168.1.50"},
		{Serial: "SN-B", VenueID: "vB", IP: "192.168.1.50"}, // same IP, other venue
		{Serial: "SN-C", VenueID: "vC", IP: "192.168.1.99"},
	}
	f.passwords = map[string]string{"SN-A": "pw-a", "SN-B": "pw-b", "SN-C": "pw-c"}

	r := resolverWith(t, f, "admin")
	pull := r.credentialsFor(context.Background(), []string{"192.168.1.50"})
	creds, matched, pulled := pull.Creds, pull.Matched, pull.Pulled

	if matched != 1 {
		t.Errorf("matched = %d, want 1 address", matched)
	}
	if pulled != 2 {
		t.Errorf("pulled = %d, want 2 passwords for the duplicate", pulled)
	}
	got := creds["192.168.1.50"]
	if len(got) != 2 {
		t.Fatalf("got %d credentials for the duplicate IP, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, c := range got {
		if c.User != "admin" {
			t.Errorf("credential user = %q, want admin", c.User)
		}
		seen[c.Password] = true
	}
	if !seen["pw-a"] || !seen["pw-b"] {
		t.Errorf("both venue passwords should be present, got %v", seen)
	}
}

// Only the addresses that were reached get a password pull, and an AP whose
// password this account cannot read is simply absent rather than fatal.
func TestR1ResolverSkipsUnreachableAndDenied(t *testing.T) {
	f := newFakeR1(t)
	f.aps = []r1AP{
		{Serial: "SN-1", VenueID: "v", IP: "10.0.0.1"},
		{Serial: "SN-2", VenueID: "v", IP: "10.0.0.2"},
		{Serial: "SN-3", VenueID: "v", IP: "10.0.0.3"},
	}
	f.passwords = map[string]string{"SN-1": "p1", "SN-2": "p2", "SN-3": "p3"}
	f.denySN = map[string]bool{"SN-2": true} // account cannot read SN-2

	r := resolverWith(t, f, "admin")
	// Only .1 and .2 were reachable; .3 is not in the alive list.
	pull := r.credentialsFor(context.Background(), []string{"10.0.0.1", "10.0.0.2"})
	creds, matched, pulled := pull.Creds, pull.Matched, pull.Pulled

	if matched != 2 {
		t.Errorf("matched = %d, want 2", matched)
	}
	if pulled != 1 {
		t.Errorf("pulled = %d, want 1 (SN-2 denied, SN-3 not reached)", pulled)
	}
	if len(creds["10.0.0.1"]) != 1 || creds["10.0.0.1"][0].Password != "p1" {
		t.Errorf("10.0.0.1 should carry p1, got %v", creds["10.0.0.1"])
	}
	if _, ok := creds["10.0.0.2"]; ok {
		t.Error("a denied AP must not appear in the credentials")
	}
	if f.pwCalls.Load() != 2 {
		t.Errorf("password endpoint hit %d times, want 2 (only the reached APs)", f.pwCalls.Load())
	}
}

// The read-only-key case, as seen on a live tenant: R1 answers 200 with the
// password blanked to asterisks. It must be counted as masked and never become
// a credential — sending "**********" fails every login.
func TestR1MaskedPasswordIsNeverSent(t *testing.T) {
	f := newFakeR1(t)
	f.aps = []r1AP{
		{Serial: "SN-1", VenueID: "v", IP: "10.4.28.71"},
		{Serial: "SN-2", VenueID: "v", IP: "10.4.28.68"},
	}
	f.passwords = map[string]string{"SN-1": "**********", "SN-2": "**********"}

	r := resolverWith(t, f, "admin")
	pull := r.credentialsFor(context.Background(), []string{"10.4.28.71", "10.4.28.68"})

	if pull.Masked != 2 || pull.Pulled != 0 {
		t.Fatalf("masked = %d, pulled = %d; want 2 masked, 0 pulled", pull.Masked, pull.Pulled)
	}
	if len(pull.Creds) != 0 {
		t.Fatalf("a masked password became a credential: %v", pull.Creds)
	}

	lines := r1PullSummary(pull, 2)
	if len(lines) != 2 || !strings.Contains(lines[1], "every password masked") ||
		!strings.Contains(lines[1], "read-only") {
		t.Errorf("all-masked run should warn about the API key; got %q", lines)
	}
}

// A mix: real passwords are used, masked ones skipped, and the warning names
// how many were dropped rather than claiming the key is useless.
func TestR1MixedMaskedAndReal(t *testing.T) {
	f := newFakeR1(t)
	f.aps = []r1AP{
		{Serial: "SN-1", VenueID: "v", IP: "10.0.0.1"},
		{Serial: "SN-2", VenueID: "v", IP: "10.0.0.2"},
	}
	f.passwords = map[string]string{"SN-1": "jZS^M3#k6u8J#6*i", "SN-2": "*****"}

	r := resolverWith(t, f, "admin")
	pull := r.credentialsFor(context.Background(), []string{"10.0.0.1", "10.0.0.2"})

	if pull.Pulled != 1 || pull.Masked != 1 {
		t.Fatalf("pulled = %d, masked = %d; want 1 and 1", pull.Pulled, pull.Masked)
	}
	if got := pull.Creds["10.0.0.1"]; len(got) != 1 || got[0].Password != "jZS^M3#k6u8J#6*i" {
		t.Errorf("real password lost: %v", got)
	}
	if _, ok := pull.Creds["10.0.0.2"]; ok {
		t.Error("the masked AP must not carry a credential")
	}
	lines := r1PullSummary(pull, 2)
	if len(lines) != 2 || !strings.Contains(lines[1], "1 password(s) masked") {
		t.Errorf("mixed run should say how many were skipped; got %q", lines)
	}
}

// Detection is "nothing but asterisks". A real R1 password can contain an
// asterisk — the one that logged in above does — and must not be mistaken for
// a mask.
func TestIsMaskedPassword(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"**********", true},
		{"*", true},
		{"jZS^M3#k6u8J#6*i", false}, // real, contains '*'
		{"*a*", false},
		{"", false}, // empty is "no password", handled separately
		{"#M^o!Eu#Y19t9H2b", false},
	} {
		if got := isMaskedPassword(tc.in); got != tc.want {
			t.Errorf("isMaskedPassword(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
