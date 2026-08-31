// Package wifi drives the sensor's radio the way a client device would: it
// associates to an SSID, times each step of getting on, and reads what the
// radio can see around it.
//
// Everything here goes through wpa_supplicant's control interface — the same
// interface wpa_cli uses — because that is what gives a full 802.1X supplicant,
// WPA2 and WPA3, and event-level timing without writing a supplicant.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Ctrl is one connection to wpa_supplicant's control socket for an interface.
type Ctrl struct {
	conn  *net.UnixConn
	local string
	path  string
}

var ctrlSeq atomic.Int64

// DefaultCtrlDir is where wpa_supplicant puts its per-interface sockets on a
// Raspberry Pi OS install.
const DefaultCtrlDir = "/run/wpa_supplicant"

// Dial opens the control socket for one interface. The client end is a socket
// of our own in the temporary directory, which is how the datagram interface
// works: wpa_supplicant replies to the address we sent from.
func Dial(dir, iface string) (*Ctrl, error) {
	if dir == "" {
		dir = DefaultCtrlDir
	}
	path := filepath.Join(dir, iface)
	local := filepath.Join(os.TempDir(), fmt.Sprintf("cbsensor-%d-%d", os.Getpid(), ctrlSeq.Add(1)))
	conn, err := net.DialUnix("unixgram",
		&net.UnixAddr{Name: local, Net: "unixgram"},
		&net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		os.Remove(local)
		return nil, fmt.Errorf("wpa_supplicant control socket %s: %w", path, err)
	}
	return &Ctrl{conn: conn, local: local, path: path}, nil
}

// Close releases the socket and removes our end of it.
func (c *Ctrl) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	os.Remove(c.local)
	c.conn = nil
	return err
}

// Request sends one command and returns the reply. wpa_supplicant answers
// "FAIL" or "UNKNOWN COMMAND" as a normal reply, so those are turned into
// errors here rather than being passed up as text that reads like success.
func (c *Ctrl) Request(cmd string) (string, error) {
	if c == nil || c.conn == nil {
		return "", errors.New("control socket is closed")
	}
	if err := c.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", err
	}
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return "", err
	}
	buf := make([]byte, 8192)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return "", fmt.Errorf("%s: %w", firstWord(cmd), err)
		}
		reply := string(buf[:n])
		// An attached socket also carries events; they are prefixed with a
		// priority in angle brackets and are never the answer to a command.
		if strings.HasPrefix(reply, "<") {
			continue
		}
		trimmed := strings.TrimRight(reply, "\n")
		// Refusals come back as ordinary replies: "FAIL", "FAIL-BUSY" while a
		// scan is already running, "UNKNOWN COMMAND" on an older build.
		if strings.HasPrefix(trimmed, "FAIL") || trimmed == "UNKNOWN COMMAND" {
			return trimmed, fmt.Errorf("wpa_supplicant refused %q: %s", firstWord(cmd), trimmed)
		}
		return trimmed, nil
	}
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}

// Events returns a channel of the unsolicited messages wpa_supplicant emits —
// the association, EAP and key-exchange milestones the timing breakdown is
// built from. The channel closes when ctx is done.
func (c *Ctrl) Events(ctx context.Context) (<-chan string, error) {
	if _, err := c.Request("ATTACH"); err != nil {
		return nil, err
	}
	out := make(chan string, 64)
	go func() {
		defer close(out)
		buf := make([]byte, 4096)
		for {
			if ctx.Err() != nil {
				return
			}
			c.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, err := c.conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			msg := strings.TrimRight(string(buf[:n]), "\n")
			// Strip the "<3>" priority prefix events carry.
			if strings.HasPrefix(msg, "<") {
				if i := strings.IndexByte(msg, '>'); i > 0 {
					msg = msg[i+1:]
				}
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Status is what wpa_supplicant reports about the current association.
type Status struct {
	State     string // COMPLETED, SCANNING, DISCONNECTED, 4WAY_HANDSHAKE...
	SSID      string
	BSSID     string
	Freq      int
	Channel   int
	Band      string
	IPAddress string
	KeyMgmt   string
	Pairwise  string
	Group     string
	EAPMethod string
	ID        int
}

// Connected reports whether the radio is associated and keyed.
func (s Status) Connected() bool { return s.State == "COMPLETED" }

// Status reads the current state of the interface.
func (c *Ctrl) Status() (Status, error) {
	reply, err := c.Request("STATUS")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(reply), nil
}

func parseStatus(reply string) Status {
	var s Status
	s.ID = -1
	for _, line := range strings.Split(reply, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "wpa_state":
			s.State = v
		case "ssid":
			s.SSID = v
		case "bssid":
			s.BSSID = v
		case "freq":
			s.Freq, _ = strconv.Atoi(v)
		case "ip_address":
			s.IPAddress = v
		case "key_mgmt":
			s.KeyMgmt = v
		case "pairwise_cipher":
			s.Pairwise = v
		case "group_cipher":
			s.Group = v
		case "EAP state", "eap_method", "selectedMethod":
			if s.EAPMethod == "" {
				s.EAPMethod = v
			}
		case "id":
			s.ID, _ = strconv.Atoi(v)
		}
	}
	s.Channel = ChannelFor(s.Freq)
	s.Band = BandFor(s.Freq)
	return s
}

// Signal is one radio-level reading, taken at the moment it is asked for.
type Signal struct {
	RSSI      int // dBm
	Noise     int // dBm, where the driver reports it
	SNR       int // dB
	TxBitrate float64
	RxBitrate float64
	Freq      int
	Channel   int
	Width     string
}

// SignalPoll reads the current signal. This is the number a user means by
// "the wifi is weak", and it belongs on every result the sensor records.
func (c *Ctrl) SignalPoll() (Signal, error) {
	reply, err := c.Request("SIGNAL_POLL")
	if err != nil {
		return Signal{}, err
	}
	return parseSignalPoll(reply), nil
}

func parseSignalPoll(reply string) Signal {
	var s Signal
	s.Noise = 1 // sentinel: no reading
	for _, line := range strings.Split(reply, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "RSSI":
			s.RSSI, _ = strconv.Atoi(v)
		case "NOISE":
			s.Noise, _ = strconv.Atoi(v)
		case "FREQUENCY":
			s.Freq, _ = strconv.Atoi(v)
		case "LINKSPEED", "TXBITRATE":
			s.TxBitrate, _ = strconv.ParseFloat(v, 64)
		case "RXBITRATE":
			s.RxBitrate, _ = strconv.ParseFloat(v, 64)
		case "WIDTH":
			s.Width = v
		}
	}
	s.Channel = ChannelFor(s.Freq)
	if s.Noise <= 0 && s.RSSI != 0 {
		s.SNR = s.RSSI - s.Noise
	}
	if s.Noise == 1 {
		s.Noise = 0
	}
	return s
}
