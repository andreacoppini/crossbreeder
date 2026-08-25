package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
	"golang.org/x/crypto/ssh"
)

// restartableAP is a fake Ruckus AP that can be taken offline and brought back
// on a different firmware version — which is exactly the sequence the watch
// phase exists to follow.
type restartableAP struct {
	port   string
	conf   *ssh.ServerConfig
	mu     sync.Mutex
	ln     net.Listener
	verStr string
}

func newRestartableAP(t *testing.T, version string) *restartableAP {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	conf := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
	}
	conf.AddHostKey(signer)

	a := &restartableAP{conf: conf, verStr: version}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, a.port, _ = net.SplitHostPort(ln.Addr().String())
	a.ln = ln
	go a.accept(ln)
	t.Cleanup(a.stop)
	return a
}

func (a *restartableAP) setVersion(v string) {
	a.mu.Lock()
	a.verStr = v
	a.mu.Unlock()
}

func (a *restartableAP) version() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.verStr
}

func (a *restartableAP) stop() {
	a.mu.Lock()
	ln := a.ln
	a.ln = nil
	a.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
}

func (a *restartableAP) start(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:"+a.port)
	if err != nil {
		t.Fatalf("could not come back up on %s: %v", a.port, err)
	}
	a.mu.Lock()
	a.ln = ln
	a.mu.Unlock()
	go a.accept(ln)
}

func (a *restartableAP) accept(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go a.serve(c)
	}
}

func (a *restartableAP) serve(c net.Conn) {
	defer c.Close()
	sc, chans, reqs, err := ssh.NewServerConn(c, a.conf)
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
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				if r.WantReply {
					_ = r.Reply(r.Type == "pty-req" || r.Type == "shell", nil)
				}
			}
		}()
		a.cli(ch)
		_ = ch.Close()
	}
}

func (a *restartableAP) cli(ch ssh.Channel) {
	in := bufio.NewReader(ch)
	say := func(s string) { _, _ = ch.Write([]byte(s)) }
	read := func() bool {
		_, err := in.ReadString('\n')
		return err == nil
	}
	say("\r\nPlease login: ")
	if !read() {
		return
	}
	say("password : ")
	if !read() {
		return
	}
	say("\r\nrkscli: ")
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimRight(line, "\r\n") {
		case "get version":
			say(fmt.Sprintf("Ruckus R550 Multimedia Hotzone Wireless AP\r\nVersion: %s\r\nOK\r\n", a.version()))
		case "get boarddata":
			say("Customer ID: 0, base DC:AE:EB:1D:2A:20\r\nOK\r\n")
		default:
			say("OK\r\n")
		}
		say("rkscli: ")
	}
}

// TestWatchFollowsARebootAndUpgrade is the whole feature: an AP that drops off
// must read as rebooting rather than failed, and coming back on a new version
// must be recognised as the upgrade landing.
func TestWatchFollowsARebootAndUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fake := newRestartableAP(t, "7.1.1.0.6250")

	opt := options{
		// TCP probing, because every address on loopback answers ICMP and the
		// point here is to model an AP that has gone away.
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 300 * time.Millisecond, pingConcurrency: 8,
		concurrency: 4, watchEnabled: true, watchInterval: 300 * time.Millisecond,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		ConnectTimeout: 2 * time.Second, DialogTimeout: 2 * time.Second, Deadline: 20 * time.Second,
	}
	results := []ap.Result{{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1.1.0.6250"}}

	var mu sync.Mutex
	var notes []string
	emit := func(e Event) {
		if e.Kind != EvResult || e.Result == nil {
			return
		}
		mu.Lock()
		notes = append(notes, e.Result.Note)
		mu.Unlock()
	}
	sawNote := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, n := range notes {
			if strings.Contains(n, want) {
				return true
			}
		}
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	done := make(chan map[string]ap.Result, 1)
	go func() { done <- watchAPs(ctx, opt, cfg, results, emit) }()

	// The AP goes away, as it would while rebooting into the new image.
	time.Sleep(600 * time.Millisecond)
	fake.stop()

	waitFor(t, 6*time.Second, func() bool { return sawNote(NoteRebooting) },
		"an AP that stopped answering was never reported as Rebooting")

	// It comes back on the new firmware.
	fake.setVersion("7.2.0.620.5111")
	fake.start(t)

	waitFor(t, 10*time.Second, func() bool { return sawNote("Upgraded from 7.1.1.0.6250") },
		"the new firmware version was never recognised as an upgrade")

	// It keeps scanning until stopped, so stopping is what ends it.
	cancel()
	select {
	case updates := <-done:
		u := updates["127.0.0.1"]
		if u.Firmware != "7.2.0.620.5111" {
			t.Errorf("final firmware = %q, want the new version", u.Firmware)
		}
		if !strings.Contains(u.Note, "Upgraded") {
			t.Errorf("final note = %q", u.Note)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not stop when the context was cancelled")
	}
}

// An AP that never changes version must not be declared upgraded, and the
// optional cap must end the loop rather than leaving it running.
func TestWatchStopsAtItsDeadlineWithoutAnUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fake := newRestartableAP(t, "7.1.1.0.6250")

	opt := options{
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 300 * time.Millisecond, pingConcurrency: 8,
		concurrency: 4, watchEnabled: true, watch: 1200 * time.Millisecond, watchInterval: 300 * time.Millisecond,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		ConnectTimeout: 2 * time.Second, DialogTimeout: 2 * time.Second, Deadline: 20 * time.Second,
	}
	results := []ap.Result{{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1.1.0.6250"}}

	start := time.Now()
	updates := watchAPs(context.Background(), opt, cfg, results, func(Event) {})
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Errorf("watch ran for %v, well past its %v deadline", elapsed, opt.watch)
	}
	if u, ok := updates["127.0.0.1"]; ok && strings.Contains(u.Note, "Upgraded") {
		t.Errorf("unchanged firmware reported as upgraded: %q", u.Note)
	}
}

// An address that never answered has nothing to re-scan. One that answered but
// refused the login is still worth pinging — it can still go away and come
// back — so it stays in the loop even though its version cannot be re-read.
func TestWatchSkipsAddressesThatNeverAnswered(t *testing.T) {
	opt := options{watchEnabled: true, watch: 400 * time.Millisecond, watchInterval: 100 * time.Millisecond,
		probe: "tcp", sshPort: "1", pingTimeout: 100 * time.Millisecond, pingConcurrency: 4, concurrency: 2}

	// Nothing reachable at all: the loop must not even start.
	watchAPs(context.Background(), opt, ap.Config{}, []ap.Result{
		{IP: "10.0.0.1", Status: "No ping reply"},
	}, func(Event) { t.Error("an address that never answered should not be watched") })

	// One that answered but could not be logged into is still followed.
	var mu sync.Mutex
	sawPhase := false
	watchAPs(context.Background(), opt, ap.Config{}, []ap.Result{
		{IP: "10.0.0.2", Status: "Login Failed", Reachable: true},
	}, func(e Event) {
		if e.Kind == EvPhase {
			mu.Lock()
			sawPhase = true
			mu.Unlock()
		}
	})
	mu.Lock()
	defer mu.Unlock()
	if !sawPhase {
		t.Error("an AP that answered the first sweep should still be re-scanned")
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(msg)
}

// Watching now applies to any run: the first pass does whatever was asked and
// every pass after that only scans, so an inventory-only run is re-scanned too.
func TestWatchReScansEvenAnInventoryOnlyRun(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fake := newRestartableAP(t, "7.1.1.0.6250")
	opt := options{
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 300 * time.Millisecond, pingConcurrency: 8,
		concurrency: 4, watchEnabled: true, watchInterval: 200 * time.Millisecond,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		ConnectTimeout: 2 * time.Second, DialogTimeout: 2 * time.Second, Deadline: 20 * time.Second,
	}
	results := []ap.Result{{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1.1.0.6250"}}

	var mu sync.Mutex
	passes := 0
	emit := func(e Event) {
		if e.Kind == EvProgress && e.Phase == "watch" {
			mu.Lock()
			passes++
			mu.Unlock()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watchAPs(ctx, opt, cfg, results, emit); close(done) }()

	waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return passes >= 3
	}, "an inventory-only run was not re-scanned")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch ignored Stop")
	}
}
