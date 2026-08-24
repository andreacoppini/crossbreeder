package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serveTempDir(t *testing.T, files map[string]int) *fileServer {
	t.Helper()
	dir := t.TempDir()
	for name, size := range files {
		b := make([]byte, size)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := startFileServer(dir, "127.0.0.1", []string{"127.0.0.1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestServesFilesAndCountsAFullImage(t *testing.T) {
	f := serveTempDir(t, map[string]int{
		"118.2.0.0.875.rcks": 61,
		"118.2.0.0.875.bl7":  minImageBytes + 4096,
	})

	// The control file first, as the AP does. Too small to count as the image.
	get(t, f, "/118.2.0.0.875.rcks", 61)
	if done, _ := f.Completed([]string{"127.0.0.1"}); len(done) != 0 {
		t.Error("the control file was mistaken for the image")
	}

	get(t, f, "/118.2.0.0.875.bl7", minImageBytes+4096)
	done, pending := f.Completed([]string{"127.0.0.1"})
	if len(done) != 1 || len(pending) != 0 {
		t.Errorf("done = %v, pending = %v", done, pending)
	}

	if lines := strings.Join(f.Transfers(), "\n"); !strings.Contains(lines, "118.2.0.0.875.bl7") {
		t.Errorf("transfer log missing the image:\n%s", lines)
	}
}

// Ruckus retries and may resume, so a transfer split over several requests must
// still add up to a completed image.
func TestRangedDownloadStillCompletes(t *testing.T) {
	const size = minImageBytes + 1000
	f := serveTempDir(t, map[string]int{"img.bl7": size})

	half := size / 2
	getRange(t, f, "/img.bl7", 0, half-1)
	if done, _ := f.Completed([]string{"127.0.0.1"}); len(done) != 0 {
		t.Fatal("half a file counted as complete")
	}
	getRange(t, f, "/img.bl7", half, size-1)
	if done, _ := f.Completed([]string{"127.0.0.1"}); len(done) != 1 {
		t.Error("the two halves did not add up to a complete image")
	}
}

func TestServerIsReadOnlyAndConfined(t *testing.T) {
	f := serveTempDir(t, map[string]int{"img.bl7": 16})

	req, _ := http.NewRequest(http.MethodPost, "http://"+f.addr+"/img.bl7", strings.NewReader("x"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST returned %d, want 405", resp.StatusCode)
	}

	// Nothing outside the served directory may be reachable.
	for _, p := range []string{"/../../etc/passwd", "/..%2f..%2fetc/passwd"} {
		resp, err := http.Get("http://" + f.addr + p)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "root:") {
			t.Errorf("%s escaped the served directory", p)
		}
	}
}

// Wait must return as soon as every AP has the image, rather than sitting out
// its whole timeout.
func TestWaitReturnsOnceEveryAPHasTheImage(t *testing.T) {
	f := serveTempDir(t, map[string]int{"img.bl7": minImageBytes + 1})
	go func() {
		time.Sleep(150 * time.Millisecond)
		get(t, f, "/img.bl7", minImageBytes+1)
	}()

	start := time.Now()
	f.Wait(context.Background(), []string{"127.0.0.1"}, 20*time.Second, os.Stderr)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("waited %v after the download finished", elapsed)
	}
}

// An AP reaching the server from an address other than the one we drove it on
// must still finish the wait, rather than stalling until the timeout.
func TestWaitAcceptsDownloadsFromAnotherAddress(t *testing.T) {
	f := serveTempDir(t, map[string]int{"img.bl7": minImageBytes + 1})
	go func() {
		time.Sleep(100 * time.Millisecond)
		get(t, f, "/img.bl7", minImageBytes+1) // arrives as 127.0.0.1
	}()

	start := time.Now()
	// We asked about an AP at a different address.
	f.Wait(context.Background(), []string{"10.9.9.9"}, 20*time.Second, os.Stderr)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("waited %v; a download from an unexpected address was not counted", elapsed)
	}
	if other := f.CompletedElsewhere([]string{"10.9.9.9"}); len(other) != 1 || other[0] != "127.0.0.1" {
		t.Errorf("CompletedElsewhere = %v", other)
	}
}

func TestWaitStopsOnInterrupt(t *testing.T) {
	f := serveTempDir(t, map[string]int{"img.bl7": minImageBytes + 1})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	start := time.Now()
	f.Wait(ctx, []string{"10.0.0.1"}, time.Hour, os.Stderr) // never downloads
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("ignored the interrupt, waited %v", elapsed)
	}
}

func TestPickFirmwareFile(t *testing.T) {
	dir := t.TempDir()
	write := func(n string) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("118.2.0.0.875.bl7")
	if got, err := pickFirmwareFile(dir); err != nil || got != "118.2.0.0.875.bl7" {
		t.Errorf("lone image: got %q, %v", got, err)
	}
	// A control file present alongside images is the one to push.
	write("118.2.0.0.875.rcks")
	if got, err := pickFirmwareFile(dir); err != nil || got != "118.2.0.0.875.rcks" {
		t.Errorf("control file: got %q, %v", got, err)
	}
	// Ambiguous directories must ask rather than guess.
	write("119.0.0.0.1.rcks")
	if _, err := pickFirmwareFile(dir); err == nil {
		t.Error("two control files should be ambiguous")
	}
}

func get(t *testing.T, f *fileServer, path string, wantBytes int) {
	t.Helper()
	resp, err := http.Get("http://" + f.addr + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, resp.Body)
	if int(n) != wantBytes {
		t.Fatalf("%s: got %d bytes, want %d", path, n, wantBytes)
	}
}

func getRange(t *testing.T, f *fileServer, path string, from, to int) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://"+f.addr+path, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range request returned %d", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)
}
