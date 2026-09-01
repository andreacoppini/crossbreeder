package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeReleases stands in for GitHub: a release with one asset for this
// platform and a checksum file, optionally a wrong one.
func fakeReleases(t *testing.T, tag string, payload []byte, corrupt bool) *httptest.Server {
	t.Helper()
	name := fmt.Sprintf("crossbreeder-sensor-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	if corrupt {
		digest = strings.Repeat("0", 64)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n%s  other-file\n", digest, name, strings.Repeat("1", 64))
	})
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"assets": []map[string]any{
				{"name": name, "browser_download_url": srv.URL + "/asset", "size": len(payload)},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums"},
			},
		})
	})
	return srv
}

// runInFakeBinary re-runs fn with os.Executable pointing at a copy in a
// temporary directory, so the update replaces that rather than the test binary.
func withFakeBinary(t *testing.T, fn func(path string)) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "crossbreeder-sensor")
	if err := os.WriteFile(path, []byte("the old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executable
	executable = func() (string, error) { return path, nil }
	t.Cleanup(func() { executable = old })
	fn(path)
}

func TestSelfUpdateReplacesTheBinary(t *testing.T) {
	payload := []byte("a newer binary")
	srv := fakeReleases(t, "v2.0.0", payload, false)
	old := UpdateAPI
	UpdateAPI = srv.URL + "/release"
	defer func() { UpdateAPI = old }()

	withFakeBinary(t, func(path string) {
		if err := SelfUpdate(context.Background(), "1.0.0", nil); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(payload) {
			t.Fatalf("the binary was not replaced: %q", got)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm()&0o111 == 0 {
			t.Error("the new binary is not executable")
		}
		// Nothing must be left behind beside it.
		entries, _ := os.ReadDir(filepath.Dir(path))
		if len(entries) != 1 {
			t.Errorf("the update left %d files behind", len(entries)-1)
		}
	})
}

// A download that does not match its published checksum is not installed. This
// is the whole reason the checksum file is fetched at all.
func TestSelfUpdateRefusesAMismatchedChecksum(t *testing.T) {
	srv := fakeReleases(t, "v2.0.0", []byte("a newer binary"), true)
	old := UpdateAPI
	UpdateAPI = srv.URL + "/release"
	defer func() { UpdateAPI = old }()

	withFakeBinary(t, func(path string) {
		err := SelfUpdate(context.Background(), "1.0.0", nil)
		if err == nil {
			t.Fatal("a binary with the wrong checksum was installed")
		}
		if !strings.Contains(err.Error(), "checksum") {
			t.Errorf("error = %v", err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "the old binary" {
			t.Error("the running binary was replaced anyway")
		}
	})
}

func TestSelfUpdateDoesNothingWhenCurrent(t *testing.T) {
	srv := fakeReleases(t, "v1.0.0", []byte("same version"), false)
	old := UpdateAPI
	UpdateAPI = srv.URL + "/release"
	defer func() { UpdateAPI = old }()

	withFakeBinary(t, func(path string) {
		if err := SelfUpdate(context.Background(), "1.0.0", nil); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "the old binary" {
			t.Error("a sensor already on the current version replaced its binary")
		}
	})
}

func TestParseSums(t *testing.T) {
	sums, err := parseSums(strings.NewReader(
		"abc123  crossbreeder-sensor-linux-arm64\ndef456 *crossbreeder-sensor-linux-amd64\nrubbish\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sums["crossbreeder-sensor-linux-arm64"] != "abc123" {
		t.Errorf("sums = %v", sums)
	}
	// sha256sum marks binary mode with a leading asterisk on the name.
	if sums["crossbreeder-sensor-linux-amd64"] != "def456" {
		t.Errorf("binary-mode name was not handled: %v", sums)
	}
	if len(sums) != 2 {
		t.Errorf("a malformed line was kept: %v", sums)
	}
}
