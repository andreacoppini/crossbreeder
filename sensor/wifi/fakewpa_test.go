package wifi

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSupplicant is a stand-in for wpa_supplicant: a real unixgram control
// socket that answers commands and pushes events to whoever has attached, so
// the client, the command wire format and the event timing are all exercised
// without a radio.
type fakeSupplicant struct {
	t    *testing.T
	dir  string
	conn *net.UnixConn

	mu        sync.Mutex
	commands  []string
	attached  []*net.UnixAddr
	replies   map[string]string
	onCommand func(f *fakeSupplicant, cmd string)
	closed    bool
}

func newFakeSupplicant(t *testing.T, iface string) *fakeSupplicant {
	t.Helper()
	// The socket path has to be short: a unix address is capped at about 100
	// bytes, and a temporary directory name is long.
	dir, err := os.MkdirTemp("", "cbw")
	if err != nil {
		t.Fatal(err)
	}
	addr := &net.UnixAddr{Name: filepath.Join(dir, iface), Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Skipf("unix datagram sockets are unavailable: %v", err)
	}
	f := &fakeSupplicant{t: t, dir: dir, conn: conn, replies: map[string]string{}}
	t.Cleanup(f.Close)
	go f.serve()
	return f
}

func (f *fakeSupplicant) Close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.mu.Unlock()
	f.conn.Close()
	os.RemoveAll(f.dir)
}

func (f *fakeSupplicant) serve() {
	buf := make([]byte, 4096)
	for {
		n, from, err := f.conn.ReadFromUnix(buf)
		if err != nil {
			return
		}
		cmd := string(buf[:n])
		f.mu.Lock()
		f.commands = append(f.commands, cmd)
		reply, ok := f.replies[cmd]
		if !ok {
			reply, ok = f.replies[firstWord(cmd)]
		}
		if !ok {
			reply = "OK"
		}
		if cmd == "ATTACH" {
			f.attached = append(f.attached, from)
		}
		hook := f.onCommand
		f.mu.Unlock()

		f.conn.WriteToUnix([]byte(reply), from)
		if hook != nil {
			go hook(f, cmd)
		}
	}
}

// emit pushes an event to every attached client, as wpa_supplicant does.
func (f *fakeSupplicant) emit(event string) {
	f.mu.Lock()
	targets := append([]*net.UnixAddr(nil), f.attached...)
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return
	}
	for _, to := range targets {
		f.conn.WriteToUnix([]byte("<3>"+event), to)
	}
}

// emitSequence plays a scripted association, with a pause between steps so the
// phase timings come out non-zero.
func (f *fakeSupplicant) emitSequence(step time.Duration, events ...string) {
	for _, e := range events {
		time.Sleep(step)
		f.emit(e)
	}
}

func (f *fakeSupplicant) reply(cmd, reply string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[cmd] = reply
}

func (f *fakeSupplicant) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *fakeSupplicant) sentContaining(substr string) []string {
	var out []string
	for _, c := range f.sent() {
		if strings.Contains(c, substr) {
			out = append(out, c)
		}
	}
	return out
}
