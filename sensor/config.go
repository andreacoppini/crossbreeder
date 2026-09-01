package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

// Duration is a time.Duration that reads and writes as "30s" or "5m" in the
// configuration file, because a sensor's config is edited by people.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("%q is not a duration: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	// A bare number is read as seconds, which is what people write by mistake
	// and mean.
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return errors.New("a duration must be written as a string, such as \"30s\"")
	}
	*d = Duration(time.Duration(n * float64(time.Second)))
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

// Config is everything a sensor needs to know. One file, editable by hand,
// pushed down from the collector when a sensor is part of a fleet.
type Config struct {
	Sensor     SensorConfig `json:"sensor"`
	Networks   []Network    `json:"networks"`
	Thresholds Thresholds   `json:"thresholds"`
	Alerts     AlertConfig  `json:"alerts"`
	Upstream   Upstream     `json:"upstream"`
	Storage    Storage      `json:"storage"`
}

// SensorConfig identifies the sensor and names its hardware.
type SensorConfig struct {
	Name string `json:"name"`
	Site string `json:"site"`
	// Group is what a fleet is organised by: a floor, a building, a customer.
	Group string `json:"group"`
	Notes string `json:"notes,omitempty"`

	// WirelessInterface is the radio that associates and runs the tests.
	WirelessInterface string `json:"wireless_interface"`
	// MonitorInterface is a second radio, if the sensor has one, that stays
	// out of the tests and only listens: scanning while associated costs the
	// test radio air time and skews every measurement taken during the scan.
	MonitorInterface string `json:"monitor_interface,omitempty"`
	WiredInterface   string `json:"wired_interface,omitempty"`
	CtrlDir          string `json:"wpa_control_dir,omitempty"`

	// Interval is the rest between passes over every network.
	Interval Duration `json:"interval"`
	// Listen is where the local dashboard binds. Loopback by default: a
	// sensor on a guest network must not offer a web interface to it.
	Listen string `json:"listen"`
	// ReflectorListen answers other sensors' voice tests, so a pair of
	// sensors can measure the path between two sites.
	ReflectorListen string `json:"reflector_listen,omitempty"`
	// ThroughputListen answers other sensors' rate tests.
	ThroughputListen string `json:"throughput_listen,omitempty"`
}

// Network is one thing to test: an SSID, or the wired port.
type Network struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // wifi or wired
	Enabled *bool  `json:"enabled,omitempty"`
	// Interface overrides the sensor's default for this network, which is how
	// a sensor with two radios tests two SSIDs at once.
	Interface string       `json:"interface,omitempty"`
	Profile   wifi.Profile `json:"profile,omitzero"`
	Tests     TestPlan     `json:"tests"`
}

// On reports whether this network is tested. Absent means yes: a network in
// the file is there to be tested.
func (n Network) On() bool { return n.Enabled == nil || *n.Enabled }

// Wireless reports whether this network needs the radio.
func (n Network) Wireless() bool { return !strings.EqualFold(n.Kind, "wired") }

// TestPlan is what to run on a network, in the order the layers depend on
// each other.
type TestPlan struct {
	DHCP          *bool             `json:"dhcp,omitempty"`
	Gateway       *bool             `json:"gateway,omitempty"`
	CaptivePortal *bool             `json:"captive_portal,omitempty"`
	Discovery     *bool             `json:"discovery,omitempty"` // LLDP/CDP on a wired port
	DNS           []DNSTarget       `json:"dns,omitempty"`
	Internet      []string          `json:"internet,omitempty"` // hosts to ping
	Web           []WebTarget       `json:"web,omitempty"`
	Apps          []string          `json:"apps,omitempty"` // names from the catalogue
	Traceroute    []string          `json:"traceroute,omitempty"`
	VoIP          *VoIPTarget       `json:"voip,omitempty"`
	Throughput    *ThroughputTarget `json:"throughput,omitempty"`
	Roaming       *bool             `json:"roaming,omitempty"`
	Certificates  []string          `json:"certificates,omitempty"` // host:port to watch expiry on
}

func on(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// DNSTarget is one resolver and one name to ask it for.
type DNSTarget struct {
	Name string `json:"name,omitempty"`
	// Server empty means whatever DHCP handed out, which is the test that
	// matters most: the resolver the clients on this network are using.
	Server string `json:"server,omitempty"`
	Query  string `json:"query"`
	Type   string `json:"type,omitempty"`
	Proto  string `json:"proto,omitempty"`
	Expect string `json:"expect,omitempty"`
}

// WebTarget is one page or endpoint to fetch.
type WebTarget struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	ExpectStatus int    `json:"expect_status,omitempty"`
	ExpectBody   string `json:"expect_body,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
	Follow       bool   `json:"follow,omitempty"`
}

// VoIPTarget is a call-shaped stream to a reflector.
type VoIPTarget struct {
	Reflector string `json:"reflector"`
	Packets   int    `json:"packets,omitempty"`
	DSCP      int    `json:"dscp,omitempty"`
	Codec     string `json:"codec,omitempty"`
}

// ThroughputTarget is a rate measurement.
type ThroughputTarget struct {
	Mode     string   `json:"mode,omitempty"` // http, peer, iperf3
	URL      string   `json:"url,omitempty"`
	Peer     string   `json:"peer,omitempty"`
	Streams  int      `json:"streams,omitempty"`
	Duration Duration `json:"duration,omitempty"`
	Upload   bool     `json:"upload,omitempty"`
	// Every is how often to run it. A rate test moves real traffic, so it
	// runs on its own slower schedule rather than every pass.
	Every Duration `json:"every,omitempty"`
	// ExpectMbps is the rate the site is paying for; below it is a finding.
	ExpectMbps float64 `json:"expect_mbps,omitempty"`
}

// Thresholds turn measurements into judgements. They are in the config
// because "slow" at a hospital and "slow" at a warehouse are different
// numbers, and because an operator who cannot move the line will ignore the
// alerts instead.
type Thresholds struct {
	AssociationWarn Duration `json:"association_warn"`
	AssociationFail Duration `json:"association_fail"`
	EAPWarn         Duration `json:"eap_warn"`
	DHCPWarn        Duration `json:"dhcp_warn"`
	DHCPFail        Duration `json:"dhcp_fail"`
	DNSWarn         Duration `json:"dns_warn"`
	DNSFail         Duration `json:"dns_fail"`
	GatewayWarn     Duration `json:"gateway_warn"`
	GatewayFail     Duration `json:"gateway_fail"`
	InternetWarn    Duration `json:"internet_warn"`
	InternetFail    Duration `json:"internet_fail"`
	WebWarn         Duration `json:"web_warn"`
	WebFail         Duration `json:"web_fail"`
	SignalWarn      int      `json:"signal_warn"` // dBm
	SignalFail      int      `json:"signal_fail"`
	SNRWarn         int      `json:"snr_warn"`
	MOSWarn         float64  `json:"mos_warn"`
	MOSFail         float64  `json:"mos_fail"`
	UtilisationWarn float64  `json:"utilisation_warn"` // percent of air time
	UtilisationFail float64  `json:"utilisation_fail"`
	CertWarnDays    int      `json:"cert_warn_days"`
	CertFailDays    int      `json:"cert_fail_days"`
	LossWarnPct     float64  `json:"loss_warn_pct"`
	LossFailPct     float64  `json:"loss_fail_pct"`
}

// AlertConfig says where findings go.
type AlertConfig struct {
	Enabled  bool     `json:"enabled"`
	Webhooks []string `json:"webhooks,omitempty"`
	Slack    string   `json:"slack_webhook,omitempty"`
	Syslog   string   `json:"syslog,omitempty"` // host:port, RFC 5424 over UDP
	Email    *Email   `json:"email,omitempty"`
	// MinSeverity is "warning" or "critical".
	MinSeverity string `json:"min_severity,omitempty"`
	// Repeat is how long an issue stays quiet after it has been reported
	// once. Without it a flapping network sends a message every pass.
	Repeat Duration `json:"repeat,omitempty"`
}

// Email is an SMTP destination for alerts.
type Email struct {
	Server   string   `json:"server"` // host:port
	From     string   `json:"from"`
	To       []string `json:"to"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	StartTLS bool     `json:"starttls,omitempty"`
}

// Upstream links a sensor to a collector, which is what turns a box on a shelf
// into a fleet.
type Upstream struct {
	URL       string   `json:"url,omitempty"`
	Token     string   `json:"token,omitempty"`
	Every     Duration `json:"every,omitempty"`
	Insecure  bool     `json:"insecure,omitempty"`
	AcceptCfg bool     `json:"accept_config,omitempty"` // let the collector push config
}

// Storage bounds what the sensor keeps. An SD card is small and does not like
// being written to, which is the practical limit on a Pi.
type Storage struct {
	Dir    string   `json:"dir,omitempty"`
	Keep   Duration `json:"keep,omitempty"`
	MaxMiB int      `json:"max_mib,omitempty"`
}

// DefaultConfig is a sensor that does something useful the moment it is
// switched on: it tests the wired port it is plugged into, resolves and
// fetches over it, and keeps a fortnight of history.
func DefaultConfig() Config {
	yes, no := true, false
	_ = no
	return Config{
		Sensor: SensorConfig{
			Name:              defaultName(),
			WirelessInterface: "wlan0",
			WiredInterface:    "eth0",
			CtrlDir:           wifi.DefaultCtrlDir,
			Interval:          Duration(5 * time.Minute),
			Listen:            "127.0.0.1:52414",
		},
		Networks: []Network{{
			Name: "Wired", Kind: "wired", Enabled: &yes,
			Tests: TestPlan{
				DHCP: &yes, Gateway: &yes, CaptivePortal: &yes, Discovery: &yes,
				DNS:      []DNSTarget{{Query: "www.google.com"}, {Query: "outlook.office365.com"}},
				Internet: []string{"1.1.1.1", "8.8.8.8"},
				Apps:     []string{"Microsoft 365", "Google", "Zoom"},
			},
		}},
		Thresholds: DefaultThresholds(),
		Alerts:     AlertConfig{MinSeverity: "warning", Repeat: Duration(time.Hour)},
		Storage:    Storage{Keep: Duration(14 * 24 * time.Hour), MaxMiB: 512},
	}
}

// DefaultThresholds are the lines this tool draws when nobody has drawn their
// own. They are the ones a wireless engineer would argue for: a client that
// takes more than five seconds to get on has a problem, a DNS answer past
// half a second is broken rather than slow, and -75 dBm is where a phone
// starts to struggle.
func DefaultThresholds() Thresholds {
	return Thresholds{
		AssociationWarn: Duration(5 * time.Second),
		AssociationFail: Duration(15 * time.Second),
		EAPWarn:         Duration(3 * time.Second),
		DHCPWarn:        Duration(time.Second),
		DHCPFail:        Duration(5 * time.Second),
		DNSWarn:         Duration(100 * time.Millisecond),
		DNSFail:         Duration(500 * time.Millisecond),
		GatewayWarn:     Duration(20 * time.Millisecond),
		GatewayFail:     Duration(150 * time.Millisecond),
		InternetWarn:    Duration(100 * time.Millisecond),
		InternetFail:    Duration(300 * time.Millisecond),
		WebWarn:         Duration(1500 * time.Millisecond),
		WebFail:         Duration(5 * time.Second),
		SignalWarn:      -70,
		SignalFail:      -80,
		SNRWarn:         20,
		MOSWarn:         4.0,
		MOSFail:         3.6,
		UtilisationWarn: 50,
		UtilisationFail: 75,
		CertWarnDays:    30,
		CertFailDays:    7,
		LossWarnPct:     1,
		LossFailPct:     5,
	}
}

func defaultName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "sensor"
	}
	return host
}

// LoadConfig reads a configuration file, filling in every default the file
// leaves out, so a two-line file is a valid one.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	// Networks are replaced rather than merged: a file that names one SSID
	// means that SSID, not that one plus the default wired network.
	cfg.Networks = nil
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	cfg.applyDefaults()
	return cfg, cfg.Validate()
}

// applyDefaults fills in the fields a hand-written file leaves out.
func (c *Config) applyDefaults() {
	d := DefaultConfig()
	if c.Sensor.Name == "" {
		c.Sensor.Name = d.Sensor.Name
	}
	if c.Sensor.WirelessInterface == "" {
		c.Sensor.WirelessInterface = d.Sensor.WirelessInterface
	}
	if c.Sensor.WiredInterface == "" {
		c.Sensor.WiredInterface = d.Sensor.WiredInterface
	}
	if c.Sensor.CtrlDir == "" {
		c.Sensor.CtrlDir = d.Sensor.CtrlDir
	}
	if c.Sensor.Interval <= 0 {
		c.Sensor.Interval = d.Sensor.Interval
	}
	if c.Sensor.Listen == "" {
		c.Sensor.Listen = d.Sensor.Listen
	}
	if c.Storage.Keep <= 0 {
		c.Storage.Keep = d.Storage.Keep
	}
	if c.Storage.MaxMiB <= 0 {
		c.Storage.MaxMiB = d.Storage.MaxMiB
	}
	if c.Alerts.MinSeverity == "" {
		c.Alerts.MinSeverity = "warning"
	}
	if c.Alerts.Repeat <= 0 {
		c.Alerts.Repeat = Duration(time.Hour)
	}
	c.Thresholds.fillFrom(DefaultThresholds())
	for i := range c.Networks {
		if c.Networks[i].Kind == "" {
			c.Networks[i].Kind = "wifi"
			if c.Networks[i].Profile.SSID == "" {
				c.Networks[i].Kind = "wired"
			}
		}
		if c.Networks[i].Name == "" {
			if ssid := c.Networks[i].Profile.SSID; ssid != "" {
				c.Networks[i].Name = ssid
			} else {
				c.Networks[i].Name = strings.Title(c.Networks[i].Kind)
			}
		}
	}
}

// fillFrom replaces any threshold left at zero with the default, so a file
// that tunes one line does not silently switch the others off.
func (t *Thresholds) fillFrom(d Thresholds) {
	if t.AssociationWarn <= 0 {
		t.AssociationWarn = d.AssociationWarn
	}
	if t.AssociationFail <= 0 {
		t.AssociationFail = d.AssociationFail
	}
	if t.EAPWarn <= 0 {
		t.EAPWarn = d.EAPWarn
	}
	if t.DHCPWarn <= 0 {
		t.DHCPWarn = d.DHCPWarn
	}
	if t.DHCPFail <= 0 {
		t.DHCPFail = d.DHCPFail
	}
	if t.DNSWarn <= 0 {
		t.DNSWarn = d.DNSWarn
	}
	if t.DNSFail <= 0 {
		t.DNSFail = d.DNSFail
	}
	if t.GatewayWarn <= 0 {
		t.GatewayWarn = d.GatewayWarn
	}
	if t.GatewayFail <= 0 {
		t.GatewayFail = d.GatewayFail
	}
	if t.InternetWarn <= 0 {
		t.InternetWarn = d.InternetWarn
	}
	if t.InternetFail <= 0 {
		t.InternetFail = d.InternetFail
	}
	if t.WebWarn <= 0 {
		t.WebWarn = d.WebWarn
	}
	if t.WebFail <= 0 {
		t.WebFail = d.WebFail
	}
	if t.SignalWarn == 0 {
		t.SignalWarn = d.SignalWarn
	}
	if t.SignalFail == 0 {
		t.SignalFail = d.SignalFail
	}
	if t.SNRWarn == 0 {
		t.SNRWarn = d.SNRWarn
	}
	if t.MOSWarn == 0 {
		t.MOSWarn = d.MOSWarn
	}
	if t.MOSFail == 0 {
		t.MOSFail = d.MOSFail
	}
	if t.UtilisationWarn == 0 {
		t.UtilisationWarn = d.UtilisationWarn
	}
	if t.UtilisationFail == 0 {
		t.UtilisationFail = d.UtilisationFail
	}
	if t.CertWarnDays == 0 {
		t.CertWarnDays = d.CertWarnDays
	}
	if t.CertFailDays == 0 {
		t.CertFailDays = d.CertFailDays
	}
	if t.LossWarnPct == 0 {
		t.LossWarnPct = d.LossWarnPct
	}
	if t.LossFailPct == 0 {
		t.LossFailPct = d.LossFailPct
	}
}

// Validate reports configuration errors before the sensor starts testing,
// where they read as configuration errors rather than as a network fault.
func (c Config) Validate() error {
	if len(c.Networks) == 0 {
		return errors.New("no networks to test")
	}
	names := map[string]bool{}
	for _, n := range c.Networks {
		if names[n.Name] {
			return fmt.Errorf("two networks are both called %q", n.Name)
		}
		names[n.Name] = true
		if n.Wireless() {
			if err := n.Profile.Validate(); err != nil {
				return err
			}
		}
		for _, app := range n.Tests.Apps {
			if _, ok := LookupApp(app); !ok {
				return fmt.Errorf("%s: no application called %q — run with -apps to list them", n.Name, app)
			}
		}
		if t := n.Tests.Throughput; t != nil {
			switch strings.ToLower(t.Mode) {
			case "", "http":
				if t.URL == "" {
					return fmt.Errorf("%s: a throughput test over HTTP needs a URL", n.Name)
				}
			case "peer", "iperf3":
				if t.Peer == "" {
					return fmt.Errorf("%s: a %s throughput test needs a peer", n.Name, t.Mode)
				}
			default:
				return fmt.Errorf("%s: unknown throughput mode %q", n.Name, t.Mode)
			}
		}
		if v := n.Tests.VoIP; v != nil && v.Reflector == "" {
			return fmt.Errorf("%s: a voice test needs a reflector to answer it", n.Name)
		}
	}
	if c.Upstream.URL != "" && c.Upstream.Token == "" {
		return errors.New("a collector URL needs a token")
	}
	return nil
}

// Save writes the configuration back, preserving the file's permissions where
// it already exists. It writes through a temporary file so a sensor that
// loses power mid-write still has a config to start from.
func (c Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".new"
	// The file holds passphrases and 802.1X passwords, so it is never
	// world-readable, whatever umask the sensor was started with.
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Redacted returns a copy with every secret replaced, for the dashboard, the
// API and the logs.
func (c Config) Redacted() Config {
	out := c
	out.Networks = make([]Network, len(c.Networks))
	copy(out.Networks, c.Networks)
	for i := range out.Networks {
		out.Networks[i].Profile = out.Networks[i].Profile.Redacted()
	}
	if out.Upstream.Token != "" {
		out.Upstream.Token = "********"
	}
	if out.Alerts.Email != nil {
		email := *out.Alerts.Email
		if email.Password != "" {
			email.Password = "********"
		}
		out.Alerts.Email = &email
	}
	if out.Alerts.Slack != "" {
		out.Alerts.Slack = "********"
	}
	return out
}

// restoreSecrets copies back any secret that came in redacted, so a
// configuration edited in the dashboard — which is only ever shown the
// redacted form — does not overwrite a passphrase with asterisks.
func (c *Config) restoreSecrets(old Config) {
	const mask = "********"
	if c.Upstream.Token == mask {
		c.Upstream.Token = old.Upstream.Token
	}
	if c.Alerts.Slack == mask {
		c.Alerts.Slack = old.Alerts.Slack
	}
	if c.Alerts.Email != nil && c.Alerts.Email.Password == mask && old.Alerts.Email != nil {
		c.Alerts.Email.Password = old.Alerts.Email.Password
	}
	previous := map[string]wifi.Profile{}
	for _, n := range old.Networks {
		previous[n.Name] = n.Profile
	}
	for i, n := range c.Networks {
		was, ok := previous[n.Name]
		if !ok {
			continue
		}
		if n.Profile.PSK == mask {
			c.Networks[i].Profile.PSK = was.PSK
		}
		if n.Profile.Password == mask {
			c.Networks[i].Profile.Password = was.Password
		}
		if n.Profile.WEPKey == mask {
			c.Networks[i].Profile.WEPKey = was.WEPKey
		}
		if n.Profile.PrivateKeyPasswd == mask {
			c.Networks[i].Profile.PrivateKeyPasswd = was.PrivateKeyPasswd
		}
	}
}
