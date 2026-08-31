package netprobe

import (
	"context"
	"net"
	"testing"
	"time"
)

// fakeDHCPServer answers on loopback the way a scope would. handler decides
// what to send back for each message type; returning nil stays silent.
func fakeDHCPServer(t *testing.T, handler func(m dhcpMessage) [][]byte) (net.PacketConn, net.Addr) {
	t.Helper()
	srv, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := srv.ReadFrom(buf)
			if err != nil {
				return
			}
			m, err := parseDHCP(buf[:n])
			if err != nil {
				continue
			}
			for _, out := range handler(m) {
				srv.WriteTo(out, from)
			}
		}
	}()
	client, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client, srv.LocalAddr()
}

// scopeReply builds the OFFER or ACK a normal server would send.
func scopeReply(m dhcpMessage, msgType byte, yourIP string) []byte {
	out := make([]byte, 240, 300)
	out[0] = 2 // BOOTREPLY
	out[1], out[2] = 1, 6
	copy(out[4:8], []byte{byte(m.xid >> 24), byte(m.xid >> 16), byte(m.xid >> 8), byte(m.xid)})
	copy(out[16:20], net.ParseIP(yourIP).To4())
	copy(out[28:34], m.mac)
	copy(out[236:240], dhcpMagic[:])

	out = append(out, optMessageType, 1, msgType)
	out = append(out, optServerID, 4)
	out = append(out, net.ParseIP("10.20.30.1").To4()...)
	out = append(out, optSubnetMask, 4)
	out = append(out, net.ParseIP("255.255.255.0").To4()...)
	out = append(out, optRouter, 4)
	out = append(out, net.ParseIP("10.20.30.1").To4()...)
	out = append(out, optDNS, 8)
	out = append(out, net.ParseIP("10.20.30.2").To4()...)
	out = append(out, net.ParseIP("10.20.30.3").To4()...)
	out = append(out, optDomainName, 11)
	out = append(out, "site.local"...)
	out = append(out, 0)
	out = append(out, optLeaseTime, 4, 0, 0, 0x1c, 0x20) // 7200s
	out = append(out, optEnd)
	return out
}

func TestDHCPProbeCompletesTheExchange(t *testing.T) {
	conn, server := fakeDHCPServer(t, func(m dhcpMessage) [][]byte {
		switch m.msgType {
		case dhcpDiscover:
			return [][]byte{scopeReply(m, dhcpOffer, "10.20.30.55")}
		case dhcpRequest:
			return [][]byte{scopeReply(m, dhcpACK, "10.20.30.55")}
		}
		return nil
	})

	c := &DHCPClient{Conn: conn, Server: server, Hostname: "sensor-lobby", Timeout: time.Second}
	r := c.Probe(context.Background())
	if !r.OK() {
		t.Fatalf("exchange failed: %v", r.Err)
	}
	if r.YourIP.String() != "10.20.30.55" {
		t.Errorf("address = %v", r.YourIP)
	}
	if r.Router.String() != "10.20.30.1" || len(r.DNS) != 2 {
		t.Errorf("router = %v, DNS = %v", r.Router, r.DNS)
	}
	if r.Domain != "site.local" {
		t.Errorf("domain = %q", r.Domain)
	}
	if r.Lease != 2*time.Hour {
		t.Errorf("lease = %v, want 2h", r.Lease)
	}
	if r.Offer <= 0 || r.Ack <= 0 || r.Total < r.Offer {
		t.Errorf("phase timings are not coherent: %+v", r)
	}
	if len(r.Offers) != 1 {
		t.Errorf("offers = %v", r.Offers)
	}
}

func TestDHCPProbeReportsSilence(t *testing.T) {
	conn, server := fakeDHCPServer(t, func(dhcpMessage) [][]byte { return nil })
	c := &DHCPClient{Conn: conn, Server: server, Timeout: 150 * time.Millisecond, Retries: 1}

	start := time.Now()
	r := c.Probe(context.Background())
	if r.OK() {
		t.Fatal("a silent scope produced a lease")
	}
	if r.Err == nil || !contains(r.Err.Error(), "no DHCP offer") {
		t.Fatalf("error = %v, want it to name the missing offer", r.Err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("two 150ms attempts took %v", elapsed)
	}
}

func TestDHCPProbeReportsANak(t *testing.T) {
	conn, server := fakeDHCPServer(t, func(m dhcpMessage) [][]byte {
		if m.msgType == dhcpDiscover {
			return [][]byte{scopeReply(m, dhcpOffer, "10.20.30.55")}
		}
		out := scopeReply(m, dhcpNAK, "0.0.0.0")
		out = append(out[:len(out)-1], optMessage, 9)
		out = append(out, "pool full"...)
		return [][]byte{append(out, optEnd)}
	})
	c := &DHCPClient{Conn: conn, Server: server, Timeout: 500 * time.Millisecond}
	r := c.Probe(context.Background())
	if r.OK() {
		t.Fatal("a NAK was treated as a lease")
	}
	if !contains(r.Err.Error(), "pool full") {
		t.Fatalf("error = %v, want the server's own text", r.Err)
	}
}

// A busy broadcast domain carries other clients' traffic. Replies for another
// transaction must not be mistaken for ours.
func TestDHCPProbeIgnoresOtherTransactions(t *testing.T) {
	conn, server := fakeDHCPServer(t, func(m dhcpMessage) [][]byte {
		// Another client's exchange, sent first, on the same broadcast domain.
		stranger := m
		stranger.xid = m.xid + 1
		stranger.mac = net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}
		switch m.msgType {
		case dhcpDiscover:
			return [][]byte{
				scopeReply(stranger, dhcpOffer, "10.20.30.99"),
				scopeReply(m, dhcpOffer, "10.20.30.55"),
			}
		case dhcpRequest:
			return [][]byte{scopeReply(m, dhcpACK, "10.20.30.55")}
		}
		return nil
	})
	c := &DHCPClient{Conn: conn, Server: server, Timeout: time.Second}
	if r := c.Probe(context.Background()); !r.OK() || r.YourIP.String() != "10.20.30.55" {
		t.Fatalf("result = %+v", r)
	}
}

func TestDHCPMessageRoundTrips(t *testing.T) {
	mac, _ := net.ParseMAC("b8:27:eb:11:22:33")
	m := dhcpMessage{
		op: 1, xid: 0x12345678, mac: mac, msgType: dhcpRequest, hostname: "sensor-1",
		requestedIP: net.ParseIP("10.0.0.9"), serverIdent: net.ParseIP("10.0.0.1"),
	}
	back, err := parseDHCP(m.marshal())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if back.xid != m.xid || back.msgType != dhcpRequest || !macEqual(back.mac, mac) {
		t.Fatalf("round trip lost the header: %+v", back)
	}
	if string(back.options[optHostname]) != "sensor-1" {
		t.Errorf("hostname option = %q", back.options[optHostname])
	}
	if back.ipOption(optRequestedIP).String() != "10.0.0.9" {
		t.Errorf("requested IP = %v", back.ipOption(optRequestedIP))
	}
	if len(m.marshal()) < 300 {
		t.Error("the message was not padded to the 300-byte minimum")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
