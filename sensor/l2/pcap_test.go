package l2

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestPcapWriterProducesAReadableFile(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewPcapWriter(&buf, 128)
	if err != nil {
		t.Fatal(err)
	}
	frame := bytes.Repeat([]byte{0xab}, 300)
	when := time.Unix(1700000000, 123456000)
	if err := w.WritePacket(when, frame, len(frame)); err != nil {
		t.Fatal(err)
	}

	out := buf.Bytes()
	if binary.LittleEndian.Uint32(out) != pcapMagic {
		t.Fatalf("magic = %#x", binary.LittleEndian.Uint32(out))
	}
	if binary.LittleEndian.Uint32(out[16:]) != 128 {
		t.Errorf("snaplen in the header = %d", binary.LittleEndian.Uint32(out[16:]))
	}
	if binary.LittleEndian.Uint32(out[20:]) != linkTypeEther {
		t.Error("the link type is not Ethernet")
	}
	rec := out[24:]
	if binary.LittleEndian.Uint32(rec) != 1700000000 {
		t.Errorf("timestamp = %d", binary.LittleEndian.Uint32(rec))
	}
	if binary.LittleEndian.Uint32(rec[4:]) != 123456 {
		t.Errorf("microseconds = %d", binary.LittleEndian.Uint32(rec[4:]))
	}
	// The frame is longer than the snap length, so it is truncated on disk
	// and its real length is recorded beside it.
	if got := binary.LittleEndian.Uint32(rec[8:]); got != 128 {
		t.Errorf("captured length = %d, want the snap length", got)
	}
	if got := binary.LittleEndian.Uint32(rec[12:]); got != 300 {
		t.Errorf("original length = %d, want 300", got)
	}
	if packets, bytes := w.Stats(); packets != 1 || bytes != 128+16 {
		t.Errorf("stats = %d packets, %d bytes", packets, bytes)
	}
}

// ipv4Frame builds an Ethernet/IPv4/TCP-or-UDP frame for the filter tests.
func ipv4Frame(src, dst string, proto byte, sport, dport int) []byte {
	frame := make([]byte, 54)
	binary.BigEndian.PutUint16(frame[12:], 0x0800)
	ip := frame[14:]
	ip[0] = 0x45
	ip[9] = proto
	copy(ip[12:16], net.ParseIP(src).To4())
	copy(ip[16:20], net.ParseIP(dst).To4())
	binary.BigEndian.PutUint16(ip[20:], uint16(sport))
	binary.BigEndian.PutUint16(ip[22:], uint16(dport))
	return frame
}

func TestCaptureFilter(t *testing.T) {
	web := ipv4Frame("10.20.30.55", "203.0.113.10", 6, 51234, 443)
	dns := ipv4Frame("10.20.30.55", "10.20.30.2", 17, 51235, 53)
	other := ipv4Frame("10.20.30.99", "10.20.30.2", 6, 22, 51236)

	cases := []struct {
		name  string
		f     Filter
		frame []byte
		want  bool
	}{
		{"everything", Filter{}, web, true},
		{"host matches the source", Filter{Host: "10.20.30.55"}, web, true},
		{"host matches the destination", Filter{Host: "203.0.113.10"}, web, true},
		{"host does not match", Filter{Host: "10.20.30.55"}, other, false},
		{"port either way", Filter{Port: 443}, web, true},
		{"port does not match", Filter{Port: 443}, dns, false},
		{"protocol", Filter{Proto: "udp"}, dns, true},
		{"protocol excludes", Filter{Proto: "udp"}, web, false},
		{"host and port together", Filter{Host: "10.20.30.2", Port: 53, Proto: "udp"}, dns, true},
		{"host right, port wrong", Filter{Host: "10.20.30.2", Port: 80}, dns, false},
	}
	for _, c := range cases {
		if got := c.f.Match(c.frame); got != c.want {
			t.Errorf("%s: match = %v, want %v", c.name, got, c.want)
		}
	}

	// A short or non-IP frame must be dropped by a filter, not crash it.
	runt := Filter{Host: "10.0.0.1"}
	if runt.Match([]byte{1, 2, 3}) {
		t.Error("a runt frame matched a host filter")
	}
}

func TestCaptureOptionsAreBounded(t *testing.T) {
	// A capture with nothing set must still end on its own.
	var o CaptureOptions
	o.withDefaults()
	if o.Duration == 0 || o.MaxBytes == 0 || o.MaxPacket == 0 || o.Snaplen == 0 {
		t.Fatalf("an unbounded capture would be allowed: %+v", o)
	}
}
