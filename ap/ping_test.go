package ap

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

// deadAddrs are TEST-NET ranges (RFC 5737). Nothing routable lives there, so a
// probe to one is a genuine silent timeout rather than an instant refusal.
func deadAddrs(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("203.0.113.%d", 1+i%254))
	}
	return out
}

func skipIfNoICMP(t *testing.T) {
	t.Helper()
	if r := Ping(t.Context(), "127.0.0.1", time.Second); !r.Alive {
		t.Skipf("ICMP unavailable here (%v); needs root or net.ipv4.ping_group_range", r.Err)
	}
}

func TestPingLoopbackAnswers(t *testing.T) {
	skipIfNoICMP(t)
	r := Ping(t.Context(), "127.0.0.1", 1500*time.Millisecond)
	if !r.Alive {
		t.Fatalf("loopback did not answer: %v", r.Err)
	}
	if r.RTT <= 0 || r.RTT > time.Second {
		t.Errorf("implausible RTT %v", r.RTT)
	}
}

func TestPingSilentAddressTimesOutOnce(t *testing.T) {
	skipIfNoICMP(t)
	const timeout = 900 * time.Millisecond
	start := time.Now()
	r := Ping(t.Context(), "203.0.113.7", timeout)
	elapsed := time.Since(start)

	if r.Alive {
		t.Fatal("TEST-NET address reported alive")
	}
	// It must wait the full timeout, and must not wait appreciably longer.
	if elapsed < timeout/2 || elapsed > timeout*2 {
		t.Errorf("waited %v for a %v timeout", elapsed, timeout)
	}
}

// TestSweepDeadListIsBoundedByOneTimeout is the point of the whole change: a
// list that is mostly dead must cost about one timeout in total, not one per
// address.
func TestSweepDeadListIsBoundedByOneTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	skipIfNoICMP(t)

	const n = 300
	const timeout = 1500 * time.Millisecond
	hosts := deadAddrs(n)

	start := time.Now()
	res := Sweep(t.Context(), hosts, SweepOptions{
		Mode: ProbeICMP, Timeout: timeout, Retries: 0, Concurrency: 256,
	})
	elapsed := time.Since(start)

	for h, r := range res {
		if r.Alive {
			t.Fatalf("%s reported alive", h)
		}
	}
	// Serially this would be n*timeout = 7m30s.
	serial := time.Duration(n) * timeout
	t.Logf("%d dead addresses swept in %v (serial would be %v, %.0fx)",
		n, elapsed.Round(time.Millisecond), serial, float64(serial)/float64(elapsed))

	if elapsed > 4*timeout {
		t.Errorf("sweep took %v, expected a small multiple of one %v timeout", elapsed, timeout)
	}
}

func TestSweepSeparatesLiveFromDead(t *testing.T) {
	skipIfNoICMP(t)
	hosts := append([]string{"127.0.0.1", "127.0.0.2"}, deadAddrs(8)...)

	res := Sweep(t.Context(), hosts, SweepOptions{
		Mode: ProbeICMP, Timeout: time.Second, Concurrency: 32,
	})
	for _, h := range []string{"127.0.0.1", "127.0.0.2"} {
		if !res[h].Alive {
			t.Errorf("%s should be alive: %v", h, res[h].Err)
		}
	}
	for _, h := range deadAddrs(8) {
		if res[h].Alive {
			t.Errorf("%s should be dead", h)
		}
	}
}

// A silent host must be retried before it is written off, so one dropped packet
// does not skip a live AP.
func TestSweepRetriesSilentHosts(t *testing.T) {
	skipIfNoICMP(t)
	start := time.Now()
	res := Sweep(t.Context(), []string{"203.0.113.9"}, SweepOptions{
		Mode: ProbeICMP, Timeout: 400 * time.Millisecond, Retries: 2, Concurrency: 4,
	})
	elapsed := time.Since(start)

	if res["203.0.113.9"].Alive {
		t.Fatal("reported alive")
	}
	// Three attempts of 400ms.
	if elapsed < 900*time.Millisecond {
		t.Errorf("finished in %v; looks like the retries did not happen", elapsed)
	}
}

func TestSweepTCPModeFindsListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	res := Sweep(t.Context(), []string{"127.0.0.1"}, SweepOptions{
		Mode: ProbeTCP, Timeout: time.Second, SSHPort: port, Concurrency: 4,
	})
	if !res["127.0.0.1"].Alive {
		t.Fatalf("listener not detected: %v", res["127.0.0.1"].Err)
	}

	// Nothing listening on a closed port is a refusal, not a live AP.
	res = Sweep(t.Context(), []string{"127.0.0.1"}, SweepOptions{
		Mode: ProbeTCP, Timeout: time.Second, SSHPort: "1", Concurrency: 4,
	})
	if res["127.0.0.1"].Alive {
		t.Error("closed port reported alive")
	}
}

func TestSweepBothModeAcceptsEither(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	res := Sweep(t.Context(), []string{"127.0.0.1"}, SweepOptions{
		Mode: ProbeBoth, Timeout: time.Second, SSHPort: port, Concurrency: 4,
	})
	if !res["127.0.0.1"].Alive {
		t.Error("both-mode missed a host answering on TCP")
	}
}

func TestSweepNoneModePassesEverythingThrough(t *testing.T) {
	hosts := deadAddrs(5)
	res := Sweep(context.Background(), hosts, SweepOptions{Mode: ProbeNone})
	for _, h := range hosts {
		if !res[h].Alive {
			t.Fatalf("%s should pass through in none mode", h)
		}
	}
}
