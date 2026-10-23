package main

import (
	"context"
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
	creds, matched, pulled := r.credentialsFor(context.Background(), []string{"192.168.1.50"})

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
	creds, matched, pulled := r.credentialsFor(context.Background(), []string{"10.0.0.1", "10.0.0.2"})

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
