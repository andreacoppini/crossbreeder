package wifi

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestChannelAndBandMapping(t *testing.T) {
	cases := []struct {
		freq    int
		channel int
		band    string
	}{
		{2412, 1, "2.4 GHz"},
		{2437, 6, "2.4 GHz"},
		{2484, 14, "2.4 GHz"},
		{5180, 36, "5 GHz"},
		{5500, 100, "5 GHz"},
		{5825, 165, "5 GHz"},
		{5955, 1, "6 GHz"},
		{6175, 45, "6 GHz"},
		{7115, 233, "6 GHz"},
		{0, 0, ""},
	}
	for _, c := range cases {
		if got := ChannelFor(c.freq); got != c.channel {
			t.Errorf("ChannelFor(%d) = %d, want %d", c.freq, got, c.channel)
		}
		if got := BandFor(c.freq); got != c.band {
			t.Errorf("BandFor(%d) = %q, want %q", c.freq, got, c.band)
		}
	}
}

func TestSecurityNames(t *testing.T) {
	cases := map[string]string{
		"[WPA2-PSK-CCMP][ESS]":           "WPA2-Personal",
		"[WPA2-EAP-CCMP][ESS]":           "WPA2-Enterprise",
		"[WPA2-PSK-CCMP][WPS][ESS]":      "WPA2-Personal",
		"[RSN-SAE-CCMP][MFPR][ESS]":      "WPA3-Personal",
		"[RSN-OWE-CCMP][ESS]":            "Enhanced Open",
		"[WEP][ESS]":                     "WEP",
		"[ESS]":                          "Open",
		"[WPA2-EAP+SAE-CCMP][MFPR][ESS]": "WPA3-Enterprise",
	}
	for flags, want := range cases {
		if got := SecurityFor(flags); got != want {
			t.Errorf("SecurityFor(%q) = %q, want %q", flags, got, want)
		}
	}
}

const scanTable = `bssid / frequency / signal level / flags / ssid
b8:27:eb:aa:bb:01	2437	-42	[WPA2-PSK-CCMP][ESS]	Campus Guest
b8:27:eb:aa:bb:02	5180	-55	[WPA2-EAP-CCMP][ESS]	Campus-Secure
b8:27:eb:aa:bb:03	2437	-77	[WPA2-PSK-CCMP][ESS]	Neighbour
b8:27:eb:aa:bb:04	2412	-60	[ESS]	
b8:27:eb:aa:bb:05	5500	-70	[WPA2-EAP-CCMP][ESS]	Campus-Secure
b8:27:eb:aa:bb:06	2427	-65	[WPA2-PSK-CCMP][ESS]	Overlapper
`

func TestParseScanResults(t *testing.T) {
	bsses := parseScanResults(scanTable)
	if len(bsses) != 6 {
		t.Fatalf("parsed %d radios", len(bsses))
	}
	if bsses[0].Signal != -42 {
		t.Errorf("results are not sorted by signal: %+v", bsses[0])
	}
	if bsses[0].SSID != "Campus Guest" {
		t.Errorf("an SSID containing a space was mangled: %q", bsses[0].SSID)
	}
	var hidden BSS
	for _, b := range bsses {
		if b.BSSID == "b8:27:eb:aa:bb:04" {
			hidden = b
		}
	}
	if hidden.SSID != "" || hidden.Security != "Open" {
		t.Errorf("hidden network = %+v", hidden)
	}
	if bsses[1].Channel != 36 || bsses[1].Band != "5 GHz" {
		t.Errorf("channel mapping failed: %+v", bsses[1])
	}
}

func TestSurveyCountsNeighbours(t *testing.T) {
	// Associated to the 5 GHz radio of Campus-Secure on channel 36.
	own := BSS{BSSID: "b8:27:eb:aa:bb:02", SSID: "Campus-Secure", Channel: 36, Band: "5 GHz"}
	n := Survey(parseScanResults(scanTable), own)
	if n.Total != 6 {
		t.Errorf("total = %d", n.Total)
	}
	if len(n.SameSSID) != 1 || n.SameSSID[0].BSSID != "b8:27:eb:aa:bb:05" {
		t.Errorf("roaming candidates = %+v, want the other Campus-Secure radio", n.SameSSID)
	}
	if n.Strongest == nil || n.Strongest.BSSID != "b8:27:eb:aa:bb:05" {
		t.Errorf("strongest candidate = %+v", n.Strongest)
	}
	if n.CoChannel != 0 {
		t.Errorf("co-channel on 36 = %d, want 0 — nothing else is up there", n.CoChannel)
	}

	// The guest radio on channel 6 shares its channel with one neighbour and
	// is partly covered by another on channel 4. Channel 1 is far enough away
	// to be neither, which is the whole point of the 1/6/11 plan.
	guest := BSS{BSSID: "b8:27:eb:aa:bb:01", SSID: "Campus Guest", Channel: 6, Band: "2.4 GHz"}
	crowded := Survey(parseScanResults(scanTable), guest)
	if crowded.CoChannel != 1 {
		t.Errorf("co-channel on 6 = %d, want 1", crowded.CoChannel)
	}
	if crowded.Overlapping != 1 {
		t.Errorf("overlapping on 6 = %d, want the channel 4 radio", crowded.Overlapping)
	}
	if crowded.Channels[6] != 2 {
		t.Errorf("channel 6 count = %d, want both radios there including our own", crowded.Channels[6])
	}
}

const surveyDump = `Survey data from wlan0
	frequency:			2412 MHz
	noise:				-95 dBm
	channel active time:		10000 ms
	channel busy time:		1500 ms
Survey data from wlan0
	frequency:			2437 MHz [in use]
	noise:				-92 dBm
	channel active time:		20000 ms
	channel busy time:		15000 ms
	channel receive time:		9000 ms
`

func TestParseSurveyDump(t *testing.T) {
	entries := parseSurveyDump(surveyDump)
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].Utilisation() != 15 {
		t.Errorf("channel 1 utilisation = %.1f%%, want 15", entries[0].Utilisation())
	}
	inUse := entries[1]
	if !inUse.InUse || inUse.Channel != 6 {
		t.Errorf("in-use entry = %+v", inUse)
	}
	if inUse.Utilisation() != 75 {
		t.Errorf("channel 6 utilisation = %.1f%%, want 75", inUse.Utilisation())
	}
	if inUse.Noise != -92 {
		t.Errorf("noise = %d", inUse.Noise)
	}
	// A driver that reports nothing must read as zero, not as a division by
	// zero.
	if (SurveyEntry{}).Utilisation() != 0 {
		t.Error("an empty survey entry produced a utilisation")
	}
}

func TestScanFallsBackToTheExistingTableWhenBusy(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("SCAN", "FAIL-BUSY")
	f.reply("SCAN_RESULTS", scanTable)
	c := dialFake(t, f, "wlan0")

	bsses, err := c.Scan(context.Background(), 200*time.Millisecond)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(bsses) != 6 {
		t.Errorf("a busy scan lost the previous results: %d radios", len(bsses))
	}
}

func TestScanWaitsForResultsEvent(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("SCAN_RESULTS", scanTable)
	f.onCommand = func(f *fakeSupplicant, cmd string) {
		if cmd == "SCAN" {
			f.emitSequence(20*time.Millisecond, "CTRL-EVENT-SCAN-RESULTS ")
		}
	}
	c := dialFake(t, f, "wlan0")

	start := time.Now()
	bsses, err := c.Scan(context.Background(), 5*time.Second)
	if err != nil || len(bsses) == 0 {
		t.Fatalf("scan: %d radios, %v", len(bsses), err)
	}
	// It must return when the results arrive, not sit out the whole window.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the scan waited %v after the results event", elapsed)
	}
}

func TestProfileCommands(t *testing.T) {
	p := Profile{
		SSID: "Campus-Secure", EAP: "peap", Identity: "sensor@example.com",
		Password: `pa"ss`, Phase2: "auth=MSCHAPV2", CACert: "/etc/ca.pem",
		SubjectMatch: "CN=radius.example.com", Freq: 5180, PMF: 1,
	}
	cmds := strings.Join(p.Commands(2), "\n")
	for _, want := range []string{
		`SET_NETWORK 2 ssid "Campus-Secure"`,
		`SET_NETWORK 2 key_mgmt WPA-EAP`,
		`SET_NETWORK 2 eap PEAP`,
		`SET_NETWORK 2 phase2 "auth=MSCHAPV2"`,
		`SET_NETWORK 2 subject_match "CN=radius.example.com"`,
		`SET_NETWORK 2 freq_list 5180`,
		`SET_NETWORK 2 ieee80211w 1`,
	} {
		if !strings.Contains(cmds, want) {
			t.Errorf("missing command: %s\ngot:\n%s", want, cmds)
		}
	}
	// A quote inside a passphrase would end the value early and leave the rest
	// of it being read as command syntax.
	if strings.Contains(cmds, `pa"ss`) {
		t.Error("a quote in a password reached the command line intact")
	}
}

func TestProfileSecurityInference(t *testing.T) {
	cases := []struct {
		p    Profile
		want string
	}{
		{Profile{SSID: "a", PSK: "passphrase"}, "WPA-PSK"},
		{Profile{SSID: "a"}, "NONE"},
		{Profile{SSID: "a", Security: "sae", PSK: "passphrase"}, "SAE"},
		{Profile{SSID: "a", Security: "owe"}, "OWE"},
		{Profile{SSID: "a", EAP: "TLS", ClientCert: "/c.pem", PrivateKey: "/k.pem"}, "WPA-EAP"},
	}
	for _, c := range cases {
		cmds := strings.Join(c.p.Commands(0), "\n")
		if !strings.Contains(cmds, "key_mgmt "+c.want) {
			t.Errorf("%+v produced %s, want key_mgmt %s", c.p.Redacted(), cmds, c.want)
		}
	}
	// A 64-character hexadecimal PSK is the key itself and must not be quoted.
	hex := strings.Repeat("ab", 32)
	if !strings.Contains(strings.Join(Profile{SSID: "a", PSK: hex}.Commands(0), "\n"), "psk "+hex) {
		t.Error("a raw PSK was quoted as a passphrase")
	}
}

func TestProfileValidation(t *testing.T) {
	bad := []Profile{
		{},
		{SSID: "a", PSK: "short"},
		{SSID: "a", EAP: "TLS"},
		{SSID: "a", EAP: "PEAP", Identity: "who"},
		{SSID: "a", Security: "wep"},
	}
	for _, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("%+v passed validation", p.Redacted())
		}
	}
	good := []Profile{
		{SSID: "a"},
		{SSID: "a", PSK: "passphrase"},
		{SSID: "a", EAP: "PEAP", Identity: "who", Password: "pw"},
		{SSID: "a", EAP: "TLS", ClientCert: "/c.pem", PrivateKey: "/k.pem"},
	}
	for _, p := range good {
		if err := p.Validate(); err != nil {
			t.Errorf("%+v: %v", p.Redacted(), err)
		}
	}
}

func TestRedactedKeepsSecretsOffTheScreen(t *testing.T) {
	p := Profile{SSID: "a", PSK: "supersecret", Password: "radiuspw", PrivateKeyPasswd: "keypw"}
	r := p.Redacted()
	if strings.Contains(r.PSK+r.Password+r.PrivateKeyPasswd, "secret") ||
		strings.Contains(r.Password, "radius") || strings.Contains(r.PrivateKeyPasswd, "keypw") {
		t.Fatalf("secrets survived redaction: %+v", r)
	}
	if p.PSK != "supersecret" {
		t.Error("redaction modified the original profile")
	}
}
