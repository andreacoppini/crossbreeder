package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

func TestLoadConfigFillsInDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sensor.json")
	os.WriteFile(path, []byte(`{
	  "sensor": {"name": "lobby-1", "site": "Head office", "interval": "2m"},
	  "networks": [
	    {"name": "Corp", "kind": "wifi", "profile": {"SSID": "Corp", "PSK": "passphrase"},
	     "tests": {"dhcp": true, "dns": [{"query": "intranet.example.com"}]}}
	  ],
	  "thresholds": {"dns_warn": "250ms"}
	}`), 0o600)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Sensor.Interval.D() != 2*time.Minute {
		t.Errorf("interval = %v", cfg.Sensor.Interval.D())
	}
	if cfg.Sensor.WirelessInterface != "wlan0" || cfg.Sensor.Listen == "" {
		t.Errorf("defaults were not filled in: %+v", cfg.Sensor)
	}
	// A file that tunes one threshold must not switch the rest off.
	if cfg.Thresholds.DNSWarn.D() != 250*time.Millisecond {
		t.Errorf("DNS warn = %v", cfg.Thresholds.DNSWarn.D())
	}
	if cfg.Thresholds.DHCPFail.D() != DefaultThresholds().DHCPFail.D() {
		t.Errorf("an untouched threshold was zeroed: %v", cfg.Thresholds.DHCPFail.D())
	}
	// The default wired network must not survive alongside a file's own list.
	if len(cfg.Networks) != 1 || cfg.Networks[0].Name != "Corp" {
		t.Fatalf("networks = %+v", cfg.Networks)
	}
	if !cfg.Networks[0].On() {
		t.Error("a network with no enabled flag was treated as disabled")
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(c *Config)
		want string
	}{
		{"no networks", func(c *Config) { c.Networks = nil }, "no networks"},
		{"duplicate names", func(c *Config) {
			c.Networks = append(c.Networks, c.Networks[0])
		}, "both called"},
		{"unknown application", func(c *Config) {
			c.Networks[0].Tests.Apps = []string{"Lotus Notes"}
		}, "no application called"},
		{"throughput without a target", func(c *Config) {
			c.Networks[0].Tests.Throughput = &ThroughputTarget{Mode: "peer"}
		}, "needs a peer"},
		{"voice without a reflector", func(c *Config) {
			c.Networks[0].Tests.VoIP = &VoIPTarget{}
		}, "needs a reflector"},
		{"collector without a token", func(c *Config) {
			c.Upstream.URL = "https://collector.example.com"
		}, "needs a token"},
		{"bad passphrase", func(c *Config) {
			c.Networks[0].Kind = "wifi"
			c.Networks[0].Profile.SSID = "Corp"
			c.Networks[0].Profile.PSK = "short"
		}, "8 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.cfg(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("the configuration passed validation")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("the default configuration does not validate: %v", err)
	}
}

func TestConfigSaveRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sensor.json")
	cfg := DefaultConfig()
	cfg.Sensor.Name = "roof-1"
	cfg.Networks = append(cfg.Networks, Network{
		Name: "Corp", Kind: "wifi",
		Profile: wifiProfile("Corp", "a-passphrase"),
		Tests:   TestPlan{Apps: []string{"Microsoft 365"}},
	})
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// The file holds passphrases.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
	back, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if back.Sensor.Name != "roof-1" || len(back.Networks) != 2 {
		t.Fatalf("round trip changed the config: %+v", back.Sensor)
	}
	if back.Networks[1].Profile.PSK != "a-passphrase" {
		t.Error("the passphrase did not survive a save and reload")
	}
}

func TestRedactedRemovesSecrets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Networks = append(cfg.Networks, Network{
		Name: "Corp", Kind: "wifi", Profile: wifiProfile("Corp", "a-passphrase"),
	})
	cfg.Upstream = Upstream{URL: "https://collector", Token: "s3cr3t"}
	cfg.Alerts.Slack = "https://hooks.slack.com/services/T/B/XYZ"
	cfg.Alerts.Email = &Email{Server: "smtp:25", Password: "mailpw"}

	blob, err := json.Marshal(cfg.Redacted())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"a-passphrase", "s3cr3t", "XYZ", "mailpw"} {
		if strings.Contains(string(blob), secret) {
			t.Errorf("%q survived redaction", secret)
		}
	}
	// Redaction must not damage the live configuration.
	if cfg.Upstream.Token != "s3cr3t" || cfg.Networks[1].Profile.PSK != "a-passphrase" {
		t.Error("redaction modified the original")
	}
}

func TestDurationParsing(t *testing.T) {
	var d struct {
		A Duration `json:"a"`
		B Duration `json:"b"`
	}
	if err := json.Unmarshal([]byte(`{"a":"90s","b":45}`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.A.D() != 90*time.Second {
		t.Errorf("a = %v", d.A.D())
	}
	// A bare number is what people write when they mean seconds.
	if d.B.D() != 45*time.Second {
		t.Errorf("b = %v", d.B.D())
	}
	out, _ := json.Marshal(d)
	if !strings.Contains(string(out), `"1m30s"`) {
		t.Errorf("durations are not written back readably: %s", out)
	}
	if err := json.Unmarshal([]byte(`{"a":"soon"}`), &d); err == nil {
		t.Error("\"soon\" parsed as a duration")
	}
}

func TestAppCatalogue(t *testing.T) {
	if _, ok := LookupApp("microsoft365"); !ok {
		t.Error("the catalogue is case and spacing sensitive")
	}
	if _, ok := LookupApp("  Microsoft 365 "); !ok {
		t.Error("a name with stray spaces was not found")
	}
	if _, ok := LookupApp("Nothing"); ok {
		t.Error("an application that does not exist was found")
	}
	if len(AppNames()) < 10 {
		t.Errorf("the catalogue holds only %d applications", len(AppNames()))
	}
	for _, app := range appCatalogue {
		if len(app.Tests) == 0 {
			t.Errorf("%s has no endpoints to test", app.Name)
		}
		for _, target := range app.Tests {
			if !strings.HasPrefix(target.URL, "http") || target.Name == "" {
				t.Errorf("%s: bad target %+v", app.Name, target)
			}
		}
	}
	if len(AppsByCategory()) < 3 {
		t.Error("the catalogue is not grouped into categories")
	}
}

// wifiProfile is shorthand for the tests.
func wifiProfile(ssid, psk string) wifi.Profile {
	return wifi.Profile{SSID: ssid, PSK: psk}
}
