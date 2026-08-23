package ap

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeAP is an in-process SSH server that speaks enough of the Ruckus CLI to
// exercise the whole session: transport auth, PTY, the AP's own login prompt,
// inventory output and the command loop.
type fakeAP struct {
	ln       net.Listener
	kind     Kind
	latency  time.Duration // stalls the login banner, standing in for a slow AP
	badLogin bool          // reject the first credential pair

	mu       sync.Mutex
	commands []string
	attempts int
}

func newFakeAP(t *testing.T, kind Kind, latency time.Duration, badLogin bool) *fakeAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAP{ln: ln, kind: kind, latency: latency, badLogin: badLogin}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	conf := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, nil // the AP does its real check at the CLI layer
		},
	}
	conf.AddHostKey(signer)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c, conf)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeAP) addr() (host, port string) {
	h, p, _ := net.SplitHostPort(f.ln.Addr().String())
	return h, p
}

func (f *fakeAP) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *fakeAP) serve(c net.Conn, conf *ssh.ServerConfig) {
	defer c.Close()
	sc, chans, reqs, err := ssh.NewServerConn(c, conf)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)

	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range chReqs {
				if r.WantReply {
					_ = r.Reply(r.Type == "pty-req" || r.Type == "shell", nil)
				}
			}
		}()
		f.cli(ch)
		_ = ch.Close()
	}
}

func (f *fakeAP) cli(ch ssh.Channel) {
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	in := bufio.NewReader(ch)
	say := func(s string) { _, _ = ch.Write([]byte(s)) }

	readLine := func() (string, bool) {
		l, err := in.ReadString('\n')
		if err != nil {
			return "", false
		}
		return strings.TrimRight(l, "\r\n"), true
	}

	for {
		say("\r\nPlease login: ")
		if _, ok := readLine(); !ok {
			return
		}
		say("password : ")
		if _, ok := readLine(); !ok {
			return
		}
		f.mu.Lock()
		f.attempts++
		reject := f.badLogin && f.attempts == 1
		f.mu.Unlock()
		if reject {
			say("\r\nLogin incorrect\r\n")
			continue
		}
		break
	}

	if f.kind == KindZoneFlex {
		f.zoneFlexLoop(in, say)
		return
	}
	f.unleashedLoop(in, say)
}

func (f *fakeAP) record(cmd string) {
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
}

func (f *fakeAP) zoneFlexLoop(in *bufio.Reader, say func(string)) {
	say("\r\nrkscli: ")
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		f.record(cmd)
		switch cmd {
		case "get version":
			say("Ruckus R720 Multimedia Hotzone Wireless AP\r\nVersion: 110.0.0.0.1347\r\nOK\r\n")
		case "get boarddata":
			say("Board Data:\r\nCustomer ID: 0, base 8C:0C:90:12:34:56\r\nOK\r\n")
		case "reboot":
			return
		default:
			say("OK\r\n")
		}
		say("rkscli: ")
	}
}

func (f *fakeAP) unleashedLoop(in *bufio.Reader, say func(string)) {
	prompt := "\r\nruckus> "
	say(prompt)
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		f.record(cmd)
		switch cmd {
		case "enable force":
			prompt = "\r\nruckus# "
		case "show sysinfo":
			say("Model= R610\r\nVersion= 200.7.10.202 Build 79\r\nMAC Address= 2c:c5:d3:aa:bb:cc\r\n")
		case "ap-mode":
			prompt = "\r\nruckus(ap-mode)# "
		case "reboot":
			return
		default:
			say("OK\r\n")
		}
		say(prompt)
	}
}

func testConfig() Config {
	return Config{
		Credentials:    []Credentials{{User: "admin", Password: "Ruckus123"}, {User: "super", Password: "sp-admin"}},
		Port:           "22",
		ConnectTimeout: 5 * time.Second,
		DialogTimeout:  5 * time.Second,
		Deadline:       30 * time.Second,
	}
}

func TestZoneFlexInventoryAndFirmware(t *testing.T) {
	f := newFakeAP(t, KindZoneFlex, 0, false)
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Actions = Actions{UpdateFirmware: true, CustomCommand: "set scg ip 10.0.0.5"}
	cfg.Firmware = Firmware{Proto: "http", Host: "10.0.0.9", Port: "8080", Filename: "%M_110.bl7"}

	r := Run(t.Context(), host, cfg)
	if r.Status != "Done" {
		t.Fatalf("status = %q, err = %q\ntranscript:\n%s", r.Status, r.Error, r.Transcript)
	}
	if r.Kind != KindZoneFlex {
		t.Errorf("kind = %q, want %q", r.Kind, KindZoneFlex)
	}
	if r.Model != "R720" || r.Firmware != "110.0.0.0.1347" || r.MAC != "8C:0C:90:12:34:56" {
		t.Errorf("inventory = %q/%q/%q", r.Model, r.Firmware, r.MAC)
	}

	got := strings.Join(f.seen(), "\n")
	// %M must have been expanded from the model we just discovered.
	if !strings.Contains(got, "fw set control R720_110.bl7") {
		t.Errorf("firmware filename not templated:\n%s", got)
	}
	for _, want := range []string{"fw auto disable", "fw set proto http", "fw set host 10.0.0.9", "fw update", "set scg ip 10.0.0.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing command %q in:\n%s", want, got)
		}
	}
}

func TestUnleashedInventory(t *testing.T) {
	f := newFakeAP(t, KindUnleashed, 0, false)
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Actions = Actions{UpdateFirmware: true}
	cfg.Firmware = Firmware{Proto: "tftp", Host: "10.0.0.9", Port: "69", Filename: "%M.img"}

	r := Run(t.Context(), host, cfg)
	if r.Status != "Done" {
		t.Fatalf("status = %q, err = %q\ntranscript:\n%s", r.Status, r.Error, r.Transcript)
	}
	if r.Kind != KindUnleashed {
		t.Errorf("kind = %q", r.Kind)
	}
	if r.Model != "R610" || r.Firmware != "200.7.10.202.79" || r.MAC != "2C:C5:D3:AA:BB:CC" {
		t.Errorf("inventory = %q/%q/%q", r.Model, r.Firmware, r.MAC)
	}
	if got := strings.Join(f.seen(), "\n"); !strings.Contains(got, "fw set control R610.img") {
		t.Errorf("firmware filename not templated:\n%s", got)
	}
}

// The original falls back to super/sp-admin when the configured pair is
// refused; that behaviour has to survive the port.
func TestFallbackToDefaultCredentials(t *testing.T) {
	f := newFakeAP(t, KindZoneFlex, 0, true)
	host, port := f.addr()
	cfg := testConfig()
	cfg.Port = port

	r := Run(t.Context(), host, cfg)
	if r.Status != "Done" {
		t.Fatalf("status = %q, err = %q\ntranscript:\n%s", r.Status, r.Error, r.Transcript)
	}
	if f.attempts != 2 {
		t.Errorf("login attempts = %d, want 2", f.attempts)
	}
}

func TestUnreachableHostIsNotFatal(t *testing.T) {
	// Port 1 on loopback refuses immediately.
	cfg := testConfig()
	cfg.Port = "1"
	cfg.ConnectTimeout = time.Second
	r := Run(t.Context(), "127.0.0.1", cfg)
	if r.Status != "Unreachable" {
		t.Fatalf("status = %q, want Unreachable", r.Status)
	}
}

// TestConcurrencyBeatsSerial is the headline claim, measured rather than
// asserted: with a per-AP stall of `stall`, serial execution costs
// n*stall while a pool of n costs roughly one stall.
func TestConcurrencyBeatsSerial(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	const n = 24
	const stall = 250 * time.Millisecond

	aps := make([]*fakeAP, n)
	for i := range aps {
		aps[i] = newFakeAP(t, KindZoneFlex, stall, false)
	}

	runAll := func(workers int) time.Duration {
		start := time.Now()
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		var failures int64
		var mu sync.Mutex
		for _, f := range aps {
			host, port := f.addr()
			cfg := testConfig()
			cfg.Port = port
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if r := Run(t.Context(), host, cfg); r.Status != "Done" {
					mu.Lock()
					failures++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if failures > 0 {
			t.Fatalf("%d/%d sessions failed at %d workers", failures, n, workers)
		}
		return time.Since(start)
	}

	serial := runAll(1)
	parallel := runAll(n)
	t.Logf("%d APs @ %v stall: serial %v, %d workers %v (%.1fx)",
		n, stall, serial.Round(time.Millisecond), n, parallel.Round(time.Millisecond),
		float64(serial)/float64(parallel))

	if parallel > serial/4 {
		t.Errorf("pool of %d took %v, expected well under a quarter of the serial %v", n, parallel, serial)
	}
}
