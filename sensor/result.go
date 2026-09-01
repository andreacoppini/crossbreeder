package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Service is the layer a measurement belongs to. Grouping by service is what
// makes a result readable: nine measurements failing at once is one story if
// they are all applications and a different one if they are all DNS.
type Service string

const (
	ServiceWireless     Service = "wireless"
	ServiceAuth         Service = "authentication"
	ServiceDHCP         Service = "dhcp"
	ServiceDNS          Service = "dns"
	ServiceGateway      Service = "gateway"
	ServiceInternet     Service = "internet"
	ServiceApplications Service = "applications"
	ServiceVoice        Service = "voice"
	ServiceThroughput   Service = "throughput"
	ServiceLAN          Service = "lan"
)

// ServiceOrder is the order the layers depend on each other, which is the
// order everything is reported in: the first failure down this list is
// usually the cause of the rest.
var ServiceOrder = []Service{
	ServiceWireless, ServiceAuth, ServiceDHCP, ServiceGateway,
	ServiceDNS, ServiceInternet, ServiceApplications, ServiceVoice,
	ServiceThroughput, ServiceLAN,
}

// Status is the judgement on one measurement.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

// Worse reports whether s is a worse outcome than other.
func (s Status) Worse(other Status) bool { return statusRank[s] > statusRank[other] }

var statusRank = map[Status]int{StatusSkipped: 0, StatusOK: 1, StatusWarn: 2, StatusFail: 3}

// Measurement is one number with a judgement attached. Everything the sensor
// does ends up as one of these, which is what lets one dashboard, one alert
// path and one export cover every test.
type Measurement struct {
	Test    string            `json:"test"`
	Service Service           `json:"service"`
	Target  string            `json:"target,omitempty"`
	Status  Status            `json:"status"`
	Value   float64           `json:"value"`
	Unit    string            `json:"unit,omitempty"`
	Detail  string            `json:"detail,omitempty"`
	Error   string            `json:"error,omitempty"`
	Extra   map[string]string `json:"extra,omitempty"`
	At      time.Time         `json:"at"`
}

// Failed reports whether this measurement is a failure rather than a slow
// pass. Skipped tests are not failures: a test that could not run because the
// layer beneath it was down has nothing to say.
func (m Measurement) Failed() bool { return m.Status == StatusFail }

// String renders a measurement the way the command line prints it.
func (m Measurement) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-6s %-28s", strings.ToUpper(string(m.Status)), m.Test)
	if m.Unit != "" {
		fmt.Fprintf(&b, " %8s", formatValue(m.Value, m.Unit))
	} else {
		fmt.Fprintf(&b, " %8s", "")
	}
	switch {
	case m.Error != "":
		fmt.Fprintf(&b, "  %s", m.Error)
	case m.Detail != "":
		fmt.Fprintf(&b, "  %s", m.Detail)
	}
	return b.String()
}

func formatValue(v float64, unit string) string {
	switch unit {
	case "ms":
		switch {
		case v >= 1000:
			return fmt.Sprintf("%.2fs", v/1000)
		case v < 10:
			// A gateway on the same switch answers in tenths of a
			// millisecond, and rounding that to "0ms" loses the measurement.
			return fmt.Sprintf("%.1fms", v)
		}
		return fmt.Sprintf("%.0fms", v)
	case "dBm", "dB", "%":
		return fmt.Sprintf("%.0f%s", v, unit)
	case "Mbps":
		return fmt.Sprintf("%.1fM", v)
	case "MOS":
		return fmt.Sprintf("%.2f", v)
	case "days":
		return fmt.Sprintf("%.0fd", v)
	}
	return fmt.Sprintf("%.2f%s", v, unit)
}

// Millis is the conventional value for a timing measurement.
func Millis(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// RadioState is what the sensor's own radio saw during a pass.
type RadioState struct {
	SSID        string  `json:"ssid,omitempty"`
	BSSID       string  `json:"bssid,omitempty"`
	Channel     int     `json:"channel,omitempty"`
	Band        string  `json:"band,omitempty"`
	Width       string  `json:"width,omitempty"`
	RSSI        int     `json:"rssi,omitempty"`
	Noise       int     `json:"noise,omitempty"`
	SNR         int     `json:"snr,omitempty"`
	TxRate      float64 `json:"tx_rate,omitempty"`
	Security    string  `json:"security,omitempty"`
	Neighbours  int     `json:"neighbours,omitempty"`
	CoChannel   int     `json:"co_channel,omitempty"`
	Overlapping int     `json:"overlapping,omitempty"`
	Utilisation float64 `json:"utilisation,omitempty"`
	RoamTargets int     `json:"roam_targets,omitempty"`
}

// Lease is what DHCP handed the sensor, kept beside the results because half
// of what looks like a DNS or gateway fault is a scope handing out the wrong
// thing.
type Lease struct {
	Address  string   `json:"address,omitempty"`
	Mask     string   `json:"mask,omitempty"`
	Router   string   `json:"router,omitempty"`
	DNS      []string `json:"dns,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Server   string   `json:"server,omitempty"`
	LeaseSec int      `json:"lease_seconds,omitempty"`
	Offers   []string `json:"offers,omitempty"`
}

// SuiteResult is one pass over one network: everything the sensor learned,
// in one record that can be stored, sent upstream, exported or drawn.
type SuiteResult struct {
	Sensor       string          `json:"sensor"`
	Site         string          `json:"site,omitempty"`
	Group        string          `json:"group,omitempty"`
	Network      string          `json:"network"`
	Kind         string          `json:"kind"`
	Interface    string          `json:"interface,omitempty"`
	Start        time.Time       `json:"start"`
	Duration     time.Duration   `json:"duration_ns"`
	Measurements []Measurement   `json:"measurements"`
	Scores       map[Service]int `json:"scores"`
	Overall      int             `json:"overall"`
	Radio        *RadioState     `json:"radio,omitempty"`
	Lease        *Lease          `json:"lease,omitempty"`
	Neighbour    string          `json:"switch,omitempty"` // LLDP/CDP summary
	Portal       string          `json:"captive_portal,omitempty"`
	Issues       []Issue         `json:"issues,omitempty"`
	Aborted      string          `json:"aborted,omitempty"`
}

// Status is the worst thing that happened in the pass.
func (r SuiteResult) Status() Status {
	worst := StatusOK
	for _, m := range r.Measurements {
		if m.Status.Worse(worst) {
			worst = m.Status
		}
	}
	return worst
}

// Failures lists the measurements that failed, worst service first, which is
// the order an operator should read them in.
func (r SuiteResult) Failures() []Measurement {
	var out []Measurement
	for _, m := range r.Measurements {
		if m.Failed() {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return serviceRank(out[i].Service) < serviceRank(out[j].Service)
	})
	return out
}

func serviceRank(s Service) int {
	for i, v := range ServiceOrder {
		if v == s {
			return i
		}
	}
	return len(ServiceOrder)
}

// Add appends a measurement, stamping it with the time it was taken.
func (r *SuiteResult) Add(m Measurement) {
	if m.At.IsZero() {
		m.At = time.Now()
	}
	r.Measurements = append(r.Measurements, m)
}
