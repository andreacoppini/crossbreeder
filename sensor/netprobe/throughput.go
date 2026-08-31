package netprobe

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Throughput is measured three ways, because sites differ in what they will
// let a sensor do: against a web server nobody has to install anything for,
// against another sensor or the collector, or against an iperf3 server the
// site already runs.
type ThroughputMode string

const (
	ThroughputHTTP   ThroughputMode = "http"   // download from a URL
	ThroughputPeer   ThroughputMode = "peer"   // against a sensor or the collector
	ThroughputIperf3 ThroughputMode = "iperf3" // against an iperf3 server, if installed
)

// ThroughputTest is one measurement.
type ThroughputTest struct {
	Mode     ThroughputMode
	URL      string // http mode
	Peer     string // peer and iperf3 modes: host:port
	Upload   bool
	Duration time.Duration // stop after this long, default 5s
	Bytes    int64         // stop after this many bytes, default 50 MiB
	Streams  int           // parallel connections, default 1
}

// ThroughputResult is the rate achieved and how it was reached.
type ThroughputResult struct {
	Mbps     float64
	Bytes    int64
	Duration time.Duration
	Streams  int
	Mode     ThroughputMode
	Upload   bool
	Err      error
}

// OK reports whether the measurement completed.
func (r ThroughputResult) OK() bool { return r.Err == nil && r.Bytes > 0 }

// RunThroughput measures the rate in the direction asked for.
func RunThroughput(ctx context.Context, t ThroughputTest) ThroughputResult {
	if t.Duration <= 0 {
		t.Duration = 5 * time.Second
	}
	if t.Bytes <= 0 {
		t.Bytes = 50 << 20
	}
	if t.Streams <= 0 {
		t.Streams = 1
	}
	res := ThroughputResult{Mode: t.Mode, Streams: t.Streams, Upload: t.Upload}

	ctx, cancel := context.WithTimeout(ctx, t.Duration+15*time.Second)
	defer cancel()

	var (
		total int64
		mu    sync.Mutex
		wg    sync.WaitGroup
		first error
	)
	start := time.Now()
	deadline := start.Add(t.Duration)
	perStream := t.Bytes / int64(t.Streams)

	for i := 0; i < t.Streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var n int64
			var err error
			switch t.Mode {
			case ThroughputPeer:
				n, err = peerStream(ctx, t.Peer, t.Upload, perStream, deadline)
			case ThroughputIperf3:
				// iperf3 runs its own parallel streams; only one is started.
				return
			default:
				n, err = httpStream(ctx, t, perStream, deadline)
			}
			mu.Lock()
			total += n
			if err != nil && first == nil {
				first = err
			}
			mu.Unlock()
		}()
	}
	if t.Mode == ThroughputIperf3 {
		wg.Wait()
		return runIperf3(ctx, t)
	}
	wg.Wait()

	res.Duration = time.Since(start)
	res.Bytes = total
	if res.Duration > 0 {
		res.Mbps = float64(total) * 8 / res.Duration.Seconds() / 1e6
	}
	if total == 0 && first == nil {
		first = errors.New("no data was transferred")
	}
	res.Err = first
	return res
}

func httpStream(ctx context.Context, t ThroughputTest, limit int64, deadline time.Time) (int64, error) {
	if t.URL == "" {
		return 0, errors.New("no URL to measure against")
	}
	if t.Upload {
		body := io.LimitReader(newFiller(), limit)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, body)
		if err != nil {
			return 0, err
		}
		req.ContentLength = limit
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return limit, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, unwrapURLError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("server answered HTTP %d", resp.StatusCode)
	}
	return copyUntil(resp.Body, limit, deadline)
}

// copyUntil drains r until the byte budget or the wall-clock deadline, and
// reports how much it read. Running out of time is not an error: it is the
// normal way a rate test ends.
func copyUntil(r io.Reader, limit int64, deadline time.Time) (int64, error) {
	buf := make([]byte, 128<<10)
	var total int64
	for total < limit && time.Now().Before(deadline) {
		n, err := r.Read(buf)
		total += int64(n)
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

// The peer protocol is one line, so a stream can be driven by hand with nc
// when a site is arguing about whether the sensor or the network is slow.
const peerBanner = "CBTP1"

func peerStream(ctx context.Context, peer string, upload bool, limit int64, deadline time.Time) (int64, error) {
	if peer == "" {
		return 0, errors.New("no throughput peer configured")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	conn.SetDeadline(deadline.Add(10 * time.Second))

	dir := "DOWN"
	if upload {
		dir = "UP"
	}
	if _, err := fmt.Fprintf(conn, "%s %s %d\n", peerBanner, dir, limit); err != nil {
		return 0, err
	}
	if upload {
		n, err := io.Copy(conn, io.LimitReader(newFiller(), limit))
		if err != nil {
			return n, err
		}
		// The peer acknowledges what it received, so an upload measures what
		// arrived rather than what the local socket buffer swallowed.
		var ack int64
		fmt.Fscanf(conn, "%d", &ack)
		if ack > 0 && ack < n {
			n = ack
		}
		return n, nil
	}
	return copyUntil(conn, limit, deadline)
}

// ServeThroughput answers the peer protocol on ln until it is closed. The
// collector runs one, and so does any sensor asked to be a peer, which is what
// makes a site-to-site measurement possible with nothing else installed.
func ServeThroughput(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go serveThroughputConn(conn)
	}
}

func serveThroughputConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Minute))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[0] != peerBanner {
		fmt.Fprintln(conn, "expected: "+peerBanner+" UP|DOWN <bytes>")
		return
	}
	n, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || n <= 0 || n > 8<<30 {
		return
	}
	switch fields[1] {
	case "DOWN":
		io.Copy(conn, io.LimitReader(newFiller(), n))
	case "UP":
		got, _ := io.Copy(io.Discard, io.LimitReader(conn, n))
		fmt.Fprintf(conn, "%d\n", got)
	}
}

// filler produces bytes as fast as the network will take them. The pattern is
// random once and then repeated: random data defeats any compression on the
// path, and generating it once keeps the sender from measuring its own CPU.
type filler struct{ block []byte }

func newFiller() io.Reader {
	b := make([]byte, 256<<10)
	rand.Read(b)
	return &filler{block: b}
}

func (f *filler) Read(p []byte) (int, error) {
	n := copy(p, f.block)
	return n, nil
}

// iperf3JSON is the part of iperf3's report we use.
type iperf3JSON struct {
	End struct {
		SumSent struct {
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_sent"`
		SumReceived struct {
			BitsPerSecond float64 `json:"bits_per_second"`
			Bytes         int64   `json:"bytes"`
			Seconds       float64 `json:"seconds"`
		} `json:"sum_received"`
	} `json:"end"`
	Error string `json:"error"`
}

func runIperf3(ctx context.Context, t ThroughputTest) ThroughputResult {
	res := ThroughputResult{Mode: ThroughputIperf3, Streams: t.Streams, Upload: t.Upload}
	bin, err := exec.LookPath("iperf3")
	if err != nil {
		res.Err = errors.New("iperf3 is not installed on this sensor")
		return res
	}
	host, port := t.Peer, "5201"
	if h, p, err := net.SplitHostPort(t.Peer); err == nil {
		host, port = h, p
	}
	args := []string{"-c", host, "-p", port, "-J",
		"-t", strconv.Itoa(int(t.Duration.Seconds())), "-P", strconv.Itoa(t.Streams)}
	if !t.Upload {
		args = append(args, "-R") // iperf3 sends by default; -R measures download
	}
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if len(out) == 0 && err != nil {
		res.Err = err
		return res
	}
	var report iperf3JSON
	if jsonErr := json.Unmarshal(out, &report); jsonErr != nil {
		res.Err = fmt.Errorf("could not read iperf3's report: %w", jsonErr)
		return res
	}
	if report.Error != "" {
		res.Err = errors.New(report.Error)
		return res
	}
	res.Mbps = report.End.SumReceived.BitsPerSecond / 1e6
	res.Bytes = report.End.SumReceived.Bytes
	res.Duration = time.Duration(report.End.SumReceived.Seconds * float64(time.Second))
	if res.Mbps == 0 {
		res.Err = errors.New("iperf3 reported no throughput")
	}
	return res
}

// decodeJSON is here so the report parser can be exercised without running
// iperf3, which is not installed everywhere the tests run.
func decodeJSON(s string, v any) error { return json.Unmarshal([]byte(s), v) }
