//go:build !windows

package ap

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

var pingPayload = []byte("crossbreeder-engine-probe")

// Ping sends one ICMP echo request and waits up to timeout for the reply.
//
// It prefers the unprivileged datagram socket ("udp4"), which macOS allows for
// any user and Linux allows for gids inside net.ipv4.ping_group_range, and
// falls back to a raw socket when running as root.
func Ping(ctx context.Context, host string, timeout time.Duration) PingResult {
	ip, err := resolve4(ctx, host)
	if err != nil {
		return PingResult{Err: err}
	}

	conn, unprivileged, err := listenICMP()
	if err != nil {
		noteICMPUnavailable(err)
		return PingResult{Err: err}
	}
	defer conn.Close()

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: &icmp.Echo{ID: id, Seq: 1, Data: pingPayload},
	}
	wire, err := msg.Marshal(nil)
	if err != nil {
		return PingResult{Err: err}
	}

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return PingResult{Err: err}
	}

	var dst net.Addr = &net.UDPAddr{IP: ip}
	if !unprivileged {
		dst = &net.IPAddr{IP: ip}
	}

	start := time.Now()
	if _, err := conn.WriteTo(wire, dst); err != nil {
		return PingResult{Err: err}
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return PingResult{RTT: time.Since(start)} // timeout: host is silent
		}
		// A raw socket sees every ICMP packet on the box, so replies have to be
		// matched to the request rather than assumed.
		if peerIP(peer) == nil || !peerIP(peer).Equal(ip) {
			continue
		}
		reply, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), buf[:n])
		if err != nil || reply.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := reply.Body.(*icmp.Echo)
		if !ok || !bytes.Equal(echo.Data, pingPayload) {
			continue
		}
		// The kernel rewrites the ID on unprivileged sockets, so it is only
		// meaningful on the raw path.
		if !unprivileged && echo.ID != id {
			continue
		}
		return PingResult{Alive: true, RTT: time.Since(start)}
	}
}

func listenICMP() (*icmp.PacketConn, bool, error) {
	if c, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		return c, true, nil
	}
	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, false, fmt.Errorf("no ICMP socket available (try -probe tcp, or widen net.ipv4.ping_group_range): %w", err)
	}
	return c, false, nil
}

func resolve4(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
		return nil, fmt.Errorf("no ICMP for IPv6 %s", host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4, nil
		}
	}
	return nil, fmt.Errorf("no IPv4 address for %s", host)
}

func peerIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}
