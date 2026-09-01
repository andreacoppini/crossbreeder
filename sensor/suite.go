package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
	"github.com/andreacoppini/crossbreeder/sensor/l2"
	"github.com/andreacoppini/crossbreeder/sensor/netprobe"
	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

// RadioLink is the part of wpa_supplicant the suite uses. It is an interface
// so the whole run can be exercised against a scripted radio, which is the
// only way to test the ordering and the gating without hardware.
type RadioLink interface {
	Connect(ctx context.Context, p wifi.Profile, timeout time.Duration) wifi.Association
	Scan(ctx context.Context, wait time.Duration) ([]wifi.BSS, error)
	SignalPoll() (wifi.Signal, error)
	Roam(ctx context.Context, bssid string, timeout time.Duration) (time.Duration, string, error)
	Disconnect() error
	Close() error
}

// Deps are the pieces of the outside world the suite touches. Everything that
// needs a radio, a raw socket or a network is reached through here.
type Deps struct {
	DialRadio func(dir, iface string) (RadioLink, error)
	OpenDHCP  func(iface string) (net.PacketConn, net.Addr, error)
	Ping      func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error)
	Discover  func(ctx context.Context, iface string, window time.Duration) ([]l2.Neighbour, error)
	Survey    func(ctx context.Context, iface string) ([]wifi.SurveyEntry, error)
	Gateway   func(iface string) (net.IP, error)
	Address   func(iface string) (net.IP, error)
	Now       func() time.Time
}

// DefaultDeps wires the suite to the real hardware.
func DefaultDeps() Deps {
	return Deps{
		DialRadio: func(dir, iface string) (RadioLink, error) { return wifi.Dial(dir, iface) },
		OpenDHCP:  netprobe.OpenDHCP,
		Ping: func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
			r := ap.Ping(ctx, host, timeout)
			if r.Err != nil {
				return r.RTT, r.Err
			}
			if !r.Alive {
				return r.RTT, fmt.Errorf("%s did not answer", host)
			}
			return r.RTT, nil
		},
		Discover: l2.Discover,
		Survey:   wifi.ChannelSurvey,
		Gateway:  netprobe.DefaultGateway,
		Address:  netprobe.IPv4Of,
		Now:      time.Now,
	}
}

// Per-test deadlines. They are constants rather than configuration because
// they bound the run, not the judgement: how long a test is allowed to take
// before it is abandoned is a different question from how long it may take
// before it is called slow, and only the second is a matter of local taste.
const (
	associationTimeout = 30 * time.Second
	dhcpTimeout        = 5 * time.Second
	dnsTimeout         = 3 * time.Second
	pingTimeout        = 2 * time.Second
	webTimeout         = 15 * time.Second
	discoveryWindow    = 70 * time.Second
)

// Runner performs one pass over one network.
type Runner struct {
	cfg  Config
	deps Deps
	log  func(format string, args ...any)
	// The deadlines above, held as fields so a test can shorten them without
	// waiting out a real DHCP timeout.
	assocTimeout, dhcpTimeout, dnsTimeout, pingTimeout, webTimeout, discoveryWindow time.Duration
	// lastThroughput remembers when a rate test last ran, since those move
	// real traffic and run on their own slower schedule.
	lastThroughput map[string]time.Time
}

// NewRunner builds a runner. A nil log discards.
func NewRunner(cfg Config, deps Deps, log func(string, ...any)) *Runner {
	if log == nil {
		log = func(string, ...any) {}
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Runner{
		cfg: cfg, deps: deps, log: log, lastThroughput: map[string]time.Time{},
		assocTimeout: associationTimeout, dhcpTimeout: dhcpTimeout, dnsTimeout: dnsTimeout,
		pingTimeout: pingTimeout, webTimeout: webTimeout, discoveryWindow: discoveryWindow,
	}
}

// Run performs a pass over one network, in the order the layers depend on one
// another: get on, get an address, reach the gateway, resolve a name, reach
// the internet, then the applications. When a layer fails, the ones above it
// are recorded as skipped rather than run — a DNS test with no address is not
// a DNS failure, and reporting it as one sends people to the wrong team.
func (r *Runner) Run(ctx context.Context, network Network) SuiteResult {
	start := r.deps.Now()
	iface := r.interfaceFor(network)
	res := SuiteResult{
		Sensor: r.cfg.Sensor.Name, Site: r.cfg.Sensor.Site, Group: r.cfg.Sensor.Group,
		Network: network.Name, Kind: network.Kind, Interface: iface, Start: start,
	}
	th := r.cfg.Thresholds

	var radio RadioLink
	if network.Wireless() {
		radio = r.associate(ctx, &res, network, iface)
		if radio != nil {
			defer radio.Close()
		}
		if res.Aborted != "" {
			return r.finish(&res, start)
		}
	}

	lease := r.runDHCP(ctx, &res, network, iface)
	if network.Tests.DHCP != nil && *network.Tests.DHCP && lease == nil {
		// Without an address nothing above this layer can be tested, and
		// pretending otherwise would bury the one finding that matters.
		r.skipAbove(&res, "no address was leased")
		return r.finish(&res, start)
	}

	r.runGateway(ctx, &res, network, iface, lease, th)
	resolvers := r.resolversFor(network, lease)
	r.runDNS(ctx, &res, network, resolvers, th)
	r.runInternet(ctx, &res, network, th)
	r.runPortal(ctx, &res, network)
	r.runWeb(ctx, &res, network, resolvers, th)
	r.runPorts(ctx, &res, network, th)
	r.runCertificates(ctx, &res, network, th)
	r.runTraceroute(ctx, &res, network)
	r.runVoIP(ctx, &res, network, th)
	r.runThroughput(ctx, &res, network)
	r.runDiscovery(ctx, &res, network, iface)
	if radio != nil {
		r.runRoaming(ctx, &res, network, radio, th)
	}
	return r.finish(&res, start)
}

func (r *Runner) finish(res *SuiteResult, start time.Time) SuiteResult {
	res.Duration = r.deps.Now().Sub(start)
	res.Scores, res.Overall = Score(res.Measurements)
	res.Issues = DetectIssues(*res)
	return *res
}

func (r *Runner) interfaceFor(n Network) string {
	if n.Interface != "" {
		return n.Interface
	}
	if n.Wireless() {
		return r.cfg.Sensor.WirelessInterface
	}
	return r.cfg.Sensor.WiredInterface
}

// associate gets the radio onto the network and records every phase of it.
func (r *Runner) associate(ctx context.Context, res *SuiteResult, n Network, iface string) RadioLink {
	th := r.cfg.Thresholds
	radio, err := r.deps.DialRadio(r.cfg.Sensor.CtrlDir, iface)
	if err != nil {
		res.Aborted = err.Error()
		res.Add(Measurement{
			Test: "association", Service: ServiceWireless, Target: n.Profile.SSID,
			Status: StatusFail, Error: err.Error(),
		})
		return nil
	}

	a := radio.Connect(ctx, n.Profile, r.assocTimeout)
	res.Add(Measurement{
		Test: "association", Service: ServiceWireless, Target: a.SSID,
		Status: statusOrFail(a.Err, judgeDuration(a.Total, th.AssociationWarn.D(), th.AssociationFail.D())),
		Value:  Millis(a.Total), Unit: "ms",
		Detail: associationDetail(a),
		Error:  errText(a.Err),
		Extra: map[string]string{
			"bssid": a.BSSID, "channel": strconv.Itoa(a.Channel), "band": a.Band,
			"scan_ms": fmt.Sprintf("%.0f", Millis(a.Scan)), "auth_ms": fmt.Sprintf("%.0f", Millis(a.Auth)),
			"key_ms": fmt.Sprintf("%.0f", Millis(a.Key)),
		},
	})
	if a.Err != nil {
		res.Aborted = a.Failure
		return radio
	}

	if a.EAP > 0 {
		res.Add(Measurement{
			Test: "802.1X authentication", Service: ServiceAuth, Target: n.Profile.Identity,
			Status: judgeDuration(a.EAP, th.EAPWarn.D(), th.AssociationFail.D()),
			Value:  Millis(a.EAP), Unit: "ms",
		})
	}

	radioInfo := &RadioState{
		SSID: a.SSID, BSSID: a.BSSID, Channel: a.Channel, Band: a.Band,
		Width: a.Signal.Width, RSSI: a.Signal.RSSI, Noise: a.Signal.Noise,
		SNR: a.Signal.SNR, TxRate: a.Signal.TxBitrate, Security: a.Security,
	}
	res.Radio = radioInfo

	if a.Signal.RSSI != 0 {
		res.Add(Measurement{
			Test: "signal", Service: ServiceWireless, Target: a.BSSID,
			Status: judgeAtLeast(float64(a.Signal.RSSI), float64(th.SignalWarn), float64(th.SignalFail)),
			Value:  float64(a.Signal.RSSI), Unit: "dBm",
			Detail: signalDetail(a.Signal),
		})
	}
	if a.Signal.SNR > 0 {
		res.Add(Measurement{
			Test: "signal to noise", Service: ServiceWireless, Target: a.BSSID,
			Status: judgeAtLeast(float64(a.Signal.SNR), float64(th.SNRWarn), float64(th.SNRWarn)/2),
			Value:  float64(a.Signal.SNR), Unit: "dB",
		})
	}

	// What else is on the air. This runs after the association so the scan
	// cannot be blamed for the association's timing.
	if bsses, err := radio.Scan(ctx, 4*time.Second); err == nil && len(bsses) > 0 {
		hood := wifi.Survey(bsses, wifi.BSS{BSSID: a.BSSID, SSID: a.SSID, Channel: a.Channel, Band: a.Band})
		radioInfo.Neighbours = hood.Total
		radioInfo.CoChannel = hood.CoChannel
		radioInfo.Overlapping = hood.Overlapping
		radioInfo.RoamTargets = len(hood.SameSSID)
		res.Add(Measurement{
			Test: "channel occupancy", Service: ServiceWireless,
			Target: "channel " + strconv.Itoa(a.Channel),
			Status: judgeAtMost(float64(hood.CoChannel+hood.Overlapping), 4, 8),
			Value:  float64(hood.CoChannel + hood.Overlapping), Unit: "",
			Detail: fmt.Sprintf("%d radios heard, %d on this channel, %d overlapping",
				hood.Total, hood.CoChannel, hood.Overlapping),
		})
	}
	if r.deps.Survey != nil {
		if entries, err := r.deps.Survey(ctx, iface); err == nil {
			for _, e := range entries {
				if !e.InUse || e.ActiveMs == 0 {
					continue
				}
				radioInfo.Utilisation = e.Utilisation()
				res.Add(Measurement{
					Test: "air time in use", Service: ServiceWireless,
					Target: "channel " + strconv.Itoa(e.Channel),
					Status: judgeAtMost(e.Utilisation(), th.UtilisationWarn, th.UtilisationFail),
					Value:  e.Utilisation(), Unit: "%",
				})
			}
		}
	}
	return radio
}

func associationDetail(a wifi.Association) string {
	if a.Err != nil {
		return a.Failure
	}
	parts := []string{fmt.Sprintf("%s on channel %d", a.BSSID, a.Channel)}
	if a.Security != "" {
		parts = append(parts, a.Security)
	}
	if a.EAP > 0 {
		parts = append(parts, fmt.Sprintf("EAP %.0fms", Millis(a.EAP)))
	}
	return strings.Join(parts, ", ")
}

func signalDetail(s wifi.Signal) string {
	if s.Noise != 0 {
		return fmt.Sprintf("noise %d dBm, SNR %d dB", s.Noise, s.SNR)
	}
	return ""
}

// runDHCP performs a full exchange and keeps the lease, which the tests above
// it are then run against.
func (r *Runner) runDHCP(ctx context.Context, res *SuiteResult, n Network, iface string) *Lease {
	if !on(n.Tests.DHCP, false) {
		return nil
	}
	th := r.cfg.Thresholds
	conn, server, err := r.deps.OpenDHCP(iface)
	if err != nil {
		res.Add(Measurement{
			Test: "DHCP", Service: ServiceDHCP, Target: iface,
			Status: StatusFail, Error: err.Error(),
		})
		return nil
	}
	defer conn.Close()

	client := &netprobe.DHCPClient{
		Conn: conn, Server: server, Hostname: r.cfg.Sensor.Name,
		Timeout: r.dhcpTimeout, Release: true,
	}
	out := client.Probe(ctx)
	status := statusOrFail(out.Err, judgeDuration(out.Total, th.DHCPWarn.D(), th.DHCPFail.D()))
	m := Measurement{
		Test: "DHCP", Service: ServiceDHCP, Target: iface, Status: status,
		Value: Millis(out.Total), Unit: "ms", Error: errText(out.Err),
	}
	if out.OK() {
		m.Detail = fmt.Sprintf("%s from %s, offer %.0fms, ack %.0fms",
			out.YourIP, out.ServerID, Millis(out.Offer), Millis(out.Ack))
	}
	res.Add(m)
	if !out.OK() {
		return nil
	}

	lease := &Lease{
		Address: out.YourIP.String(), Router: ipString(out.Router),
		Mask: ipString(out.SubnetMask), Domain: out.Domain,
		Server: ipString(out.ServerID), LeaseSec: int(out.Lease.Seconds()),
		Offers: out.Offers,
	}
	for _, d := range out.DNS {
		lease.DNS = append(lease.DNS, d.String())
	}
	res.Lease = lease

	// More than one server answering a DISCOVER on a network that should have
	// one scope is a rogue server, and it is the sort of thing nobody finds
	// until half the site cannot print.
	if len(out.Offers) > 1 {
		res.Add(Measurement{
			Test: "DHCP servers", Service: ServiceDHCP, Target: iface,
			Status: StatusWarn, Value: float64(len(out.Offers)),
			Detail: "more than one server offered an address: " + strings.Join(out.Offers, "; "),
		})
	}
	return lease
}

// skipAbove records the tests that were not attempted, so the pass says what
// it did not do rather than silently omitting it.
func (r *Runner) skipAbove(res *SuiteResult, why string) {
	for _, s := range []Service{ServiceGateway, ServiceDNS, ServiceInternet, ServiceApplications} {
		res.Add(Measurement{
			Test: string(s), Service: s, Status: StatusSkipped, Detail: "not attempted: " + why,
		})
	}
}

func (r *Runner) runGateway(ctx context.Context, res *SuiteResult, n Network, iface string, lease *Lease, th Thresholds) {
	if !on(n.Tests.Gateway, false) {
		return
	}
	gateway := ""
	if lease != nil {
		gateway = lease.Router
	}
	if gateway == "" && r.deps.Gateway != nil {
		if ip, err := r.deps.Gateway(iface); err == nil {
			gateway = ip.String()
		}
	}
	if gateway == "" {
		res.Add(Measurement{
			Test: "gateway", Service: ServiceGateway, Status: StatusFail,
			Error: "this network has no default gateway",
		})
		return
	}
	rtt, err := r.deps.Ping(ctx, gateway, r.pingTimeout)
	res.Add(Measurement{
		Test: "gateway", Service: ServiceGateway, Target: gateway,
		Status: statusOrFail(err, judgeDuration(rtt, th.GatewayWarn.D(), th.GatewayFail.D())),
		Value:  Millis(rtt), Unit: "ms", Error: errText(err),
	})
}

// resolversFor decides which DNS servers to test: the ones DHCP handed out,
// unless the test names its own. Testing the resolver the clients were given
// is the point — a sensor that always asks 8.8.8.8 tests Google's uptime.
func (r *Runner) resolversFor(n Network, lease *Lease) []string {
	if lease != nil && len(lease.DNS) > 0 {
		return lease.DNS
	}
	return nil
}

func (r *Runner) runDNS(ctx context.Context, res *SuiteResult, n Network, resolvers []string, th Thresholds) {
	for _, target := range n.Tests.DNS {
		servers := []string{target.Server}
		if target.Server == "" {
			servers = resolvers
			if len(servers) == 0 {
				servers = []string{""} // the system resolver
			}
		}
		for _, server := range servers {
			out := netprobe.Resolve(ctx, netprobe.DNSQuery{
				Server: server, Name: target.Query, Type: netprobe.DNSType(target.Type),
				Proto: target.Proto, Timeout: r.dnsTimeout, Expect: target.Expect,
			})
			name := "DNS " + target.Query
			if server != "" {
				name += " @" + server
			}
			status := judgeDuration(out.RTT, th.DNSWarn.D(), th.DNSFail.D())
			if !out.OK() {
				status = StatusFail
			}
			detail := strings.Join(out.Answers, ", ")
			if out.RCode != "" && out.RCode != "NOERROR" {
				detail = out.RCode
			}
			res.Add(Measurement{
				Test: name, Service: ServiceDNS, Target: server, Status: status,
				Value: Millis(out.RTT), Unit: "ms", Detail: detail, Error: errText(out.Err),
			})
		}
	}
}

func (r *Runner) runInternet(ctx context.Context, res *SuiteResult, n Network, th Thresholds) {
	for _, host := range n.Tests.Internet {
		rtt, err := r.deps.Ping(ctx, host, r.pingTimeout)
		res.Add(Measurement{
			Test: "reach " + host, Service: ServiceInternet, Target: host,
			Status: statusOrFail(err, judgeDuration(rtt, th.InternetWarn.D(), th.InternetFail.D())),
			Value:  Millis(rtt), Unit: "ms", Error: errText(err),
		})
	}
}

func (r *Runner) runPortal(ctx context.Context, res *SuiteResult, n Network) {
	if !on(n.Tests.CaptivePortal, false) {
		return
	}
	st := netprobe.DetectCaptivePortal(ctx, "", r.webTimeout)
	switch {
	case st.Err != nil:
		res.Add(Measurement{
			Test: "captive portal check", Service: ServiceInternet,
			Status: StatusFail, Error: st.Err.Error(),
		})
	case st.Detected:
		res.Portal = st.PortalURL
		detail := "this network is intercepting web traffic"
		if st.PortalURL != "" {
			detail += ": " + st.PortalURL
		}
		res.Add(Measurement{
			Test: "captive portal check", Service: ServiceInternet,
			Status: StatusWarn, Detail: detail, Value: float64(st.Status),
		})
	default:
		res.Add(Measurement{
			Test: "captive portal check", Service: ServiceInternet,
			Status: StatusOK, Detail: "no interception",
		})
	}
}

func (r *Runner) runWeb(ctx context.Context, res *SuiteResult, n Network, resolvers []string, th Thresholds) {
	targets := append([]WebTarget(nil), n.Tests.Web...)
	for _, name := range n.Tests.Apps {
		if app, ok := LookupApp(name); ok {
			targets = append(targets, app.Tests...)
		}
	}
	resolver := ""
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	for _, target := range targets {
		out := netprobe.Fetch(ctx, netprobe.WebTest{
			Name: target.Name, URL: target.URL, Timeout: r.webTimeout,
			ExpectStatus: target.ExpectStatus, ExpectBody: target.ExpectBody,
			Insecure: target.Insecure, Follow: target.Follow, Resolver: resolver,
		})
		name := target.Name
		if name == "" {
			name = target.URL
		}
		m := Measurement{
			Test: name, Service: ServiceApplications, Target: target.URL,
			Status: statusOrFail(out.Err, judgeDuration(out.Total, th.WebWarn.D(), th.WebFail.D())),
			Value:  Millis(out.Total), Unit: "ms", Error: errText(out.Err),
			Extra: map[string]string{
				"dns_ms": fmt.Sprintf("%.0f", Millis(out.DNS)), "connect_ms": fmt.Sprintf("%.0f", Millis(out.Connect)),
				"tls_ms": fmt.Sprintf("%.0f", Millis(out.TLS)), "ttfb_ms": fmt.Sprintf("%.0f", Millis(out.TTFB)),
				"status": strconv.Itoa(out.Status),
			},
		}
		if out.Err == nil {
			m.Detail = fmt.Sprintf("HTTP %d, DNS %.0fms, connect %.0fms, TLS %.0fms, first byte %.0fms",
				out.Status, Millis(out.DNS), Millis(out.Connect), Millis(out.TLS), Millis(out.TTFB))
		}
		res.Add(m)

		// A certificate about to expire on an application the site depends on
		// is a finding today, not an outage next month.
		if days := out.DaysToCertExpiry(); days != 0 && out.CertExpiry.After(time.Time{}) {
			if days <= th.CertWarnDays {
				res.Add(certMeasurement(name, target.URL, days, th))
			}
		}
	}
}

// runPorts checks the services that are not web pages — a print server, a
// file share, the application nobody remembers the port of until it stops
// answering.
func (r *Runner) runPorts(ctx context.Context, res *SuiteResult, n Network, th Thresholds) {
	for _, port := range n.Tests.Ports {
		out := netprobe.TCPConnect(ctx, port.Address, r.pingTimeout)
		name := port.Name
		if name == "" {
			name = "connect to " + port.Address
		}
		res.Add(Measurement{
			Test: name, Service: ServiceApplications, Target: port.Address,
			Status: statusOrFail(out.Err, judgeDuration(out.RTT, th.WebWarn.D(), th.WebFail.D())),
			Value:  Millis(out.RTT), Unit: "ms", Error: errText(out.Err),
		})
	}
}

func (r *Runner) runCertificates(ctx context.Context, res *SuiteResult, n Network, th Thresholds) {
	for _, hostport := range n.Tests.Certificates {
		cert, err := netprobe.CertificateExpiry(ctx, hostport, r.webTimeout)
		if err != nil {
			res.Add(Measurement{
				Test: "certificate " + hostport, Service: ServiceApplications,
				Target: hostport, Status: StatusFail, Error: err.Error(),
			})
			continue
		}
		days := int(time.Until(cert.NotAfter).Hours() / 24)
		m := certMeasurement("certificate "+hostport, hostport, days, th)
		m.Detail = fmt.Sprintf("%s, issued by %s, expires %s",
			cert.Subject.CommonName, cert.Issuer.CommonName, cert.NotAfter.Format("2 Jan 2006"))
		res.Add(m)
	}
}

func certMeasurement(name, target string, days int, th Thresholds) Measurement {
	status := StatusOK
	switch {
	case days <= th.CertFailDays:
		status = StatusFail
	case days <= th.CertWarnDays:
		status = StatusWarn
	}
	detail := fmt.Sprintf("%d days until the certificate expires", days)
	if days < 0 {
		detail = fmt.Sprintf("the certificate expired %d days ago", -days)
	}
	return Measurement{
		Test: name + " certificate", Service: ServiceApplications, Target: target,
		Status: status, Value: float64(days), Unit: "days", Detail: detail,
	}
}

func (r *Runner) runTraceroute(ctx context.Context, res *SuiteResult, n Network) {
	for _, target := range n.Tests.Traceroute {
		out := netprobe.Traceroute(ctx, target, 20, time.Second)
		if out.Err != nil {
			res.Add(Measurement{
				Test: "path to " + target, Service: ServiceInternet, Target: target,
				Status: StatusSkipped, Error: out.Err.Error(),
			})
			continue
		}
		var last netprobe.Hop
		silent := 0
		for _, h := range out.Hops {
			if h.Timeout {
				silent++
				continue
			}
			last = h
		}
		res.Add(Measurement{
			Test: "path to " + target, Service: ServiceInternet, Target: target,
			Status: StatusOK, Value: float64(len(out.Hops)), Unit: "",
			Detail: fmt.Sprintf("%d hops, %d silent, last %s at %.0fms",
				len(out.Hops), silent, last.Addr, Millis(last.RTT)),
			Extra: map[string]string{"path": renderPath(out.Hops)},
		})
	}
}

func renderPath(hops []netprobe.Hop) string {
	parts := make([]string, 0, len(hops))
	for _, h := range hops {
		if h.Timeout {
			parts = append(parts, "*")
			continue
		}
		label := h.Addr
		if h.Name != "" {
			label = h.Name
		}
		parts = append(parts, fmt.Sprintf("%s (%.0fms)", label, Millis(h.RTT)))
	}
	return strings.Join(parts, " → ")
}

func (r *Runner) runVoIP(ctx context.Context, res *SuiteResult, n Network, th Thresholds) {
	v := n.Tests.VoIP
	if v == nil {
		return
	}
	out := netprobe.RunVoIP(ctx, netprobe.VoIPTest{
		Reflector: v.Reflector, Packets: v.Packets, DSCP: v.DSCP, Codec: codecFor(v.Codec),
	})
	res.Add(Measurement{
		Test: "call quality", Service: ServiceVoice, Target: v.Reflector,
		Status: statusOrFail(out.Err, judgeAtLeast(out.MOS, th.MOSWarn, th.MOSFail)),
		Value:  out.MOS, Unit: "MOS", Error: errText(out.Err),
		Detail: fmt.Sprintf("%.1f%% loss, %.0fms round trip, %.0fms jitter",
			out.LossPct, Millis(out.RTT), Millis(out.Jitter)),
		Extra: map[string]string{
			"loss_pct":  fmt.Sprintf("%.2f", out.LossPct),
			"jitter_ms": fmt.Sprintf("%.2f", Millis(out.Jitter)),
			"rtt_ms":    fmt.Sprintf("%.2f", Millis(out.RTT)),
		},
	})
	if out.Err == nil {
		res.Add(Measurement{
			Test: "packet loss", Service: ServiceVoice, Target: v.Reflector,
			Status: judgeAtMost(out.LossPct, th.LossWarnPct, th.LossFailPct),
			Value:  out.LossPct, Unit: "%",
		})
		// A marking that does not survive the path means the network is
		// carrying voice as ordinary traffic, whatever the policy says.
		if v.DSCP != 0 && out.SeenDSCP >= 0 && !out.DSCPPreserved() {
			res.Add(Measurement{
				Test: "QoS marking", Service: ServiceVoice, Target: v.Reflector,
				Status: StatusWarn, Value: float64(out.SeenDSCP),
				Detail: fmt.Sprintf("sent DSCP %d, arrived as %d", out.SentDSCP, out.SeenDSCP),
			})
		}
	}
}

func codecFor(name string) netprobe.Codec {
	switch strings.ToUpper(strings.ReplaceAll(name, ".", "")) {
	case "G729":
		return netprobe.G729
	case "G722":
		return netprobe.G722
	}
	return netprobe.G711
}

func (r *Runner) runThroughput(ctx context.Context, res *SuiteResult, n Network) {
	t := n.Tests.Throughput
	if t == nil {
		return
	}
	// A rate test saturates the link it is measuring, so it runs on its own
	// schedule and never on every pass.
	every := t.Every.D()
	if every <= 0 {
		every = time.Hour
	}
	if last, ok := r.lastThroughput[n.Name]; ok && r.deps.Now().Sub(last) < every {
		return
	}
	r.lastThroughput[n.Name] = r.deps.Now()

	out := netprobe.RunThroughput(ctx, netprobe.ThroughputTest{
		Mode: netprobe.ThroughputMode(strings.ToLower(t.Mode)), URL: t.URL, Peer: t.Peer,
		Upload: t.Upload, Streams: t.Streams, Duration: t.Duration.D(),
	})
	status := StatusOK
	switch {
	case out.Err != nil:
		status = StatusFail
	case t.ExpectMbps > 0:
		status = judgeAtLeast(out.Mbps, t.ExpectMbps*0.8, t.ExpectMbps*0.5)
	}
	direction := "download"
	if t.Upload {
		direction = "upload"
	}
	res.Add(Measurement{
		Test: "throughput (" + direction + ")", Service: ServiceThroughput,
		Target: firstNonEmpty(t.Peer, t.URL), Status: status,
		Value: out.Mbps, Unit: "Mbps", Error: errText(out.Err),
		Detail: fmt.Sprintf("%.1f MiB in %.1fs over %d stream(s)",
			float64(out.Bytes)/(1<<20), out.Duration.Seconds(), out.Streams),
	})
}

func (r *Runner) runDiscovery(ctx context.Context, res *SuiteResult, n Network, iface string) {
	if !on(n.Tests.Discovery, false) || r.deps.Discover == nil {
		return
	}
	neighbours, err := r.deps.Discover(ctx, iface, r.discoveryWindow)
	if err != nil && len(neighbours) == 0 {
		res.Add(Measurement{
			Test: "switch port", Service: ServiceLAN, Target: iface,
			Status: StatusSkipped, Error: err.Error(),
		})
		return
	}
	if len(neighbours) == 0 {
		res.Add(Measurement{
			Test: "switch port", Service: ServiceLAN, Target: iface,
			Status: StatusWarn,
			Detail: "nothing advertised itself on this port in " + r.discoveryWindow.String(),
		})
		return
	}
	n0 := neighbours[0]
	res.Neighbour = n0.String()
	res.Add(Measurement{
		Test: "switch port", Service: ServiceLAN, Target: iface, Status: StatusOK,
		Value: float64(n0.VLAN), Detail: n0.String(),
		Extra: map[string]string{
			"protocol": n0.Protocol, "switch": n0.SystemName, "port": n0.PortDesc,
			"vlan": strconv.Itoa(n0.VLAN), "management": n0.MgmtAddr,
		},
	})
}

func (r *Runner) runRoaming(ctx context.Context, res *SuiteResult, n Network, radio RadioLink, th Thresholds) {
	if !on(n.Tests.Roaming, false) {
		return
	}
	if res.Radio == nil || res.Radio.RoamTargets == 0 {
		res.Add(Measurement{
			Test: "roaming", Service: ServiceWireless, Status: StatusSkipped,
			Detail: "no other radio carrying this SSID was heard",
		})
		return
	}
	bsses, err := radio.Scan(ctx, 2*time.Second)
	if err != nil {
		res.Add(Measurement{Test: "roaming", Service: ServiceWireless, Status: StatusSkipped, Error: err.Error()})
		return
	}
	hood := wifi.Survey(bsses, wifi.BSS{
		BSSID: res.Radio.BSSID, SSID: res.Radio.SSID, Channel: res.Radio.Channel, Band: res.Radio.Band,
	})
	if hood.Strongest == nil {
		res.Add(Measurement{
			Test: "roaming", Service: ServiceWireless, Status: StatusSkipped,
			Detail: "nowhere to roam to",
		})
		return
	}
	took, landed, err := radio.Roam(ctx, hood.Strongest.BSSID, 15*time.Second)
	res.Add(Measurement{
		Test: "roaming", Service: ServiceWireless, Target: hood.Strongest.BSSID,
		// A handover is a different order of magnitude from an association:
		// anything past a second is audible in a call.
		Status: statusOrFail(err, judgeDuration(took, time.Second, 5*time.Second)),
		Value:  Millis(took), Unit: "ms", Error: errText(err),
		Detail: roamDetail(landed, hood.Strongest.Signal),
	})
}

func roamDetail(landed string, signal int) string {
	if landed == "" {
		return ""
	}
	return fmt.Sprintf("moved to %s at %d dBm", landed, signal)
}

// statusOrFail returns fail when the test errored, and otherwise the timing
// judgement. Nothing that errored is ever anything but a failure.
func statusOrFail(err error, judged Status) Status {
	if err != nil {
		return StatusFail
	}
	return judged
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
