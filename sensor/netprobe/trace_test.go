package netprobe

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func TestInnerEchoReadsTheQuotedProbe(t *testing.T) {
	// A time-exceeded message quotes the IP header of the packet that expired
	// plus the first eight bytes after it — our echo header.
	quoted := make([]byte, 28)
	quoted[0] = 0x45 // IPv4, 20-byte header
	quoted[20] = 8   // echo request
	binary.BigEndian.PutUint16(quoted[24:], 4242)
	binary.BigEndian.PutUint16(quoted[26:], 7)

	id, seq, ok := innerEcho(quoted)
	if !ok || id != 4242 || seq != 7 {
		t.Fatalf("id = %d, seq = %d, ok = %v", id, seq, ok)
	}

	if _, _, ok := innerEcho(quoted[:15]); ok {
		t.Error("a truncated quote parsed as a probe")
	}
	withOptions := make([]byte, 32)
	withOptions[0] = 0x46 // 24-byte header, one option word
	withOptions[24] = 8
	binary.BigEndian.PutUint16(withOptions[28:], 99)
	if id, _, ok := innerEcho(withOptions); !ok || id != 99 {
		t.Errorf("an IP header carrying options was misread: id = %d, ok = %v", id, ok)
	}
}

func TestClassifyICMPSeparatesRepliesFromExpiries(t *testing.T) {
	echo, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEchoReply,
		Body: &icmp.Echo{ID: 7, Seq: 3, Data: []byte("x")},
	}).Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	kind, id, seq, ok := classifyICMP(echo)
	if !ok || kind != icmpEchoReply || id != 7 || seq != 3 {
		t.Fatalf("echo reply read as kind=%d id=%d seq=%d ok=%v", kind, id, seq, ok)
	}

	quoted := make([]byte, 28)
	quoted[0] = 0x45
	quoted[20] = 8
	binary.BigEndian.PutUint16(quoted[24:], 7)
	binary.BigEndian.PutUint16(quoted[26:], 2)
	expired, err := (&icmp.Message{
		Type: ipv4.ICMPTypeTimeExceeded,
		Body: &icmp.TimeExceeded{Data: quoted},
	}).Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	kind, id, seq, ok = classifyICMP(expired)
	if !ok || kind != icmpTimeExceeded || id != 7 || seq != 2 {
		t.Fatalf("time exceeded read as kind=%d id=%d seq=%d ok=%v", kind, id, seq, ok)
	}

	if _, _, _, ok := classifyICMP([]byte{0xff, 0xff}); ok {
		t.Error("rubbish parsed as an ICMP message")
	}
}

func TestParseTracerouteOutput(t *testing.T) {
	const out = ` 1  192.168.1.1  0.412 ms
 2  * 
 3  10.20.0.1  4.117 ms
 4  203.0.113.9  18.004 ms
`
	hops := parseTracerouteOutput(out)
	if len(hops) != 4 {
		t.Fatalf("hops = %d: %+v", len(hops), hops)
	}
	if hops[0].Addr != "192.168.1.1" || hops[0].RTT != 412*time.Microsecond {
		t.Errorf("first hop = %+v", hops[0])
	}
	if !hops[1].Timeout || hops[1].Addr != "" {
		t.Errorf("a silent hop was not recorded as one: %+v", hops[1])
	}
	if hops[3].RTT != 18004*time.Microsecond {
		t.Errorf("last hop RTT = %v", hops[3].RTT)
	}
}

// Without privilege there is no raw socket and possibly no traceroute binary.
// Either way the caller gets a result it can report, not a hang.
func TestTracerouteAlwaysReturnsSomething(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := Traceroute(ctx, "192.0.2.1", 2, 200*time.Millisecond)
	if r.Err == nil && len(r.Hops) == 0 {
		t.Error("neither hops nor a reason came back")
	}
	if r.Target != "192.0.2.1" {
		t.Errorf("target = %q", r.Target)
	}
}
