package netprobe

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// reply builds a response to query by hand — question echoed back, answers
// pointed at it with a compression pointer — so the parser is tested against
// bytes assembled independently of the encoder.
func reply(query []byte, rcode uint16, tc bool, rrs ...func() (uint16, []byte)) []byte {
	qend := 12
	for query[qend] != 0 {
		qend += int(query[qend]) + 1
	}
	qend += 5 // root label + type + class

	out := make([]byte, 0, 512)
	out = append(out, query[:12]...)
	flags := uint16(0x8180) | rcode // QR, RD, RA
	if tc {
		flags |= 0x0200
	}
	binary.BigEndian.PutUint16(out[2:], flags)
	binary.BigEndian.PutUint16(out[6:], uint16(len(rrs)))
	out = append(out, query[12:qend]...)
	for _, rr := range rrs {
		rtype, rdata := rr()
		var hdr [12]byte
		binary.BigEndian.PutUint16(hdr[0:], 0xc00c) // pointer to the question name
		binary.BigEndian.PutUint16(hdr[2:], rtype)
		binary.BigEndian.PutUint16(hdr[4:], 1)
		binary.BigEndian.PutUint32(hdr[6:], 300)
		binary.BigEndian.PutUint16(hdr[10:], uint16(len(rdata)))
		out = append(out, hdr[:]...)
		out = append(out, rdata...)
	}
	return out
}

func rrA(ip string) func() (uint16, []byte) {
	return func() (uint16, []byte) { return TypeA, net.ParseIP(ip).To4() }
}

// serveUDP stands up a resolver on loopback that answers with what handler
// returns.
func serveUDP(t *testing.T, handler func(query []byte) []byte) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if out := handler(append([]byte(nil), buf[:n]...)); out != nil {
				pc.WriteTo(out, addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

func serveTCP(t *testing.T, handler func(query []byte) []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no TCP loopback: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var l [2]byte
				if _, err := c.Read(l[:]); err != nil {
					return
				}
				q := make([]byte, binary.BigEndian.Uint16(l[:]))
				if _, err := c.Read(q); err != nil {
					return
				}
				out := handler(q)
				framed := make([]byte, 2+len(out))
				binary.BigEndian.PutUint16(framed, uint16(len(out)))
				copy(framed[2:], out)
				c.Write(framed)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestResolveReadsAnswersAndTimes(t *testing.T) {
	addr := serveUDP(t, func(q []byte) []byte {
		return reply(q, 0, false, rrA("10.1.2.3"), rrA("10.1.2.4"))
	})
	r := Resolve(context.Background(), DNSQuery{Server: addr, Name: "portal.example.com", Type: TypeA})
	if !r.OK() {
		t.Fatalf("lookup failed: %+v", r)
	}
	if len(r.Answers) != 2 || r.Answers[0] != "10.1.2.3" {
		t.Fatalf("answers = %v", r.Answers)
	}
	if r.TTL != 300 {
		t.Errorf("TTL = %d, want 300", r.TTL)
	}
	if r.RTT <= 0 || r.RTT > 3*time.Second {
		t.Errorf("RTT = %v, which is not a plausible loopback timing", r.RTT)
	}
}

// A resolver that answers NXDOMAIN is up and healthy as a service; the lookup
// is what failed. The two have to be distinguishable or the issue engine
// blames the wrong thing.
func TestResolveReportsRCodeWithoutTransportError(t *testing.T) {
	addr := serveUDP(t, func(q []byte) []byte { return reply(q, 3, false) })
	r := Resolve(context.Background(), DNSQuery{Server: addr, Name: "nope.example.com"})
	if r.Err != nil {
		t.Fatalf("unexpected transport error: %v", r.Err)
	}
	if r.RCode != "NXDOMAIN" || r.OK() {
		t.Fatalf("RCode = %q, OK = %v", r.RCode, r.OK())
	}
}

func TestResolveRetriesTruncatedAnswerOverTCP(t *testing.T) {
	tcpAddr := serveTCP(t, func(q []byte) []byte { return reply(q, 0, false, rrA("10.9.9.9")) })
	// The UDP and TCP listeners must share a port number for the fallback to
	// reach the right server, so the UDP side is bound to the TCP port.
	host, port, _ := net.SplitHostPort(tcpAddr)
	pc, err := net.ListenPacket("udp", net.JoinHostPort(host, port))
	if err != nil {
		t.Skipf("cannot bind UDP on the TCP port: %v", err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(reply(buf[:n], 0, true), addr)
		}
	}()

	r := Resolve(context.Background(), DNSQuery{Server: tcpAddr, Name: "big.example.com"})
	if !r.OK() || r.Answers[0] != "10.9.9.9" {
		t.Fatalf("truncated answer was not retried over TCP: %+v", r)
	}
	if r.Proto != "udp+tcp" {
		t.Errorf("Proto = %q, want udp+tcp", r.Proto)
	}
}

// A captive portal answers every name with its own address. It is fast and it
// is NOERROR, so only checking the value catches it.
func TestResolveFailsWhenTheAnswerIsNotTheExpectedOne(t *testing.T) {
	addr := serveUDP(t, func(q []byte) []byte { return reply(q, 0, false, rrA("192.0.2.1")) })
	r := Resolve(context.Background(), DNSQuery{
		Server: addr, Name: "sensor.example.com", Expect: "10.0.0.5",
	})
	if r.OK() {
		t.Fatal("a wrong answer passed the test")
	}
	if r.Err == nil || r.RCode != "NOERROR" {
		t.Fatalf("want a NOERROR result with an error explaining the mismatch, got %+v", r)
	}
}

func TestResolveTimesOutAgainstASilentServer(t *testing.T) {
	// TEST-NET-1 (RFC 5737) is genuinely silent, so this is a real timeout.
	start := time.Now()
	r := Resolve(context.Background(), DNSQuery{
		Server: "192.0.2.1", Name: "example.com", Timeout: 300 * time.Millisecond,
	})
	if r.Err == nil {
		t.Fatal("a silent server answered")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v to give up on a 300ms timeout", elapsed)
	}
}

func TestReadNameRefusesACompressionLoop(t *testing.T) {
	msg := make([]byte, 14)
	binary.BigEndian.PutUint16(msg[12:], 0xc00c) // a pointer to itself
	if _, _, err := readName(msg, 12); err == nil {
		t.Fatal("a self-referential name parsed without error")
	}
}

func TestDNSTypeNames(t *testing.T) {
	if DNSType("aaaa") != TypeAAAA || DNSType(" SRV ") != TypeSRV {
		t.Fatal("record type names are not being normalised")
	}
	if DNSType("nonsense") != TypeA {
		t.Fatal("an unknown type should fall back to A")
	}
}
