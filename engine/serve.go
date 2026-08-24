package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// minImageBytes separates a firmware image from the small control file the AP
// fetches first, so "this AP has taken the image" is decidable without knowing
// Ruckus's naming conventions.
const minImageBytes = 1 << 20

// fileServer is a read-only HTTP server for firmware images. Hosting the images
// from the same binary that drives the APs removes the separate TFTP/FTP/HTTP
// server the tool otherwise depends on, and lets the firmware host and port be
// derived from what was actually bound rather than typed in twice.
type fileServer struct {
	dir  string
	addr string // host:port as the APs will see it
	ln   net.Listener
	srv  *http.Server

	reason     string   // why this address was chosen
	considered []string // the alternatives, for -v

	mu       sync.Mutex
	fetched  map[string]map[string]int64 // client IP -> path -> bytes served
	complete map[string]bool             // client IP -> took a full image
	log      []string
}

// startFileServer binds the server and begins serving dir.
//
// advertiseIP is what the APs are told to fetch from; empty means choose one,
// preferring an address on the same subnet as the APs - see chooseServeIP.
func startFileServer(dir, advertiseIP string, targets []string, port int) (*fileServer, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("-serve %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("-serve %s: not a directory", dir)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("cannot listen on port %d: %w", port, err)
	}

	ip, reason, considered := advertiseIP, "", []string(nil)
	if ip == "" {
		var err error
		ip, reason, considered, err = chooseServeIP(targets)
		if err != nil {
			_ = ln.Close()
			return nil, err
		}
	}

	f := &fileServer{
		dir:      dir,
		addr:     net.JoinHostPort(ip, fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)),
		ln:       ln,
		fetched:  map[string]map[string]int64{},
		complete: map[string]bool{},
	}
	f.reason, f.considered = reason, considered
	f.srv = &http.Server{Handler: f.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = f.srv.Serve(ln) }()
	return f, nil
}

// Host and Port are what to hand to "fw set host" / "fw set port".
func (f *fileServer) Host() string {
	h, _, _ := net.SplitHostPort(f.addr)
	return h
}

func (f *fileServer) Port() string {
	_, p, _ := net.SplitHostPort(f.addr)
	return p
}

func (f *fileServer) Close() error { return f.srv.Close() }

func (f *fileServer) handler() http.Handler {
	fs := http.FileServer(http.Dir(f.dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Nothing here should ever write, and the APs only ever GET.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		client, _, _ := net.SplitHostPort(r.RemoteAddr)
		start := time.Now()
		cw := &countingWriter{ResponseWriter: w, status: http.StatusOK}
		fs.ServeHTTP(cw, r)
		f.record(client, r.URL.Path, cw.n, cw.status, time.Since(start))
	})
}

func (f *fileServer) record(client, path string, n int64, status int, d time.Duration) {
	size := f.sizeOf(path)

	f.mu.Lock()
	defer f.mu.Unlock()

	per := f.fetched[client]
	if per == nil {
		per = map[string]int64{}
		f.fetched[client] = per
	}
	// Ruckus retries and may use ranges, so bytes accumulate across requests.
	per[path] += n
	if size > 0 && per[path] >= size && size >= minImageBytes {
		f.complete[client] = true
	}

	f.log = append(f.log, fmt.Sprintf("  %-15s %-3d %-28s %9s in %s",
		client, status, strings.TrimPrefix(path, "/"), humanBytes(n), d.Round(time.Millisecond)))
}

func (f *fileServer) sizeOf(urlPath string) int64 {
	clean := filepath.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	info, err := os.Stat(filepath.Join(f.dir, clean))
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

// Completed reports how many of hosts have taken a full image.
func (f *fileServer) Completed(hosts []string) (done []string, pending []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range hosts {
		if f.complete[h] {
			done = append(done, h)
		} else {
			pending = append(pending, h)
		}
	}
	return done, pending
}

// CompletedElsewhere lists clients that took a full image but are not in hosts.
// An AP that reaches the server over a different interface, or through NAT,
// arrives with a source address that is not the one we drove it on; without
// this it would look as though it never downloaded anything.
func (f *fileServer) CompletedElsewhere(hosts []string) []string {
	known := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		known[h] = true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for ip, ok := range f.complete {
		if ok && !known[ip] {
			out = append(out, ip)
		}
	}
	sort.Strings(out)
	return out
}

// Transfers returns the request log, newest last.
func (f *fileServer) Transfers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

// Wait keeps serving until every host has taken a full image, the timeout
// expires, or the run is interrupted. The APs download in the background well
// after their SSH session ends, so exiting when the SSH phase finishes would
// pull the server out from under them.
func (f *fileServer) Wait(ctx context.Context, hosts []string, timeout time.Duration, w *os.File) {
	if len(hosts) == 0 {
		return
	}
	deadline := time.After(timeout)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	fmt.Fprintf(w, "\nServing %s on http://%s — waiting for %d AP(s) to download (Ctrl-C to stop)\n",
		f.dir, f.addr, len(hosts))

	shown := 0
	for {
		for _, line := range f.Transfers()[shown:] {
			fmt.Fprintln(w, line)
			shown++
		}
		done, pending := f.Completed(hosts)
		if len(pending) == 0 {
			fmt.Fprintf(w, "All %d AP(s) took the image.\n", len(done))
			return
		}
		if other := f.CompletedElsewhere(hosts); len(done)+len(other) >= len(hosts) {
			fmt.Fprintf(w, "%d image download(s) completed, but %d came from addresses not in the list (%s).\n"+
				"An AP that reaches this server over a different interface looks like this; treat it as done.\n",
				len(done)+len(other), len(other), strings.Join(trimList(other, 6), ", "))
			return
		}

		select {
		case <-ctx.Done():
			fmt.Fprintf(w, "Interrupted: %d of %d downloaded; still pending: %s\n",
				len(done), len(hosts), strings.Join(trimList(pending, 10), ", "))
			return
		case <-deadline:
			sort.Strings(pending)
			fmt.Fprintf(w, "Gave up after %s: %d of %d downloaded; still pending: %s\n",
				timeout, len(done), len(hosts), strings.Join(trimList(pending, 10), ", "))
			return
		case <-tick.C:
		}
	}
}

func trimList(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(append([]string{}, s[:n]...), fmt.Sprintf("and %d more", len(s)-n))
}

// localIPFor reports the local address the OS would use to reach target. The
// UDP "connection" sends no packets; it just asks the routing table.
func localIPFor(target string) string {
	if target == "" {
		return ""
	}
	c, err := net.Dial("udp", net.JoinHostPort(target, "9"))
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return ""
}

// pickFirmwareFile names the control file when the served directory makes the
// choice obvious, so pointing the tool at a folder is enough.
func pickFirmwareFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var rcks, images []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".rcks":
			rcks = append(rcks, e.Name())
		case ".bl7":
			images = append(images, e.Name())
		}
	}
	if len(rcks) == 1 {
		return rcks[0], nil
	}
	if len(rcks) == 0 && len(images) == 1 {
		return images[0], nil
	}
	return "", fmt.Errorf("cannot tell which file to push from %s; pass -fw-file", dir)
}

type countingWriter struct {
	http.ResponseWriter
	n      int64
	status int
}

func (c *countingWriter) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
