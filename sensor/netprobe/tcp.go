package netprobe

import (
	"context"
	"net"
	"time"
)

// TCPResult is one connect attempt to a port.
type TCPResult struct {
	Address string
	RTT     time.Duration
	Open    bool
	Err     error
}

// TCPConnect times a TCP handshake. It is the cheapest honest test of "is that
// service listening", and it works where ICMP is filtered.
func TCPConnect(ctx context.Context, address string, timeout time.Duration) TCPResult {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", address)
	r := TCPResult{Address: address, RTT: time.Since(start), Err: err}
	if err == nil {
		r.Open = true
		conn.Close()
	}
	return r
}
