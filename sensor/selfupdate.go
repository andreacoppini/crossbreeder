package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// A fleet of sensors that has to be updated by hand is a fleet that never gets
// updated. This replaces the running binary with the latest release, checking
// the published checksum first, and leaves the restart to whatever is
// supervising the process — systemd on a Pi.
//
// It is the only connection the sensor makes that is not to the network under
// test or to its own collector, and it is only ever made when asked: on
// `-update`, or when a collector sends the command.

// executable is os.Executable, indirected so a test can point an update at a
// copy rather than at the test binary itself.
var executable = os.Executable

// UpdateAPI is where releases are published. It is a variable so the tests can
// point it at a server of their own.
var UpdateAPI = "https://api.github.com/repos/andreacoppini/crossbreeder/releases/latest"

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// SelfUpdate downloads the current release for this platform and replaces the
// running binary with it.
func SelfUpdate(ctx context.Context, current string, log func(string, ...any)) error {
	if log == nil {
		log = func(string, ...any) {}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	latest, err := fetchRelease(ctx)
	if err != nil {
		return err
	}
	tag := strings.TrimPrefix(latest.TagName, "v")
	if tag == "" {
		return errors.New("the release has no version")
	}
	if tag == strings.TrimPrefix(current, "v") {
		log("already on %s", current)
		return nil
	}

	want := fmt.Sprintf("crossbreeder-sensor-%s-%s", runtime.GOOS, runtime.GOARCH)
	var assetURL, sumsURL string
	var size int64
	for _, a := range latest.Assets {
		switch {
		case a.Name == want:
			assetURL, size = a.URL, a.Size
		case a.Name == "SHA256SUMS.txt":
			sumsURL = a.URL
		}
	}
	if assetURL == "" {
		return fmt.Errorf("release %s has no build for %s/%s", tag, runtime.GOOS, runtime.GOARCH)
	}
	if sumsURL == "" {
		return fmt.Errorf("release %s publishes no checksums, so the download cannot be trusted", tag)
	}

	sums, err := fetchSums(ctx, sumsURL)
	if err != nil {
		return err
	}
	expected, ok := sums[want]
	if !ok {
		return fmt.Errorf("the checksum file does not cover %s", want)
	}

	self, err := executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	log("updating %s to %s (%s)", current, tag, humanSize(size))

	// The new binary is written beside the old one, so the rename that
	// replaces it is atomic and stays on the same filesystem.
	tmp, err := os.CreateTemp(filepath.Dir(self), ".crossbreeder-sensor-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	sum, err := download(ctx, assetURL, tmp)
	tmp.Close()
	if err != nil {
		return err
	}
	if sum != expected {
		return fmt.Errorf("the download does not match its published checksum (%s, expected %s)", sum, expected)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), self); err != nil {
		return fmt.Errorf("replacing %s: %w", self, err)
	}
	log("updated to %s — restart to run it", tag)
	return nil
}

func fetchRelease(ctx context.Context) (release, error) {
	var out release
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UpdateAPI, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("the release API answered HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	return out, err
}

// fetchSums reads a sha256sum-format file into a map of name to digest.
func fetchSums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the checksum file answered HTTP %d", resp.StatusCode)
	}
	return parseSums(io.LimitReader(resp.Body, 1<<20))
}

func parseSums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		// sha256sum writes "<digest>  <name>", with the name possibly marked
		// with a leading * for binary mode.
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out, scanner.Err()
}

// download writes the body to w and returns its SHA-256, so the file is
// hashed as it arrives rather than read a second time.
func download(ctx context.Context, url string, w io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the download answered HTTP %d", resp.StatusCode)
	}
	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, digest), io.LimitReader(resp.Body, 512<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func humanSize(bytes int64) string {
	switch {
	case bytes <= 0:
		return "unknown size"
	case bytes < 1<<20:
		return strconv.FormatInt(bytes/1024, 10) + " KiB"
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
}
