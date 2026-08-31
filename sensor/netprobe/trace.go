package netprobe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Hop is one step along the path.
type Hop struct {
	TTL     int
	Addr    string
	Name    string
	RTT     time.Duration
	Timeout bool
}

// TraceResult is the path to a target. A sensor traces the gateway, the
// resolver and one or two applications, so that when latency moves the change
// can be pinned to the hop it appeared at.
type TraceResult struct {
	Target  string
	Hops    []Hop
	Reached bool
	Method  string // "icmp" or "traceroute"
	Err     error
}

// Traceroute walks the path with increasing TTLs. It prefers its own ICMP
// socket, which needs privilege, and falls back to the system traceroute where
// that is not available — a sensor that has been dropped to an unprivileged
// user should lose the detail, not the test.
func Traceroute(ctx context.Context, target string, maxHops int, perHop time.Duration) TraceResult {
	if maxHops <= 0 {
		maxHops = 20
	}
	if perHop <= 0 {
		perHop = time.Second
	}
	res := TraceResult{Target: target, Method: "icmp"}

	ip, err := resolveIPv4(ctx, target)
	if err != nil {
		res.Err = err
		return res
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		if sys := systemTraceroute(ctx, target, maxHops, perHop); sys.Err == nil || len(sys.Hops) > 0 {
			return sys
		}
		res.Err = fmt.Errorf("traceroute needs a raw socket or the traceroute command: %w", err)
		return res
	}
	defer conn.Close()
	pc := conn.IPv4PacketConn()

	id := os.Getpid() & 0xffff
	buf := make([]byte, 1500)
	for ttl := 1; ttl <= maxHops; ttl++ {
		if err := ctx.Err(); err != nil {
			res.Err = err
			return res
		}
		if err := pc.SetTTL(ttl); err != nil {
			res.Err = err
			return res
		}
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Body: &icmp.Echo{ID: id, Seq: ttl, Data: []byte("crossbreeder-sensor-trace")},
		}
		wire, err := msg.Marshal(nil)
		if err != nil {
			res.Err = err
			return res
		}
		start := time.Now()
		if _, err := conn.WriteTo(wire, &net.IPAddr{IP: ip}); err != nil {
			res.Err = err
			return res
		}

		hop := Hop{TTL: ttl, Timeout: true}
		limit := time.Now().Add(perHop)
		for time.Now().Before(limit) {
			conn.SetReadDeadline(limit)
			n, peer, err := conn.ReadFrom(buf)
			if err != nil {
				break
			}
			kind, hopID, hopSeq, ok := classifyICMP(buf[:n])
			if !ok || hopID != id || hopSeq != ttl {
				continue
			}
			hop = Hop{TTL: ttl, Addr: peer.String(), RTT: time.Since(start)}
			hop.Name = reverseName(ctx, hop.Addr)
			if kind == icmpEchoReply {
				res.Hops = append(res.Hops, hop)
				res.Reached = true
				return res
			}
			break
		}
		res.Hops = append(res.Hops, hop)
	}
	return res
}

const (
	icmpEchoReply = iota
	icmpTimeExceeded
)

// classifyICMP reports what a received message is and which probe it belongs
// to. A time-exceeded quotes the packet that expired, so our own echo header
// comes back inside it and identifies the hop.
func classifyICMP(b []byte) (kind, id, seq int, ok bool) {
	msg, err := icmp.ParseMessage(1, b) // 1 = IPv4 ICMP
	if err != nil {
		return 0, 0, 0, false
	}
	switch body := msg.Body.(type) {
	case *icmp.Echo:
		if msg.Type == ipv4.ICMPTypeEchoReply {
			return icmpEchoReply, body.ID, body.Seq, true
		}
	case *icmp.TimeExceeded:
		if id, seq, ok := innerEcho(body.Data); ok {
			return icmpTimeExceeded, id, seq, true
		}
	}
	return 0, 0, 0, false
}

// innerEcho pulls the identifier and sequence out of the packet quoted inside
// an ICMP error.
func innerEcho(data []byte) (id, seq int, ok bool) {
	if len(data) < 20 {
		return 0, 0, false
	}
	ihl := int(data[0]&0x0f) * 4
	if ihl < 20 || len(data) < ihl+8 {
		return 0, 0, false
	}
	inner := data[ihl:]
	if inner[0] != 8 { // the quoted packet must be our echo request
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint16(inner[4:])), int(binary.BigEndian.Uint16(inner[6:])), true
}

func resolveIPv4(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
		return nil, fmt.Errorf("%s is not an IPv4 address", host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4, nil
		}
	}
	return nil, fmt.Errorf("%s has no IPv4 address", host)
}

// reverseName is best-effort and short-fused: a hop with no PTR record must
// not add a second to every trace.
func reverseName(ctx context.Context, addr string) string {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, addr)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

func systemTraceroute(ctx context.Context, target string, maxHops int, perHop time.Duration) TraceResult {
	res := TraceResult{Target: target, Method: "traceroute"}
	bin, err := exec.LookPath("traceroute")
	if err != nil {
		res.Err = errors.New("no raw socket and no traceroute command")
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(maxHops)*perHop+5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-n", "-q", "1",
		"-m", strconv.Itoa(maxHops), "-w", strconv.Itoa(max(1, int(perHop.Seconds()))), target).Output()
	if len(out) == 0 && err != nil {
		res.Err = err
		return res
	}
	res.Hops = parseTracerouteOutput(string(out))
	for _, h := range res.Hops {
		if h.Addr == target {
			res.Reached = true
		}
	}
	return res
}

// parseTracerouteOutput reads the numeric output of traceroute -n.
func parseTracerouteOutput(out string) []Hop {
	var hops []Hop
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ttl, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		hop := Hop{TTL: ttl}
		if fields[1] == "*" {
			hop.Timeout = true
			hops = append(hops, hop)
			continue
		}
		hop.Addr = fields[1]
		for i := 2; i < len(fields)-1; i++ {
			if fields[i+1] == "ms" {
				if ms, err := strconv.ParseFloat(fields[i], 64); err == nil {
					hop.RTT = time.Duration(ms * float64(time.Millisecond))
					break
				}
			}
		}
		hops = append(hops, hop)
	}
	return hops
}
