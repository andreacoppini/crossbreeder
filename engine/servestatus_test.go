package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The console's server panel is only as good as Status(), so exercise it while
// a transfer is genuinely in flight rather than after the fact.
func TestServerStatusReportsLiveAndFinishedTransfers(t *testing.T) {
	dir := t.TempDir()
	image := make([]byte, 3<<20) // big enough to count as an image
	for i := range image {
		image[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(dir, "fw.bl7"), image, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fw.rcks"), []byte("control"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := startFileServer(dir, "127.0.0.1", []string{"127.0.0.1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if st := f.Status(); !st.Running || st.Dir != dir {
		t.Errorf("fresh server status = %+v", st)
	}

	// Read the image slowly so the request is still open when we look.
	resp, err := http.Get(fmt.Sprintf("http://%s/fw.bl7", f.addr))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32<<10)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var live ActiveConn
	for time.Now().Before(deadline) {
		if a := f.Status().Active; len(a) > 0 {
			live = a[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if live.Path != "fw.bl7" {
		t.Fatalf("no in-flight transfer reported; got %+v", live)
	}
	if live.Total != int64(len(image)) {
		t.Errorf("total = %d, want %d", live.Total, len(image))
	}
	if live.Client != "127.0.0.1" {
		t.Errorf("client = %q", live.Client)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Once finished it must leave Active and appear in Recent.
	for time.Now().Before(deadline.Add(2 * time.Second)) {
		st := f.Status()
		if len(st.Active) == 0 && len(st.Recent) > 0 {
			if st.Recent[0].Path != "fw.bl7" || st.Recent[0].Status != 200 {
				t.Errorf("recent = %+v", st.Recent[0])
			}
			if !strings.Contains(st.Recent[0].Human, "MiB") {
				t.Errorf("human size = %q", st.Recent[0].Human)
			}
			if st.Completed != 1 {
				t.Errorf("completed = %d, want 1", st.Completed)
			}
			f.Close()
			if f.Status().Running {
				t.Error("still reports Running after Close")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("transfer never settled: %+v", f.Status())
}

func TestBrowseDirListsFoldersAndFirmware(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "images"), 0o755)
	os.Mkdir(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.rcks"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)

	l, err := browseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Dirs) != 1 || l.Dirs[0].Name != "images" {
		t.Errorf("dirs = %+v (dotfiles should be skipped)", l.Dirs)
	}
	if len(l.Firmware) != 1 || l.Firmware[0] != "a.rcks" {
		t.Errorf("firmware = %v (only .rcks/.bl7 belong here)", l.Firmware)
	}
	if l.Parent == "" {
		t.Error("no parent offered")
	}
}

func TestFirmwareInReportsWhatWouldBeSent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "118.2.0.0.875.bl7"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "118.2.0.0.875.rcks"), []byte("x"), 0o600)

	c := firmwareIn(dir)
	if c.Picked != "118.2.0.0.875.rcks" {
		t.Errorf("picked %q, want the control file", c.Picked)
	}
	if len(c.Candidates) != 2 {
		t.Errorf("candidates = %v", c.Candidates)
	}
	if c.Reason == "" {
		t.Error("no reason given for the automatic choice")
	}

	// Ambiguity must be reported, not guessed at.
	os.WriteFile(filepath.Join(dir, "other.rcks"), []byte("x"), 0o600)
	if c := firmwareIn(dir); c.Picked != "" || c.Err == "" {
		t.Errorf("two control files should be refused: %+v", c)
	}

	if c := firmwareIn(t.TempDir()); c.Err == "" {
		t.Error("an empty folder should say so")
	}
}
