package wifi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func dialFake(t *testing.T, f *fakeSupplicant, iface string) *Ctrl {
	t.Helper()
	c, err := Dial(f.dir, iface)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestRequestReturnsRepliesAndRefusals(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	f.reply("PING", "PONG")
	f.reply("BAD_COMMAND", "UNKNOWN COMMAND")
	f.reply("SCAN", "FAIL-BUSY")
	c := dialFake(t, f, "wlan0")

	if reply, err := c.Request("PING"); err != nil || reply != "PONG" {
		t.Fatalf("PING = %q, %v", reply, err)
	}
	if _, err := c.Request("BAD_COMMAND"); err == nil {
		t.Error("an unknown command was reported as success")
	}
	if _, err := c.Request("SCAN"); err == nil || !strings.Contains(err.Error(), "FAIL-BUSY") {
		t.Errorf("a busy scan was not surfaced: %v", err)
	}
}

func TestCtrlClosesCleanly(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	c := dialFake(t, f, "wlan0")
	local := c.local
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := c.Request("PING"); err == nil {
		t.Error("a closed socket accepted a command")
	}
	if _, err := os.Stat(local); err == nil {
		t.Error("the client socket was left behind on disk")
	}
}

func TestParseStatus(t *testing.T) {
	const reply = `bssid=b8:27:eb:aa:bb:cc
freq=5180
ssid=Campus-Secure
id=0
mode=station
wifi_generation=6
pairwise_cipher=CCMP
group_cipher=CCMP
key_mgmt=WPA2/IEEE 802.1X/EAP
wpa_state=COMPLETED
ip_address=10.20.30.55
address=b8:27:eb:11:22:33`
	s := parseStatus(reply)
	if !s.Connected() {
		t.Fatalf("state = %q", s.State)
	}
	if s.SSID != "Campus-Secure" || s.BSSID != "b8:27:eb:aa:bb:cc" {
		t.Errorf("status = %+v", s)
	}
	if s.Channel != 36 || s.Band != "5 GHz" {
		t.Errorf("channel = %d, band = %q", s.Channel, s.Band)
	}
	if s.IPAddress != "10.20.30.55" {
		t.Errorf("address = %q", s.IPAddress)
	}
}

func TestParseSignalPoll(t *testing.T) {
	s := parseSignalPoll("RSSI=-62\nLINKSPEED=390\nNOISE=-96\nFREQUENCY=5500\nWIDTH=80 MHz\n")
	if s.RSSI != -62 || s.Noise != -96 {
		t.Fatalf("signal = %+v", s)
	}
	if s.SNR != 34 {
		t.Errorf("SNR = %d, want 34", s.SNR)
	}
	if s.Channel != 100 {
		t.Errorf("channel = %d, want 100", s.Channel)
	}
	if s.TxBitrate != 390 || s.Width != "80 MHz" {
		t.Errorf("rate = %v, width = %q", s.TxBitrate, s.Width)
	}
}

// A driver that will not report noise must not produce an invented SNR.
func TestParseSignalPollWithoutNoise(t *testing.T) {
	s := parseSignalPoll("RSSI=-55\nFREQUENCY=2412\n")
	if s.SNR != 0 || s.Noise != 0 {
		t.Fatalf("SNR was invented without a noise reading: %+v", s)
	}
}

func TestEventsStripThePriorityPrefix(t *testing.T) {
	f := newFakeSupplicant(t, "wlan0")
	c := dialFake(t, f, "wlan0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	go f.emitSequence(10*time.Millisecond, "CTRL-EVENT-SCAN-STARTED")
	select {
	case ev := <-events:
		if ev != "CTRL-EVENT-SCAN-STARTED" {
			t.Fatalf("event = %q", ev)
		}
	case <-ctx.Done():
		t.Fatal("no event arrived")
	}
}
