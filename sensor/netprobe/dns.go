package netprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DNS record types we can ask for. A resolution test is normally an A lookup,
// but a site that reports "the internet is broken" often turns out to have one
// resolver that has lost a zone, so the type belongs in the test definition.
const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeSOA   uint16 = 6
	TypePTR   uint16 = 12
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeAAAA  uint16 = 28
	TypeSRV   uint16 = 33
)

var dnsTypeNames = map[string]uint16{
	"A": TypeA, "NS": TypeNS, "CNAME": TypeCNAME, "SOA": TypeSOA,
	"PTR": TypePTR, "MX": TypeMX, "TXT": TypeTXT, "AAAA": TypeAAAA, "SRV": TypeSRV,
}

// DNSType maps a record type name to its wire value. An unknown name resolves
// to A rather than failing: a typo in a template should not read as a broken
// network.
func DNSType(name string) uint16 {
	if t, ok := dnsTypeNames[strings.ToUpper(strings.TrimSpace(name))]; ok {
		return t
	}
	return TypeA
}

// DNSQuery is one lookup against one server.
type DNSQuery struct {
	Server  string // "192.168.1.1", "192.168.1.1:53", or a DoH URL
	Name    string
	Type    uint16
	Proto   string // udp (default), tcp, tls (DoT), https (DoH)
	Timeout time.Duration
	// Expect, when set, is an answer that must appear for the lookup to pass.
	// This catches a resolver that answers quickly with the wrong thing — a
	// captive portal, or a filtering resolver handing back its own address.
	Expect string
}

// DNSResult is what one lookup produced.
type DNSResult struct {
	RTT       time.Duration
	Answers   []string
	RCode     string
	TTL       uint32 // the shortest TTL in the answer section
	Truncated bool
	Server    string
	Proto     string
	Err       error
}

// OK reports whether the lookup answered usefully: a response, NOERROR, and at
// least one record.
func (r DNSResult) OK() bool {
	return r.Err == nil && r.RCode == "NOERROR" && len(r.Answers) > 0
}

var rcodeNames = [...]string{"NOERROR", "FORMERR", "SERVFAIL", "NXDOMAIN", "NOTIMP", "REFUSED"}

func rcodeName(c uint16) string {
	if int(c) < len(rcodeNames) {
		return rcodeNames[c]
	}
	return "RCODE" + strconv.Itoa(int(c))
}

// Resolve performs one lookup and times it. A UDP answer that comes back
// truncated is retried over TCP, as a resolver client must.
func Resolve(ctx context.Context, q DNSQuery) DNSResult {
	if q.Timeout <= 0 {
		q.Timeout = 3 * time.Second
	}
	if q.Type == 0 {
		q.Type = TypeA
	}
	if q.Name == "" {
		return DNSResult{Err: errors.New("no name to look up")}
	}
	proto := strings.ToLower(q.Proto)
	if proto == "" {
		proto = "udp"
	}

	msg, err := encodeQuery(q.Name, q.Type, uint16(rand.Intn(0xffff)))
	if err != nil {
		return DNSResult{Err: err, Proto: proto}
	}

	start := time.Now()
	var reply []byte
	switch proto {
	case "udp":
		reply, err = dnsExchangeUDP(ctx, addPort(q.Server, "53"), msg, q.Timeout)
	case "tcp":
		reply, err = dnsExchangeStream(ctx, addPort(q.Server, "53"), msg, q.Timeout, nil)
	case "tls", "dot":
		host := addPort(q.Server, "853")
		name, _, _ := net.SplitHostPort(host)
		reply, err = dnsExchangeStream(ctx, host, msg, q.Timeout, &tls.Config{ServerName: name})
	case "https", "doh":
		reply, err = dnsExchangeHTTPS(ctx, q.Server, msg, q.Timeout)
	default:
		err = fmt.Errorf("unknown DNS transport %q", q.Proto)
	}
	res := DNSResult{RTT: time.Since(start), Server: q.Server, Proto: proto}
	if err != nil {
		res.Err = err
		return res
	}

	parsed, err := parseResponse(reply)
	if err != nil {
		res.Err = err
		return res
	}
	if parsed.truncated && proto == "udp" {
		retry := time.Now()
		if reply, err = dnsExchangeStream(ctx, addPort(q.Server, "53"), msg, q.Timeout, nil); err == nil {
			if p2, err2 := parseResponse(reply); err2 == nil {
				parsed = p2
				res.RTT += time.Since(retry)
				res.Proto = "udp+tcp"
			}
		}
	}
	res.Answers, res.RCode, res.TTL, res.Truncated = parsed.answers, rcodeName(parsed.rcode), parsed.ttl, parsed.truncated
	if q.Expect != "" && res.RCode == "NOERROR" {
		want := strings.ToLower(strings.TrimSuffix(q.Expect, "."))
		found := false
		for _, a := range res.Answers {
			if strings.EqualFold(strings.TrimSuffix(a, "."), want) {
				found = true
			}
		}
		if !found {
			res.Err = fmt.Errorf("answered %s, expected %s", strings.Join(res.Answers, ", "), q.Expect)
		}
	}
	return res
}

func addPort(server, def string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return "127.0.0.1:" + def
	}
	if strings.HasPrefix(server, "[") && strings.Contains(server, "]:") {
		return server
	}
	if ip := net.ParseIP(server); ip != nil {
		if ip.To4() == nil {
			return "[" + server + "]:" + def
		}
		return server + ":" + def
	}
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	return server + ":" + def
}

func dnsExchangeUDP(ctx context.Context, server string, msg []byte, timeout time.Duration) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", server)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline(ctx, timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func dnsExchangeStream(ctx context.Context, server string, msg []byte, timeout time.Duration, tlsCfg *tls.Config) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", server)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline(ctx, timeout)); err != nil {
		return nil, err
	}
	var rw net.Conn = conn
	if tlsCfg != nil {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		rw = tc
	}
	framed := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(framed, uint16(len(msg)))
	copy(framed[2:], msg)
	if _, err := rw.Write(framed); err != nil {
		return nil, err
	}
	var length [2]byte
	if _, err := io.ReadFull(rw, length[:]); err != nil {
		return nil, err
	}
	reply := make([]byte, binary.BigEndian.Uint16(length[:]))
	if _, err := io.ReadFull(rw, reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func dnsExchangeHTTPS(ctx context.Context, endpoint string, msg []byte, timeout time.Duration) ([]byte, error) {
	if _, err := url.Parse(endpoint); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resolver answered HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<16))
}

func deadline(ctx context.Context, timeout time.Duration) time.Time {
	t := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(t) {
		return d
	}
	return t
}

// encodeQuery builds a standard recursive query for one name.
func encodeQuery(name string, qtype, id uint16) ([]byte, error) {
	var b bytes.Buffer
	var hdr [12]byte
	binary.BigEndian.PutUint16(hdr[0:], id)
	binary.BigEndian.PutUint16(hdr[2:], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(hdr[4:], 1)      // one question
	b.Write(hdr[:])
	if err := writeName(&b, name); err != nil {
		return nil, err
	}
	var tail [4]byte
	binary.BigEndian.PutUint16(tail[0:], qtype)
	binary.BigEndian.PutUint16(tail[2:], 1) // class IN
	b.Write(tail[:])
	return b.Bytes(), nil
}

func writeName(b *bytes.Buffer, name string) error {
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		b.WriteByte(0)
		return nil
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("bad label %q in %q", label, name)
		}
		b.WriteByte(byte(len(label)))
		b.WriteString(label)
	}
	b.WriteByte(0)
	return nil
}

type dnsResponse struct {
	rcode     uint16
	truncated bool
	answers   []string
	ttl       uint32
}

func parseResponse(b []byte) (dnsResponse, error) {
	var out dnsResponse
	if len(b) < 12 {
		return out, errors.New("short DNS response")
	}
	flags := binary.BigEndian.Uint16(b[2:])
	out.rcode = flags & 0x0f
	out.truncated = flags&0x0200 != 0
	qd := int(binary.BigEndian.Uint16(b[4:]))
	an := int(binary.BigEndian.Uint16(b[6:]))

	off := 12
	for i := 0; i < qd; i++ {
		var err error
		if _, off, err = readName(b, off); err != nil {
			return out, err
		}
		off += 4
		if off > len(b) {
			return out, errors.New("truncated question section")
		}
	}
	for i := 0; i < an; i++ {
		var err error
		if _, off, err = readName(b, off); err != nil {
			return out, err
		}
		if off+10 > len(b) {
			return out, errors.New("truncated answer section")
		}
		rtype := binary.BigEndian.Uint16(b[off:])
		ttl := binary.BigEndian.Uint32(b[off+4:])
		rdlen := int(binary.BigEndian.Uint16(b[off+8:]))
		off += 10
		if off+rdlen > len(b) {
			return out, errors.New("answer overruns response")
		}
		if out.ttl == 0 || ttl < out.ttl {
			out.ttl = ttl
		}
		if s := renderRData(b, off, rdlen, rtype); s != "" {
			out.answers = append(out.answers, s)
		}
		off += rdlen
	}
	return out, nil
}

func renderRData(b []byte, off, rdlen int, rtype uint16) string {
	data := b[off : off+rdlen]
	switch rtype {
	case TypeA:
		if rdlen == 4 {
			return net.IP(data).String()
		}
	case TypeAAAA:
		if rdlen == 16 {
			return net.IP(data).String()
		}
	case TypeCNAME, TypeNS, TypePTR, TypeSOA:
		if name, _, err := readName(b, off); err == nil {
			return name
		}
	case TypeMX:
		if rdlen > 2 {
			if name, _, err := readName(b, off+2); err == nil {
				return strconv.Itoa(int(binary.BigEndian.Uint16(data))) + " " + name
			}
		}
	case TypeSRV:
		if rdlen > 6 {
			if name, _, err := readName(b, off+6); err == nil {
				return name + ":" + strconv.Itoa(int(binary.BigEndian.Uint16(data[4:])))
			}
		}
	case TypeTXT:
		var parts []string
		for i := 0; i < rdlen; {
			n := int(data[i])
			if i+1+n > rdlen {
				break
			}
			parts = append(parts, string(data[i+1:i+1+n]))
			i += 1 + n
		}
		return strings.Join(parts, "")
	}
	return ""
}

// readName decodes a possibly compressed name, returning it and the offset
// just past it in the message. Pointers are followed on a budget, so a corrupt
// or hostile response cannot loop us forever.
func readName(b []byte, off int) (string, int, error) {
	var labels []string
	next := -1
	for budget := 0; budget < 128; budget++ {
		if off >= len(b) {
			return "", 0, errors.New("name runs past end of message")
		}
		n := int(b[off])
		switch {
		case n == 0:
			off++
			if next >= 0 {
				off = next
			}
			return strings.Join(labels, ".") + ".", off, nil
		case n&0xc0 == 0xc0:
			if off+1 >= len(b) {
				return "", 0, errors.New("truncated compression pointer")
			}
			ptr := int(binary.BigEndian.Uint16(b[off:]) & 0x3fff)
			if next < 0 {
				next = off + 2
			}
			if ptr >= len(b) {
				return "", 0, errors.New("compression pointer past end of message")
			}
			off = ptr
		default:
			if off+1+n > len(b) {
				return "", 0, errors.New("label runs past end of message")
			}
			labels = append(labels, string(b[off+1:off+1+n]))
			off += 1 + n
		}
	}
	return "", 0, errors.New("compression loop in name")
}
