package netprobe

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// DHCP message types (RFC 2132 §9.6).
const (
	dhcpDiscover = 1
	dhcpOffer    = 2
	dhcpRequest  = 3
	dhcpDecline  = 4
	dhcpACK      = 5
	dhcpNAK      = 6
	dhcpRelease  = 7
)

// DHCP options we read or send.
const (
	optSubnetMask   = 1
	optRouter       = 3
	optDNS          = 6
	optHostname     = 12
	optDomainName   = 15
	optRequestedIP  = 50
	optLeaseTime    = 51
	optMessageType  = 53
	optServerID     = 54
	optParamRequest = 55
	optMessage      = 56
	optRenewalTime  = 58
	optClientID     = 61
	optEnd          = 255
)

// DHCPResult is a full four-way exchange, timed at each step. A network where
// the offer takes three seconds is a network where every client blames the
// wireless, so the phases have to be separable.
type DHCPResult struct {
	Offer   time.Duration // DISCOVER sent to OFFER received
	Ack     time.Duration // REQUEST sent to ACK received
	Total   time.Duration
	Retries int

	YourIP     net.IP
	SubnetMask net.IP
	Router     net.IP
	DNS        []net.IP
	Domain     string
	ServerID   net.IP
	Lease      time.Duration
	// Offers holds every server that answered. More than one on a network
	// that should have a single scope is a rogue-server finding.
	Offers []string
	Err    error
}

// OK reports whether the sensor was actually given a usable address.
func (r DHCPResult) OK() bool { return r.Err == nil && r.YourIP != nil }

// DHCPClient runs the exchange over a packet connection. The connection is
// supplied rather than opened here so the exchange itself can be tested over
// loopback, and so a platform that needs a bound raw socket can provide one.
type DHCPClient struct {
	Conn     net.PacketConn
	Server   net.Addr // where to send: the broadcast address on a real network
	MAC      net.HardwareAddr
	Hostname string
	Timeout  time.Duration // per message, default 4s
	Retries  int           // extra attempts per message, default 1
	// Release returns the lease when the exchange succeeds. On by default in
	// the sensor: a probe that runs every five minutes must not eat a
	// /24's worth of pool addresses in a day.
	Release bool
}

// Probe performs DISCOVER/OFFER/REQUEST/ACK and reports what came back.
func (c *DHCPClient) Probe(ctx context.Context) DHCPResult {
	var res DHCPResult
	if c.Conn == nil || c.Server == nil {
		res.Err = errors.New("no DHCP transport: the interface could not be opened")
		return res
	}
	if c.Timeout <= 0 {
		c.Timeout = 4 * time.Second
	}
	if c.Retries == 0 {
		c.Retries = 1
	}
	mac := c.MAC
	if len(mac) != 6 {
		// A locally administered address, so a sensor without a readable MAC
		// still transacts rather than failing, and cannot collide with real
		// hardware.
		mac = randomMAC()
	}

	xid := randomXID()
	start := time.Now()

	discover := dhcpMessage{op: 1, xid: xid, mac: mac, msgType: dhcpDiscover, hostname: c.Hostname, broadcast: true}
	offer, tries, err := c.exchange(ctx, discover, dhcpOffer, &res)
	res.Retries += tries
	if err != nil {
		res.Err = fmt.Errorf("no DHCP offer: %w", err)
		res.Total = time.Since(start)
		return res
	}
	res.Offer = time.Since(start)
	res.ServerID = offer.serverID()
	res.YourIP = offer.yiaddr

	reqStart := time.Now()
	request := dhcpMessage{
		op: 1, xid: xid, mac: mac, msgType: dhcpRequest, hostname: c.Hostname, broadcast: true,
		requestedIP: offer.yiaddr, serverIdent: offer.serverID(),
	}
	ack, tries, err := c.exchange(ctx, request, dhcpACK, nil)
	res.Retries += tries
	if err != nil {
		res.Err = fmt.Errorf("no DHCP acknowledgement: %w", err)
		res.Total = time.Since(start)
		return res
	}
	res.Ack = time.Since(reqStart)
	res.Total = time.Since(start)

	res.YourIP = ack.yiaddr
	res.SubnetMask = ack.ipOption(optSubnetMask)
	res.Router = ack.ipOption(optRouter)
	res.DNS = ack.ipListOption(optDNS)
	res.Domain = ack.stringOption(optDomainName)
	if id := ack.serverID(); id != nil {
		res.ServerID = id
	}
	if secs := ack.u32Option(optLeaseTime); secs > 0 {
		res.Lease = time.Duration(secs) * time.Second
	}

	if c.Release && res.ServerID != nil {
		release := dhcpMessage{
			op: 1, xid: randomXID(), mac: mac, msgType: dhcpRelease,
			ciaddr: res.YourIP, serverIdent: res.ServerID,
		}
		// A release is not acknowledged, so it is sent and forgotten.
		c.Conn.WriteTo(release.marshal(), c.Server)
	}
	return res
}

// exchange sends one message and waits for the wanted reply type, retrying on
// silence. Replies for other transactions on a busy broadcast domain are
// ignored rather than mistaken for ours.
func (c *DHCPClient) exchange(ctx context.Context, msg dhcpMessage, want byte, collect *DHCPResult) (dhcpMessage, int, error) {
	var lastErr error
	buf := make([]byte, 1500)
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return dhcpMessage{}, attempt, err
		}
		if _, err := c.Conn.WriteTo(msg.marshal(), c.Server); err != nil {
			return dhcpMessage{}, attempt, err
		}
		limit := deadline(ctx, c.Timeout)
		if err := c.Conn.SetReadDeadline(limit); err != nil {
			return dhcpMessage{}, attempt, err
		}
		for time.Now().Before(limit) {
			n, from, err := c.Conn.ReadFrom(buf)
			if err != nil {
				lastErr = err
				break
			}
			reply, err := parseDHCP(buf[:n])
			if err != nil || reply.xid != msg.xid || !macEqual(reply.mac, msg.mac) {
				continue
			}
			if reply.msgType == dhcpNAK {
				text := reply.stringOption(optMessage)
				if text == "" {
					text = "the server refused the request"
				}
				return dhcpMessage{}, attempt, errors.New(text)
			}
			if reply.msgType != want {
				continue
			}
			if collect != nil {
				collect.Offers = appendUnique(collect.Offers, describeOffer(reply, from))
			}
			return reply, attempt, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no reply within %v", c.Timeout)
		}
	}
	return dhcpMessage{}, c.Retries, lastErr
}

func describeOffer(m dhcpMessage, from net.Addr) string {
	id := m.serverID()
	if id == nil {
		if host, _, err := net.SplitHostPort(from.String()); err == nil {
			return host
		}
		return from.String()
	}
	return id.String() + " offering " + m.yiaddr.String()
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// dhcpMessage is a BOOTP frame carrying DHCP options.
type dhcpMessage struct {
	op          byte
	xid         uint32
	ciaddr      net.IP
	yiaddr      net.IP
	mac         net.HardwareAddr
	msgType     byte
	hostname    string
	requestedIP net.IP
	serverIdent net.IP
	broadcast   bool
	options     map[byte][]byte
}

var dhcpMagic = [4]byte{99, 130, 83, 99}

func (m dhcpMessage) marshal() []byte {
	b := make([]byte, 240, 300)
	b[0] = m.op
	b[1] = 1 // Ethernet
	b[2] = 6 // MAC length
	binary.BigEndian.PutUint32(b[4:], m.xid)
	if m.broadcast {
		binary.BigEndian.PutUint16(b[10:], 0x8000)
	}
	copy(b[12:16], to4(m.ciaddr))
	copy(b[28:34], m.mac)
	copy(b[236:240], dhcpMagic[:])

	b = append(b, optMessageType, 1, m.msgType)
	if ip := to4(m.requestedIP); ip != nil {
		b = append(b, optRequestedIP, 4)
		b = append(b, ip...)
	}
	if ip := to4(m.serverIdent); ip != nil {
		b = append(b, optServerID, 4)
		b = append(b, ip...)
	}
	if m.hostname != "" {
		h := m.hostname
		if len(h) > 63 {
			h = h[:63]
		}
		b = append(b, optHostname, byte(len(h)))
		b = append(b, h...)
	}
	// Client identifier: type 1 (Ethernet) plus the MAC, as a normal client
	// sends, so the lease looks like every other lease in the server's log.
	b = append(b, optClientID, 7, 1)
	b = append(b, m.mac...)
	b = append(b, optParamRequest, 5, optSubnetMask, optRouter, optDNS, optDomainName, optLeaseTime)
	b = append(b, optEnd)
	// Pad to the 300-byte minimum some relays and servers still insist on.
	for len(b) < 300 {
		b = append(b, 0)
	}
	return b
}

func parseDHCP(b []byte) (dhcpMessage, error) {
	var m dhcpMessage
	if len(b) < 240 {
		return m, errors.New("short DHCP message")
	}
	if [4]byte(b[236:240]) != dhcpMagic {
		return m, errors.New("not a DHCP message")
	}
	m.op = b[0]
	m.xid = binary.BigEndian.Uint32(b[4:])
	m.ciaddr = net.IP(append([]byte(nil), b[12:16]...))
	m.yiaddr = net.IP(append([]byte(nil), b[16:20]...))
	m.mac = net.HardwareAddr(append([]byte(nil), b[28:34]...))
	m.options = map[byte][]byte{}

	for i := 240; i < len(b); {
		code := b[i]
		if code == optEnd {
			break
		}
		if code == 0 { // pad
			i++
			continue
		}
		if i+2 > len(b) {
			return m, errors.New("truncated option header")
		}
		length := int(b[i+1])
		if i+2+length > len(b) {
			return m, errors.New("option overruns message")
		}
		m.options[code] = append([]byte(nil), b[i+2:i+2+length]...)
		i += 2 + length
	}
	if t := m.options[optMessageType]; len(t) == 1 {
		m.msgType = t[0]
	}
	return m, nil
}

func (m dhcpMessage) serverID() net.IP { return m.ipOption(optServerID) }

// stringOption reads a text option. Plenty of servers NUL-terminate these —
// the domain name especially — and a trailing NUL would otherwise travel all
// the way to the dashboard.
func (m dhcpMessage) stringOption(code byte) string {
	return strings.Trim(string(m.options[code]), "\x00 ")
}

func (m dhcpMessage) ipOption(code byte) net.IP {
	if v, ok := m.options[code]; ok && len(v) >= 4 {
		return net.IP(append([]byte(nil), v[:4]...))
	}
	return nil
}

func (m dhcpMessage) ipListOption(code byte) []net.IP {
	v := m.options[code]
	var out []net.IP
	for i := 0; i+4 <= len(v); i += 4 {
		out = append(out, net.IP(append([]byte(nil), v[i:i+4]...)))
	}
	return out
}

func (m dhcpMessage) u32Option(code byte) uint32 {
	if v, ok := m.options[code]; ok && len(v) >= 4 {
		return binary.BigEndian.Uint32(v)
	}
	return 0
}

func to4(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	return ip.To4()
}

func macEqual(a, b net.HardwareAddr) bool {
	if len(a) < 6 || len(b) < 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func randomXID() uint32 {
	var b [4]byte
	rand.Read(b[:])
	return binary.BigEndian.Uint32(b[:])
}

func randomMAC() net.HardwareAddr {
	var b [6]byte
	rand.Read(b[:])
	b[0] = (b[0] | 0x02) &^ 0x01 // locally administered, unicast
	return net.HardwareAddr(b[:])
}
