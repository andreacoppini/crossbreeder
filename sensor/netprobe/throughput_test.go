package netprobe

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func startPeer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no TCP loopback: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go ServeThroughput(ctx, ln)
	return ln.Addr().String()
}

func TestThroughputAgainstAPeerBothWays(t *testing.T) {
	peer := startPeer(t)
	for _, upload := range []bool{false, true} {
		r := RunThroughput(context.Background(), ThroughputTest{
			Mode: ThroughputPeer, Peer: peer, Upload: upload,
			Bytes: 4 << 20, Duration: 5 * time.Second,
		})
		if !r.OK() {
			t.Fatalf("upload=%v: %v", upload, r.Err)
		}
		if r.Bytes < 4<<20 {
			t.Errorf("upload=%v: moved %d bytes of 4 MiB", upload, r.Bytes)
		}
		if r.Mbps <= 0 {
			t.Errorf("upload=%v: rate = %.1f Mbps", upload, r.Mbps)
		}
	}
}

func TestThroughputSplitsAcrossStreams(t *testing.T) {
	peer := startPeer(t)
	r := RunThroughput(context.Background(), ThroughputTest{
		Mode: ThroughputPeer, Peer: peer, Bytes: 4 << 20, Streams: 4, Duration: 5 * time.Second,
	})
	if !r.OK() {
		t.Fatalf("four streams failed: %v", r.Err)
	}
	if r.Bytes < 4<<20 {
		t.Errorf("four streams moved %d bytes of the 4 MiB asked for", r.Bytes)
	}
	if r.Streams != 4 {
		t.Errorf("streams = %d", r.Streams)
	}
}

// A rate test has to stop when the clock runs out, not when the byte budget
// is met — otherwise a slow link makes the sensor hang for minutes.
func TestThroughputStopsAtTheDuration(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 200; i++ {
			w.Write(make([]byte, 16<<10))
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer slow.Close()

	start := time.Now()
	r := RunThroughput(context.Background(), ThroughputTest{
		Mode: ThroughputHTTP, URL: slow.URL, Duration: 400 * time.Millisecond, Bytes: 1 << 30,
	})
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Fatalf("a 400ms test took %v", elapsed)
	}
	if !r.OK() {
		t.Fatalf("no data was measured: %v", r.Err)
	}
	if r.Bytes >= 1<<30 {
		t.Errorf("moved %d bytes — the budget, not the clock, ended the test", r.Bytes)
	}
}

func TestThroughputOverHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.CopyN(w, newFiller(), 2<<20)
	}))
	defer srv.Close()

	r := RunThroughput(context.Background(), ThroughputTest{
		Mode: ThroughputHTTP, URL: srv.URL, Bytes: 2 << 20, Duration: 5 * time.Second,
	})
	if !r.OK() || r.Bytes != 2<<20 {
		t.Fatalf("result = %+v", r)
	}
}

func TestThroughputReportsAnUnreachablePeer(t *testing.T) {
	r := RunThroughput(context.Background(), ThroughputTest{
		Mode: ThroughputPeer, Peer: "192.0.2.1:9999", Bytes: 1 << 20, Duration: time.Second,
	})
	if r.OK() {
		t.Fatal("an unreachable peer produced a rate")
	}
}

func TestIperf3ReportParsing(t *testing.T) {
	const report = `{"end":{"sum_sent":{"bits_per_second":123456789.0},
	  "sum_received":{"bits_per_second":94371840.0,"bytes":58982400,"seconds":5.0}}}`
	var r iperf3JSON
	if err := decodeJSON(report, &r); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := r.End.SumReceived.BitsPerSecond / 1e6; got < 94.3 || got > 94.4 {
		t.Errorf("rate = %.2f Mbps, want ~94.37", got)
	}
	if r.End.SumReceived.Bytes != 58982400 {
		t.Errorf("bytes = %d", r.End.SumReceived.Bytes)
	}
}

func TestIperf3MissingIsReportedPlainly(t *testing.T) {
	// Nothing is asserted about whether iperf3 exists on the build machine —
	// only that both outcomes are a result rather than a panic.
	r := RunThroughput(context.Background(), ThroughputTest{
		Mode: ThroughputIperf3, Peer: "192.0.2.1:5201", Duration: time.Second,
	})
	if r.Err == nil && r.Mbps == 0 {
		t.Error("iperf3 reported neither a rate nor a reason")
	}
}
