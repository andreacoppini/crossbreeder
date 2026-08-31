package wifi

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A successful enterprise association, played out as wpa_supplicant would
// report it, has to come back with every phase separately timed.
func TestConnectTimesEachPhase(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("ADD_NETWORK", "3")
	f.reply("STATUS", "bssid=b8:27:eb:aa:bb:cc\nfreq=5180\nssid=Campus-Secure\nkey_mgmt=WPA2/IEEE 802.1X/EAP\nwpa_state=COMPLETED\n")
	f.reply("SIGNAL_POLL", "RSSI=-58\nNOISE=-95\nFREQUENCY=5180\nLINKSPEED=400\n")
	f.onCommand = func(f *fakeSupplicant, cmd string) {
		if strings.HasPrefix(cmd, "SELECT_NETWORK") {
			f.emitSequence(15*time.Millisecond,
				"CTRL-EVENT-SCAN-STARTED",
				"CTRL-EVENT-SCAN-RESULTS",
				"SME: Trying to authenticate with b8:27:eb:aa:bb:cc (SSID='Campus-Secure')",
				"Associated with b8:27:eb:aa:bb:cc",
				"CTRL-EVENT-EAP-STARTED EAP authentication started",
				"CTRL-EVENT-EAP-SUCCESS EAP authentication completed successfully",
				"WPA: Key negotiation completed with b8:27:eb:aa:bb:cc [PTK=CCMP GTK=CCMP]",
				"CTRL-EVENT-CONNECTED - Connection to b8:27:eb:aa:bb:cc completed",
			)
		}
	}
	c := dialFake(t, f, "wlan0")

	a := c.Connect(context.Background(), Profile{
		SSID: "Campus-Secure", EAP: "PEAP", Identity: "sensor@example.com",
		Password: "hunter2", Phase2: "auth=MSCHAPV2", CACert: "/etc/ssl/certs/ca.pem",
	}, 5*time.Second)

	if !a.OK() {
		t.Fatalf("association failed: %v (%s)", a.Err, a.Failure)
	}
	if a.BSSID != "b8:27:eb:aa:bb:cc" || a.Channel != 36 {
		t.Errorf("association = %+v", a)
	}
	if a.Scan <= 0 || a.Auth <= 0 || a.EAP <= 0 || a.Key <= 0 {
		t.Errorf("a phase came back unmeasured: scan=%v auth=%v eap=%v key=%v", a.Scan, a.Auth, a.EAP, a.Key)
	}
	if a.Total < a.Scan+a.Auth+a.EAP {
		t.Errorf("total %v is shorter than its own phases", a.Total)
	}
	if a.Signal.RSSI != -58 || a.Signal.SNR != 37 {
		t.Errorf("signal = %+v", a.Signal)
	}
	// The profile has to reach the supplicant, secrets included, but the
	// identity and password must be quoted rather than pasted.
	if got := f.sentContaining("identity"); len(got) == 0 || !strings.Contains(got[0], `"sensor@example.com"`) {
		t.Errorf("identity was not set as expected: %v", got)
	}
	if len(f.sentContaining("REMOVE_NETWORK all")) == 0 {
		t.Error("the previous test's profile was not cleared first")
	}
}

func TestConnectClassifiesFailures(t *testing.T) {
	cases := []struct {
		name  string
		event string
		want  string
	}{
		{"wrong passphrase", "CTRL-EVENT-SSID-TEMP-DISABLED id=0 ssid=\"Guest\" auth_failures=1 duration=10 reason=WRONG_KEY", "passphrase"},
		{"no such network", "CTRL-EVENT-NETWORK-NOT-FOUND ", "not on the air"},
		{"RADIUS said no", "CTRL-EVENT-EAP-FAILURE EAP authentication failed", "802.1X"},
		{"AP is full", "CTRL-EVENT-ASSOC-REJECT bssid=b8:27:eb:aa:bb:cc status_code=17", "association limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeSupplicant(t, "wlan0")
			f.reply("ADD_NETWORK", "0")
			f.onCommand = func(f *fakeSupplicant, cmd string) {
				if strings.HasPrefix(cmd, "SELECT_NETWORK") {
					f.emitSequence(10*time.Millisecond, tc.event)
				}
			}
			c := dialFake(t, f, "wlan0")
			a := c.Connect(context.Background(), Profile{SSID: "Guest", PSK: "letmein1"}, 3*time.Second)
			if a.OK() {
				t.Fatal("a failed association reported success")
			}
			if !strings.Contains(a.Failure, tc.want) {
				t.Errorf("failure = %q, want it to mention %q", a.Failure, tc.want)
			}
		})
	}
}

// A supplicant that says nothing at all is the commonest field failure: the
// sensor has to give up on its own clock and say where it got stuck.
func TestConnectTimesOutAndNamesTheState(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("ADD_NETWORK", "0")
	f.reply("STATUS", "wpa_state=4WAY_HANDSHAKE\n")
	c := dialFake(t, f, "wlan0")

	start := time.Now()
	a := c.Connect(context.Background(), Profile{SSID: "Guest", PSK: "letmein1"}, 400*time.Millisecond)
	if a.OK() {
		t.Fatal("a silent supplicant reported an association")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("a 400ms timeout took %v", elapsed)
	}
	if !strings.Contains(a.Failure, "4way_handshake") {
		t.Errorf("failure = %q, want the state it was stuck in", a.Failure)
	}
}

func TestConnectRefusesAMisconfiguredProfile(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	c := dialFake(t, f, "wlan0")
	a := c.Connect(context.Background(), Profile{SSID: "Guest", PSK: "short"}, time.Second)
	if a.OK() || a.Failure != "misconfigured" {
		t.Fatalf("a five-character passphrase was accepted: %+v", a)
	}
	if len(f.sent()) != 0 {
		t.Errorf("the radio was touched anyway: %v", f.sent())
	}
}

func TestRoamTimesTheHandover(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("STATUS", "bssid=b8:27:eb:99:88:77\nfreq=5200\nssid=Campus-Secure\nwpa_state=COMPLETED\n")
	f.onCommand = func(f *fakeSupplicant, cmd string) {
		if strings.HasPrefix(cmd, "ROAM") {
			f.emitSequence(20*time.Millisecond, "CTRL-EVENT-CONNECTED - Connection to b8:27:eb:99:88:77 completed")
		}
	}
	c := dialFake(t, f, "wlan0")

	took, landed, err := c.Roam(context.Background(), "b8:27:eb:99:88:77", 3*time.Second)
	if err != nil {
		t.Fatalf("roam: %v", err)
	}
	if took <= 0 {
		t.Errorf("the handover was not timed: %v", took)
	}
	if landed != "b8:27:eb:99:88:77" {
		t.Errorf("landed on %q", landed)
	}
}

func TestRoamThatNeverCompletes(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	c := dialFake(t, f, "wlan0")
	if _, _, err := c.Roam(context.Background(), "b8:27:eb:00:00:01", 300*time.Millisecond); err == nil {
		t.Fatal("a roam that never completed was reported as done")
	}
}

func TestStatusCodesAreNamed(t *testing.T) {
	if got := statusSuffix("CTRL-EVENT-ASSOC-REJECT status_code=17", "status_code="); !strings.Contains(got, "association limit") {
		t.Errorf("status 17 = %q", got)
	}
	if got := statusSuffix("CTRL-EVENT-DISCONNECTED reason=3 locally_generated=1", "reason="); got != " (code 3)" {
		t.Errorf("unnamed reason = %q", got)
	}
	if got := statusSuffix("CTRL-EVENT-ASSOC-REJECT status_code=0", "status_code="); got != "" {
		t.Errorf("a zero code produced %q", got)
	}
}
