package ap

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// forcedChangeAP is a factory-default AP: it accepts the default credentials
// and then refuses to go any further until the password is changed, rejecting
// any new password equal to the old one — which is what a real Ruckus AP does
// on first login.
type forcedChangeAP struct {
	ln net.Listener
	// relogin models the builds that throw you back to the login prompt after
	// the change, where only the new password is accepted.
	relogin bool

	mu       sync.Mutex
	offered  []string // every value sent at a "new password" prompt
	loggedIn []string // every password sent at an ordinary login prompt
	reached  bool     // did the client ever get to the CLI prompt
}

func newForcedChangeAP(t *testing.T) *forcedChangeAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &forcedChangeAP{ln: ln}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	conf := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
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

func (f *forcedChangeAP) addr() (string, string) {
	h, p, _ := net.SplitHostPort(f.ln.Addr().String())
	return h, p
}

func (f *forcedChangeAP) serve(c net.Conn, conf *ssh.ServerConfig) {
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
		f.cli(ch)
		_ = ch.Close()
	}
}

const (
	defaultPass    = "sp-admin"
	newPassForTest = "Str0ngNewPass!"
)

func (f *forcedChangeAP) cli(ch ssh.Channel) {
	in := bufio.NewReader(ch)
	say := func(s string) { _, _ = ch.Write([]byte(s)) }
	line := func() (string, bool) {
		l, err := in.ReadString('\n')
		if err != nil {
			return "", false
		}
		return strings.TrimRight(l, "\r\n"), true
	}

	say("\r\nPlease login: ")
	if _, ok := line(); !ok {
		return
	}
	say("password : ")
	pw, ok := line()
	if !ok {
		return
	}
	f.mu.Lock()
	f.loggedIn = append(f.loggedIn, pw)
	f.mu.Unlock()

	// Accepted — but the AP now insists on a new password before anything else.
	say("\r\n** The default password must be changed before continuing **\r\n")
	for {
		say("Please enter new password : ")
		np, ok := line()
		if !ok {
			return
		}
		f.mu.Lock()
		f.offered = append(f.offered, np)
		f.mu.Unlock()

		if np == defaultPass || np == "" {
			say("\r\nPassword can not be the same as the default password.\r\n")
			continue
		}
		say("Please confirm new password : ")
		cp, ok := line()
		if !ok {
			return
		}
		if cp != np {
			say("\r\nPasswords do not match.\r\n")
			continue
		}
		break
	}

	if f.relogin {
		// Log the session out and insist on the new password this time.
		say("\r\nPassword changed. Please log in again.\r\n")
		for {
			say("\r\nPlease login: ")
			if _, ok := line(); !ok {
				return
			}
			say("password : ")
			pw, ok := line()
			if !ok {
				return
			}
			f.mu.Lock()
			f.loggedIn = append(f.loggedIn, pw)
			f.mu.Unlock()
			if pw == newPassForTest {
				break
			}
			say("\r\nLogin incorrect\r\n")
		}
	}

	f.mu.Lock()
	f.reached = true
	f.mu.Unlock()
	say("\r\nPassword changed.\r\nrkscli: ")
	for {
		cmd, ok := line()
		if !ok {
			return
		}
		if cmd == "get version" {
			say("Version: 110.0.0.0.1347\r\n")
		}
		say("rkscli: ")
	}
}

// Issue #5: on an AP that demands a password change at first login, the tool
// used to answer the "new password" prompt with the password it had just
// logged in with. The AP refuses that, and the run died without ever reaching
// a prompt.
func TestForcedPasswordChangeSetsTheSuppliedPassword(t *testing.T) {
	f := newForcedChangeAP(t)
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Credentials = []Credentials{{User: "super", Password: defaultPass}}
	cfg.NewPassword = newPassForTest

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r := Run(ctx, host, cfg)

	f.mu.Lock()
	offered, reached := append([]string(nil), f.offered...), f.reached
	f.mu.Unlock()

	if r.Error != "" {
		t.Fatalf("Run failed: %s", r.Error)
	}
	if !reached {
		t.Fatal("never reached the CLI prompt")
	}
	for _, v := range offered {
		if v == defaultPass {
			t.Fatalf("sent the current password as the new one; offered = %q", offered)
		}
	}
	if len(offered) != 1 || offered[0] != newPassForTest {
		t.Errorf("offered = %q, want exactly one %q", offered, newPassForTest)
	}
	if r.Note != "password changed" {
		t.Errorf("Note = %q, want %q", r.Note, "password changed")
	}
	if r.Firmware != "110.0.0.0.1347" {
		t.Errorf("Firmware = %q; the run did not carry on past the change", r.Firmware)
	}
}

// With no new password to set, the AP is reported and skipped. It must not
// guess, and it must not sit there burning the deadline.
func TestForcedPasswordChangeWithoutOneIsReported(t *testing.T) {
	f := newForcedChangeAP(t)
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Credentials = []Credentials{{User: "super", Password: defaultPass}}
	cfg.NewPassword = ""

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r := Run(ctx, host, cfg)
	elapsed := time.Since(start)

	f.mu.Lock()
	offered := append([]string(nil), f.offered...)
	f.mu.Unlock()

	if len(offered) != 0 {
		t.Errorf("sent %q at the new-password prompt; it should send nothing", offered)
	}
	if r.Status != "Needs Password" {
		t.Errorf("Status = %q, want %q", r.Status, "Needs Password")
	}
	if !strings.Contains(r.Error, "requires a password change") {
		t.Errorf("Error = %q, want it to say a password change is required", r.Error)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v; it waited out a timeout instead of reporting straight away", elapsed)
	}
}

// Some builds throw the session back to the login prompt after the change. The
// old password is dead by then, so the new one has to be used.
func TestReloginAfterChangeUsesTheNewPassword(t *testing.T) {
	f := newForcedChangeAP(t)
	f.relogin = true
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Credentials = []Credentials{{User: "super", Password: defaultPass}}
	cfg.NewPassword = newPassForTest

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r := Run(ctx, host, cfg)

	f.mu.Lock()
	loggedIn, reached := append([]string(nil), f.loggedIn...), f.reached
	f.mu.Unlock()

	if r.Error != "" {
		t.Fatalf("Run failed: %s", r.Error)
	}
	if !reached {
		t.Fatal("never reached the CLI prompt after the re-login")
	}
	if len(loggedIn) < 2 || loggedIn[len(loggedIn)-1] != newPassForTest {
		t.Errorf("passwords sent at login = %q, want the last to be the new one", loggedIn)
	}
}

// "Password changed." contains the word the login state machine watches for.
// Treating it as a prompt would send the password as a CLI command, putting it
// into the AP's command history.
func TestStatusLineIsNotMistakenForAPasswordPrompt(t *testing.T) {
	e := newExpecter(nil, strings.NewReader("\r\nPassword changed.\r\nrkscli: "), time.Second)
	i, _, err := e.ExpectPats(
		atEndFold("password:"), atEndFold("password :"), atEnd("rkscli: "),
	)
	if err != nil {
		t.Fatalf("expect: %v", err)
	}
	if i != 2 {
		t.Errorf("matched pattern %d; the word inside \"Password changed.\" was treated as a prompt", i)
	}
}

// The prompts differ only by prefix, so the longest end-anchored match has to
// win or a confirmation gets answered as if it were the first prompt.
func TestConfirmPromptBeatsTheNewPasswordPrompt(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"\r\nPlease enter new password : ", 1},
		{"\r\nNew Password: ", 1},
		{"\r\nPlease confirm new password : ", 2},
		{"\r\nConfirm New Password: ", 2},
		{"\r\nRe-enter new password: ", 2},
		{"\r\npassword : ", 0},
		{"\r\nPassword: ", 0},
	}
	for _, c := range cases {
		e := newExpecter(nil, strings.NewReader(c.text), time.Second)
		i, _, err := e.ExpectPats(
			atEndFold("password:"),                                  // 0
			atEndFold("new password:"), atEndFold("new password :"), // 1 (and 2 below)
			atEndFold("confirm new password:"), atEndFold("confirm new password :"),
			atEndFold("re-enter new password:"),
			atEndFold("password :"),
		)
		if err != nil {
			t.Errorf("%q: %v", c.text, err)
			continue
		}
		got := 0
		switch {
		case i == 1 || i == 2:
			got = 1
		case i >= 3 && i <= 5:
			got = 2
		}
		if got != c.want {
			t.Errorf("%q matched pattern %d (class %d), want class %d", c.text, i, got, c.want)
		}
	}
}
