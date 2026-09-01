package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreacoppini/crossbreeder/sensor/l2"
	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

// fakeRadio is a scripted wpa_supplicant: it returns whatever association the
// test wants, so the ordering, the gating and the reporting can be exercised
// without a radio.
type fakeRadio struct {
	assoc      wifi.Association
	bsses      []wifi.BSS
	roamTook   time.Duration
	roamErr    error
	scans      int
	roams      int
	closed     bool
	disconnect bool
}

func (f *fakeRadio) Connect(context.Context, wifi.Profile, time.Duration) wifi.Association {
	return f.assoc
}
func (f *fakeRadio) Scan(context.Context, time.Duration) ([]wifi.BSS, error) {
	f.scans++
	return f.bsses, nil
}
func (f *fakeRadio) SignalPoll() (wifi.Signal, error) { return f.assoc.Signal, nil }
func (f *fakeRadio) Roam(_ context.Context, bssid string, _ time.Duration) (time.Duration, string, error) {
	f.roams++
	return f.roamTook, bssid, f.roamErr
}
func (f *fakeRadio) Disconnect() error { f.disconnect = true; return nil }
func (f *fakeRadio) Close() error      { f.closed = true; return nil }

// dhcpScope answers a DISCOVER and a REQUEST on loopback, the way a scope
// would, so the DHCP leg of the run is real rather than mocked out.
func dhcpScope(t *testing.T, silent bool) func(string) (net.PacketConn, net.Addr, error) {
	t.Helper()
	srv, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := srv.ReadFrom(buf)
			if err != nil {
				return
			}
			if silent || n < 240 {
				continue
			}
			srv.WriteTo(bootpReply(buf[:n]), from)
		}
	}()
	return func(string) (net.PacketConn, net.Addr, error) {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			return nil, nil, err
		}
		return conn, srv.LocalAddr(), nil
	}
}

// bootpReply builds an OFFER or an ACK for whatever was asked, by hand, so the
// sensor's own encoder is not being checked against itself.
func bootpReply(req []byte) []byte {
	msgType := byte(0)
	for i := 240; i+2 < len(req); {
		code, length := req[i], int(req[i+1])
		if code == 255 {
			break
		}
		if code == 0 {
			i++
			continue
		}
		if code == 53 && length == 1 {
			msgType = req[i+2]
		}
		i += 2 + length
	}
	reply := byte(2) // OFFER
	if msgType == 3 {
		reply = 5 // ACK
	}

	out := make([]byte, 240)
	out[0], out[1], out[2] = 2, 1, 6
	copy(out[4:8], req[4:8])     // transaction id
	copy(out[28:34], req[28:34]) // client MAC
	copy(out[16:20], net.ParseIP("10.20.30.55").To4())
	copy(out[236:240], []byte{99, 130, 83, 99})

	out = append(out, 53, 1, reply)
	out = append(out, 54, 4)
	out = append(out, net.ParseIP("10.20.30.1").To4()...)
	out = append(out, 1, 4)
	out = append(out, net.ParseIP("255.255.255.0").To4()...)
	out = append(out, 3, 4)
	out = append(out, net.ParseIP("10.20.30.1").To4()...)
	out = append(out, 6, 4)
	out = append(out, net.ParseIP("127.0.0.1").To4()...)
	out = append(out, 51, 4, 0, 0, 0x0e, 0x10) // 3600s
	return append(out, 255)
}

// dnsOn answers every A query with one address, so the DNS leg is real too.
func dnsOn(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			q := buf[:n]
			end := 12
			for end < len(q) && q[end] != 0 {
				end += int(q[end]) + 1
			}
			end += 5
			if end > len(q) {
				continue
			}
			out := append([]byte(nil), q[:end]...)
			binary.BigEndian.PutUint16(out[2:], 0x8180)
			binary.BigEndian.PutUint16(out[6:], 1)
			rr := make([]byte, 12)
			binary.BigEndian.PutUint16(rr[0:], 0xc00c)
			binary.BigEndian.PutUint16(rr[2:], 1)
			binary.BigEndian.PutUint16(rr[4:], 1)
			binary.BigEndian.PutUint32(rr[6:], 60)
			binary.BigEndian.PutUint16(rr[10:], 4)
			out = append(out, rr...)
			out = append(out, net.ParseIP("203.0.113.10").To4()...)
			pc.WriteTo(out, from)
		}
	}()
	return pc.LocalAddr().String()
}

func healthyRadio() *fakeRadio {
	return &fakeRadio{
		assoc: wifi.Association{
			SSID: "Corp", BSSID: "b8:27:eb:aa:bb:02", Freq: 5180, Channel: 36, Band: "5 GHz",
			Security: "WPA2-Enterprise", Scan: 300 * time.Millisecond, Auth: 40 * time.Millisecond,
			EAP: 250 * time.Millisecond, Key: 30 * time.Millisecond, Total: 900 * time.Millisecond,
			Signal: wifi.Signal{RSSI: -58, Noise: -95, SNR: 37, Freq: 5180, Channel: 36},
		},
		bsses: []wifi.BSS{
			{BSSID: "b8:27:eb:aa:bb:02", SSID: "Corp", Freq: 5180, Channel: 36, Band: "5 GHz", Signal: -58},
			{BSSID: "b8:27:eb:aa:bb:05", SSID: "Corp", Freq: 5500, Channel: 100, Band: "5 GHz", Signal: -66},
		},
		roamTook: 120 * time.Millisecond,
	}
}

func testRunner(t *testing.T, radio *fakeRadio, opendhcp func(string) (net.PacketConn, net.Addr, error)) (*Runner, *Config) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Sensor.Name = "lobby-1"
	cfg.Networks = nil
	deps := Deps{
		DialRadio: func(string, string) (RadioLink, error) {
			if radio == nil {
				return nil, errors.New("no radio on this sensor")
			}
			return radio, nil
		},
		OpenDHCP: opendhcp,
		Ping: func(_ context.Context, host string, _ time.Duration) (time.Duration, error) {
			if host == "192.0.2.1" {
				return 0, errors.New("no answer")
			}
			return 3 * time.Millisecond, nil
		},
		Gateway: func(string) (net.IP, error) { return net.ParseIP("10.20.30.1"), nil },
		Address: func(string) (net.IP, error) { return net.ParseIP("10.20.30.55"), nil },
		Now:     time.Now,
	}
	runner := NewRunner(cfg, deps, nil)
	// The real deadlines are seconds long; a test that waits them out teaches
	// nothing and costs a minute.
	runner.dhcpTimeout = 300 * time.Millisecond
	runner.dnsTimeout = time.Second
	runner.webTimeout = 3 * time.Second
	runner.discoveryWindow = 200 * time.Millisecond
	return runner, &cfg
}

func find(r SuiteResult, test string) (Measurement, bool) {
	for _, m := range r.Measurements {
		if m.Test == test || strings.HasPrefix(m.Test, test) {
			return m, true
		}
	}
	return Measurement{}, false
}

func TestRunWirelessNetworkEndToEnd(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("intranet"))
	}))
	defer web.Close()
	resolver := dnsOn(t)
	radio := healthyRadio()
	runner, cfg := testRunner(t, radio, dhcpScope(t, false))

	yes := true
	network := Network{
		Name: "Corp", Kind: "wifi",
		Profile: wifi.Profile{SSID: "Corp", EAP: "PEAP", Identity: "sensor", Password: "pw"},
		Tests: TestPlan{
			DHCP: &yes, Gateway: &yes, Roaming: &yes,
			DNS:      []DNSTarget{{Query: "intranet.example.com", Server: resolver}},
			Internet: []string{"1.1.1.1"},
			Web:      []WebTarget{{Name: "intranet", URL: web.URL, ExpectBody: "intranet"}},
		},
	}
	runner.cfg = *cfg

	res := runner.Run(context.Background(), network)

	if res.Aborted != "" {
		t.Fatalf("the pass aborted: %s", res.Aborted)
	}
	for _, want := range []string{"association", "802.1X authentication", "signal", "DHCP", "gateway",
		"DNS intranet.example.com", "reach 1.1.1.1", "intranet", "roaming"} {
		if _, ok := find(res, want); !ok {
			t.Errorf("no measurement for %q", want)
		}
	}
	if res.Status() != StatusOK {
		t.Errorf("a healthy network reported %s: %+v", res.Status(), res.Failures())
	}
	if res.Overall != 100 {
		t.Errorf("overall = %d", res.Overall)
	}
	if res.Radio == nil || res.Radio.RSSI != -58 || res.Radio.RoamTargets != 1 {
		t.Errorf("radio = %+v", res.Radio)
	}
	if res.Lease == nil || res.Lease.Address != "10.20.30.55" || len(res.Lease.DNS) != 1 {
		t.Errorf("lease = %+v", res.Lease)
	}
	if m, _ := find(res, "roaming"); m.Status != StatusOK || m.Value == 0 {
		t.Errorf("roaming = %+v", m)
	}
	if !radio.closed {
		t.Error("the radio was left open")
	}
	if len(res.Issues) != 0 {
		t.Errorf("a healthy pass raised issues: %+v", res.Issues)
	}
}

// A network the sensor cannot get onto has nothing else worth testing, and
// the report has to say that rather than listing nine consequential failures.
func TestRunStopsWhenTheRadioCannotAssociate(t *testing.T) {
	radio := healthyRadio()
	radio.assoc = wifi.Association{
		SSID: "Corp", Failure: "the passphrase was rejected",
		Err:   errors.New("could not join Corp: the passphrase was rejected"),
		Total: 2 * time.Second,
	}
	runner, cfg := testRunner(t, radio, dhcpScope(t, false))
	runner.cfg = *cfg

	yes := true
	res := runner.Run(context.Background(), Network{
		Name: "Corp", Kind: "wifi", Profile: wifi.Profile{SSID: "Corp", PSK: "passphrase"},
		Tests: TestPlan{DHCP: &yes, Gateway: &yes, Internet: []string{"1.1.1.1"}},
	})

	if res.Aborted == "" {
		t.Fatal("the pass carried on after the association failed")
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("tests ran anyway: %+v", res.Measurements)
	}
	if res.Overall != 0 {
		t.Errorf("overall = %d", res.Overall)
	}
	if len(res.Issues) != 1 || !res.Issues[0].RootCause {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Detail, "passphrase") {
		t.Errorf("the reason did not survive: %q", res.Issues[0].Detail)
	}
}

// Without an address, DNS and everything above it cannot be tested. They must
// be reported as not attempted, not as failures of their own.
func TestRunSkipsTheLayersAboveAFailedDHCP(t *testing.T) {
	runner, cfg := testRunner(t, nil, dhcpScope(t, true))
	runner.cfg = *cfg
	yes := true

	res := runner.Run(context.Background(), Network{
		Name: "Wired", Kind: "wired",
		Tests: TestPlan{
			DHCP: &yes, Gateway: &yes,
			DNS:      []DNSTarget{{Query: "example.com"}},
			Internet: []string{"1.1.1.1"},
			Apps:     []string{"Internet"},
		},
	})

	dhcp, ok := find(res, "DHCP")
	if !ok || dhcp.Status != StatusFail {
		t.Fatalf("DHCP = %+v", dhcp)
	}
	skipped := 0
	for _, m := range res.Measurements {
		if m.Status == StatusSkipped {
			skipped++
			if !strings.Contains(m.Detail, "no address") {
				t.Errorf("%s says %q", m.Test, m.Detail)
			}
		}
		if m.Service == ServiceDNS && m.Status == StatusFail {
			t.Error("DNS was reported as failing when it was never asked")
		}
	}
	if skipped == 0 {
		t.Error("nothing was recorded as skipped")
	}
	// One issue, about DHCP, and it is the root cause.
	if len(res.Issues) != 1 || res.Issues[0].Service != ServiceDHCP {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if res.Scores[ServiceDNS] != 0 {
		if _, scored := res.Scores[ServiceDNS]; scored {
			t.Error("a skipped service was scored")
		}
	}
}

func TestRunReportsASlowResolverAsAWarningNotAFailure(t *testing.T) {
	resolver := slowResolver(t, 300*time.Millisecond)
	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	cfg.Thresholds.DNSWarn = Duration(100 * time.Millisecond)
	cfg.Thresholds.DNSFail = Duration(2 * time.Second)
	runner.cfg = *cfg

	res := runner.Run(context.Background(), Network{
		Name: "Wired", Kind: "wired",
		Tests: TestPlan{DNS: []DNSTarget{{Query: "example.com", Server: resolver}}},
	})
	m, ok := find(res, "DNS example.com")
	if !ok {
		t.Fatal("no DNS measurement")
	}
	if m.Status != StatusWarn {
		t.Fatalf("a 300ms answer was judged %s", m.Status)
	}
	if len(res.Issues) != 1 || res.Issues[0].Severity != SeverityWarning {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Title, "slow") {
		t.Errorf("title = %q", res.Issues[0].Title)
	}
}

func slowResolver(t *testing.T, delay time.Duration) string {
	t.Helper()
	addr := dnsOn(t)
	// Wrap the working resolver in a relay that holds each answer back.
	front, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { front.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := front.ReadFrom(buf)
			if err != nil {
				return
			}
			query := append([]byte(nil), buf[:n]...)
			go func() {
				time.Sleep(delay)
				back, err := net.Dial("udp", addr)
				if err != nil {
					return
				}
				defer back.Close()
				back.Write(query)
				reply := make([]byte, 1500)
				back.SetReadDeadline(time.Now().Add(2 * time.Second))
				m, err := back.Read(reply)
				if err != nil {
					return
				}
				front.WriteTo(reply[:m], from)
			}()
		}
	}()
	return front.LocalAddr().String()
}

func TestRunRecordsTheSwitchPortOnAWiredNetwork(t *testing.T) {
	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	runner.deps.Discover = func(context.Context, string, time.Duration) ([]l2.Neighbour, error) {
		return []l2.Neighbour{{
			Protocol: "LLDP", SystemName: "sw-reception-1", PortDesc: "Gi1/0/24",
			VLAN: 100, MgmtAddr: "10.20.0.9",
		}}, nil
	}
	runner.cfg = *cfg
	yes := true

	res := runner.Run(context.Background(), Network{
		Name: "Wired", Kind: "wired", Tests: TestPlan{Discovery: &yes},
	})
	m, ok := find(res, "switch port")
	if !ok || m.Status != StatusOK {
		t.Fatalf("switch port = %+v", m)
	}
	if m.Extra["vlan"] != "100" || m.Extra["switch"] != "sw-reception-1" {
		t.Errorf("extra = %v", m.Extra)
	}
	if !strings.Contains(res.Neighbour, "sw-reception-1") {
		t.Errorf("neighbour = %q", res.Neighbour)
	}
}

func TestRunReportsAnEmptySwitchPort(t *testing.T) {
	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	runner.deps.Discover = func(context.Context, string, time.Duration) ([]l2.Neighbour, error) {
		return nil, nil
	}
	runner.cfg = *cfg
	yes := true
	res := runner.Run(context.Background(), Network{
		Name: "Wired", Kind: "wired", Tests: TestPlan{Discovery: &yes},
	})
	if m, _ := find(res, "switch port"); m.Status != StatusWarn {
		t.Fatalf("a port with nothing advertising on it = %+v", m)
	}
}

// A rate test moves real traffic, so it must not run on every pass.
func TestThroughputRunsOnItsOwnSchedule(t *testing.T) {
	served := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.Write(make([]byte, 64<<10))
	}))
	defer srv.Close()

	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	runner.cfg = *cfg
	network := Network{
		Name: "Wired", Kind: "wired",
		Tests: TestPlan{Throughput: &ThroughputTarget{
			Mode: "http", URL: srv.URL, Duration: Duration(200 * time.Millisecond),
			Every: Duration(time.Hour), ExpectMbps: 0,
		}},
	}
	first := runner.Run(context.Background(), network)
	if _, ok := find(first, "throughput"); !ok {
		t.Fatal("the first pass did not measure throughput")
	}
	second := runner.Run(context.Background(), network)
	if _, ok := find(second, "throughput"); ok {
		t.Error("the rate test ran again within its interval")
	}
}

func TestRunWithoutARadioReportsTheReason(t *testing.T) {
	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	runner.cfg = *cfg
	res := runner.Run(context.Background(), Network{
		Name: "Corp", Kind: "wifi", Profile: wifi.Profile{SSID: "Corp", PSK: "passphrase"},
	})
	if res.Aborted == "" || !strings.Contains(res.Aborted, "no radio") {
		t.Fatalf("aborted = %q", res.Aborted)
	}
}

func TestRunChecksPlainTCPServices(t *testing.T) {
	// A service that is listening, and one that is not.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no TCP loopback: %v", err)
	}
	defer ln.Close()

	runner, cfg := testRunner(t, nil, dhcpScope(t, false))
	runner.cfg = *cfg
	res := runner.Run(context.Background(), Network{
		Name: "Wired", Kind: "wired",
		Tests: TestPlan{Ports: []PortTarget{
			{Name: "File server", Address: ln.Addr().String()},
			{Name: "Print server", Address: "127.0.0.1:9"},
		}},
	})
	if m, ok := find(res, "File server"); !ok || m.Status != StatusOK {
		t.Errorf("a listening service = %+v", m)
	}
	if m, ok := find(res, "Print server"); !ok || m.Status != StatusFail {
		t.Errorf("a closed port = %+v", m)
	}
}
