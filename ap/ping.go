package ap

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ProbeMode selects how an AP is judged alive before we spend an SSH session on
// it. On a list where most addresses are dead — the normal case for a site
// sweep — this is what keeps the run short.
type ProbeMode string

const (
	// ProbeICMP is the default: one echo request, ~1.5s, no TCP handshake.
	ProbeICMP ProbeMode = "icmp"
	// ProbeTCP connects to the SSH port instead. Useful where ICMP is filtered.
	ProbeTCP ProbeMode = "tcp"
	// ProbeBoth treats an AP as alive if either answers, so an AP behind an ACL
	// that drops ICMP is still reached.
	ProbeBoth ProbeMode = "both"
	// ProbeNone skips the sweep and tries SSH on every address.
	ProbeNone ProbeMode = "none"
)

// PingResult is one echo attempt.
type PingResult struct {
	Alive bool
	RTT   time.Duration
	Err   error
}

// SweepOptions configures the reachability pass.
type SweepOptions struct {
	Mode        ProbeMode
	Timeout     time.Duration // per attempt; 1.5s is plenty on a LAN
	Retries     int           // extra attempts for hosts that stayed silent
	Concurrency int           // in-flight probes
	SSHPort     string        // for ProbeTCP / ProbeBoth
	// OnResult, if set, is called once per host as it settles.
	OnResult func(host string, r PingResult)
}

func (o *SweepOptions) withDefaults() {
	if o.Mode == "" {
		o.Mode = ProbeICMP
	}
	if o.Timeout <= 0 {
		o.Timeout = 1500 * time.Millisecond
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 256
	}
	if o.SSHPort == "" {
		o.SSHPort = "22"
	}
}

// Sweep probes every host in parallel and returns the outcome per host.
//
// A sweep of 1000 addresses at 256 in flight and a 1.5s timeout settles in
// about six seconds even when every one of them is dead, because the cost of a
// dead address is one unanswered packet rather than a TCP connect and an SSH
// handshake against a timeout.
func Sweep(ctx context.Context, hosts []string, opts SweepOptions) map[string]PingResult {
	opts.withDefaults()

	out := make(map[string]PingResult, len(hosts))
	if opts.Mode == ProbeNone {
		for _, h := range hosts {
			out[h] = PingResult{Alive: true}
		}
		return out
	}

	var mu sync.Mutex
	pending := append([]string(nil), hosts...)

	// Silent hosts are retried; hosts that answered are not probed again.
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		if len(pending) == 0 || ctx.Err() != nil {
			break
		}
		var next []string

		sem := make(chan struct{}, opts.Concurrency)
		var wg sync.WaitGroup
		for _, h := range pending {
			wg.Add(1)
			go func(host string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if ctx.Err() != nil {
					return
				}
				r := probeOnce(ctx, host, opts)

				mu.Lock()
				defer mu.Unlock()
				out[host] = r
				if !r.Alive {
					next = append(next, host)
					return
				}
				if opts.OnResult != nil {
					opts.OnResult(host, r)
				}
			}(h)
		}
		wg.Wait()
		pending = next
	}

	// Anything still silent after the last attempt is reported now.
	if opts.OnResult != nil {
		for _, h := range pending {
			opts.OnResult(h, out[h])
		}
	}
	return out
}

func probeOnce(ctx context.Context, host string, opts SweepOptions) PingResult {
	switch opts.Mode {
	case ProbeTCP:
		return tcpProbe(ctx, host, opts.SSHPort, opts.Timeout)
	case ProbeBoth:
		if r := Ping(ctx, host, opts.Timeout); r.Alive {
			return r
		}
		return tcpProbe(ctx, host, opts.SSHPort, opts.Timeout)
	default:
		return Ping(ctx, host, opts.Timeout)
	}
}

// tcpProbe connects to the SSH port. It answers a different question from ICMP
// — "is sshd listening" rather than "is the host up" — which is why it is the
// fallback rather than the default.
func tcpProbe(ctx context.Context, host, port string, timeout time.Duration) PingResult {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	rtt := time.Since(start)
	if err != nil {
		return PingResult{Err: err, RTT: rtt}
	}
	_ = conn.Close()
	return PingResult{Alive: true, RTT: rtt}
}

// icmpUnavailable latches the first "ICMP will never work here" error so the
// caller can warn once and fall back rather than failing every host.
var icmpUnavailable atomic.Pointer[error]

// ICMPUnavailable reports why ICMP could not be used, if it could not.
func ICMPUnavailable() error {
	if p := icmpUnavailable.Load(); p != nil {
		return *p
	}
	return nil
}

func noteICMPUnavailable(err error) {
	icmpUnavailable.CompareAndSwap(nil, &err)
}
