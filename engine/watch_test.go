package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	// stallBeforeLogin models an AP that is slow to answer, so a pass can be
	// made to take longer than the re-scan interval.
	stallBeforeLogin time.Duration
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
	a.mu.Lock()
	stall := a.stallBeforeLogin
	a.mu.Unlock()
	if stall > 0 {
		time.Sleep(stall)
	}
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

// Every listed address is re-scanned, so an AP that was already rebooting when
// Run was pressed still gets picked up. But "Rebooting" is only for an AP that
// was up when the run started — an address that never answered keeps whatever
// the first sweep said about it.
func TestOnlyAPsThatWereUpAreCalledRebooting(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	opt := options{
		watchEnabled: true, watch: 900 * time.Millisecond, watchInterval: 200 * time.Millisecond,
		probe: "tcp", sshPort: "1", // nothing is listening, so everything reads as down
		pingTimeout: 100 * time.Millisecond, pingConcurrency: 4, concurrency: 2,
	}

	var mu sync.Mutex
	notes := map[string]string{}
	emit := func(e Event) {
		if e.Kind == EvResult && e.Result != nil {
			mu.Lock()
			notes[e.Result.IP] = e.Result.Note
			mu.Unlock()
		}
	}

	watchAPs(context.Background(), opt, ap.Config{}, []ap.Result{
		{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1"},
		{IP: "127.0.0.2", Status: "No ping reply"},
	}, emit)

	mu.Lock()
	defer mu.Unlock()
	if notes["127.0.0.1"] != NoteRebooting {
		t.Errorf("an AP that was up and went away should read as %q, got %q", NoteRebooting, notes["127.0.0.1"])
	}
	if n, ok := notes["127.0.0.2"]; ok && n == NoteRebooting {
		t.Error("an address that never answered was called Rebooting")
	}
}

// The reported failure: with an image server up and an AP that never finishes
// downloading, the run used to sit in the download wait — for the full
// -serve-wait, 30 minutes by default — and the re-scan never started.
func TestReScanRunsWhileDownloadsAreStillOutstanding(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fake := newRestartableAP(t, "7.1.1.0.6250")

	dir := t.TempDir()
	big := make([]byte, 4<<20)
	if err := os.WriteFile(filepath.Join(dir, "fw.bl7"), big, 0o600); err != nil {
		t.Fatal(err)
	}

	opt := options{
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 300 * time.Millisecond, pingConcurrency: 8, concurrency: 4,
		watchEnabled: true, watchInterval: 300 * time.Millisecond,
		// Long enough that a blocking download wait would obviously hang the test.
		serveWait: 10 * time.Minute,
		serveDir:  dir, serveIP: "127.0.0.1", fw: true,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		Actions:        ap.Actions{UpdateFirmware: true},
		Firmware:       ap.Firmware{Filename: "fw.bl7"},
		ConnectTimeout: 2 * time.Second, DialogTimeout: 2 * time.Second, Deadline: 20 * time.Second,
	}

	var mu sync.Mutex
	scans := 0
	emit := func(e Event) {
		if e.Kind == EvProgress && e.Phase == "watch" {
			mu.Lock()
			scans++
			mu.Unlock()
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_, _ = runJob(ctx, opt, []string{"127.0.0.1"}, cfg, emit, nil)
		close(done)
	}()

	// No AP ever fetches the image, so nothing completes. The re-scan must run
	// regardless.
	waitFor(t, 8*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return scans >= 3
	}, "the re-scan never started while a download was outstanding")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the run ignored Stop")
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

// The reported failure: a pass over several hundred APs reported nothing until
// it had finished, so a working re-scan looked like a frozen table. Results and
// progress must arrive during the pass, not only at the end of it.
func TestReScanReportsDuringThePassNotOnlyAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fake := newRestartableAP(t, "7.1.1.0.6250")
	opt := options{
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 300 * time.Millisecond, pingConcurrency: 8, concurrency: 4,
		watchEnabled: true, watchInterval: 250 * time.Millisecond,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		ConnectTimeout: 2 * time.Second, DialogTimeout: 2 * time.Second, Deadline: 20 * time.Second,
	}
	results := []ap.Result{{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1.1.0.6250"}}

	var mu sync.Mutex
	var order []string
	emit := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case e.Kind == EvPhase && e.Phase == "rescan":
			order = append(order, "pass-start")
		case e.Kind == EvProgress && e.Phase == "rescan-ping":
			order = append(order, "ping-progress")
		case e.Kind == EvProgress && e.Phase == "rescan-read":
			order = append(order, "read-progress")
		case e.Kind == EvProgress && e.Phase == "watch":
			order = append(order, "pass-end")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { watchAPs(ctx, opt, cfg, results, emit); close(done) }()

	waitFor(t, 6*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range order {
			if s == "pass-end" {
				return true
			}
		}
		return false
	}, "no re-scan pass completed")
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	// Everything up to the first pass-end is what the operator sees while the
	// pass is running; it must not be empty.
	var during []string
	for _, s := range order {
		if s == "pass-end" {
			break
		}
		during = append(during, s)
	}
	if len(during) == 0 {
		t.Fatalf("nothing was reported before the pass finished: %v", order)
	}
	if during[0] != "pass-start" {
		t.Errorf("the pass did not announce itself first: %v", during)
	}
	has := func(want string) bool {
		for _, s := range during {
			if s == want {
				return true
			}
		}
		return false
	}
	if !has("ping-progress") {
		t.Errorf("no ping progress during the pass: %v", during)
	}
	if !has("read-progress") {
		t.Errorf("no version-read progress during the pass: %v", during)
	}
}

// The interval is the rest between passes, measured from the end of one to the
// start of the next. With a ticker, a pass that overran the interval left a
// tick already queued and the next pass began the instant the last finished.
func TestPassesAreSpacedFromTheEndOfThePrevious(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	// The AP stalls before its login banner, so every pass takes at least this
	// long — comfortably longer than the interval below.
	const stall = 700 * time.Millisecond
	const interval = 200 * time.Millisecond

	fake := newRestartableAP(t, "7.1.1.0.6250")
	fake.stallBeforeLogin = stall

	opt := options{
		probe: "tcp", sshPort: fake.port,
		pingTimeout: 200 * time.Millisecond, pingConcurrency: 8, concurrency: 4,
		watchEnabled: true, watchInterval: interval,
	}
	cfg := ap.Config{
		Credentials:    []ap.Credentials{{User: "admin", Password: "x"}},
		Port:           fake.port,
		ConnectTimeout: 3 * time.Second, DialogTimeout: 3 * time.Second, Deadline: 20 * time.Second,
	}
	results := []ap.Result{{IP: "127.0.0.1", Status: "Done", Reachable: true, Firmware: "7.1.1.0.6250"}}

	var mu sync.Mutex
	var starts, ends []time.Time
	emit := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case e.Kind == EvPhase && e.Phase == "rescan":
			starts = append(starts, time.Now())
		case e.Kind == EvProgress && e.Phase == "watch":
			ends = append(ends, time.Now())
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { watchAPs(ctx, opt, cfg, results, emit); close(done) }()

	waitFor(t, 15*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ends) >= 2 && len(starts) >= 3
	}, "not enough passes completed to measure the spacing")
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	// Every pass after the first must begin at least an interval after the
	// previous one ended, allowing a little slack for scheduling.
	const slack = 60 * time.Millisecond
	for i := 0; i < len(ends) && i+1 < len(starts); i++ {
		gap := starts[i+1].Sub(ends[i])
		if gap < interval-slack {
			t.Errorf("pass %d started %v after the previous ended, want at least %v",
				i+2, gap.Round(time.Millisecond), interval)
		}
	}

	// And a pass really did overrun the interval, or the test proves nothing.
	if len(ends) > 0 && ends[0].Sub(starts[0]) <= interval {
		t.Fatalf("a pass took %v, which is not longer than the %v interval; the test is not exercising the overrun",
			ends[0].Sub(starts[0]).Round(time.Millisecond), interval)
	}
}
