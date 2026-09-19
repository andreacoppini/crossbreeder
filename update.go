package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Update checking.
//
// This is the only connection the tool makes that is not to an AP or to the
// firmware server, so it is kept on a short leash: it never blocks a run, never
// reports a failure, and can be switched off entirely. A tool used on managed
// networks should not surprise anyone with an outbound call, and one whose
// selling point is finishing in seconds must not spend them waiting on GitHub.
// A var rather than a const so the tests can exercise the real wiring —
// startUpdateCheck through to the printed line — against a local server.
var releasesAPI = "https://api.github.com/repos/andreacoppini/crossbreeder/releases/latest"

// updateCheckTTL is how long a result is reused before asking again. Behind a
// corporate NAT every user shares one address against GitHub's unauthenticated
// limit of 60 requests an hour, so asking on every launch would be rude and
// would start failing for everyone at the same site.
const updateCheckTTL = 24 * time.Hour

type updateInfo struct {
	Latest string `json:"latest"`
	URL    string `json:"url"`
}

type cachedCheck struct {
	Checked time.Time  `json:"checked"`
	Info    updateInfo `json:"info"`
}

// checkForUpdate reports a release newer than current, if there is one.
//
// The second return is false for every uninteresting case — checking disabled,
// a development build, no network, a rate limit, a malformed answer, or simply
// being up to date — because none of them is worth a word to the operator.
func checkForUpdate(ctx context.Context, api, current string, allowed bool) (updateInfo, bool) {
	if !allowed || !updateCheckEnabled() {
		return updateInfo{}, false
	}
	// A build that is not a release has nothing to compare against.
	if _, ok := parseVersion(current); !ok {
		return updateInfo{}, false
	}

	info, ok := cachedResult()
	if !ok {
		var err error
		info, err = fetchLatest(ctx, api)
		if err != nil {
			return updateInfo{}, false
		}
		storeResult(info)
	}
	if newer(info.Latest, current) {
		return info, true
	}
	return updateInfo{}, false
}

// updateCheckEnabled honours the environment as well as the flag, so a site can
// switch this off for everyone without editing anyone's command line.
func updateCheckEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CROSSBREEDER_NO_UPDATE_CHECK"))) {
	case "", "0", "false", "no":
		return true
	}
	return false
}

func fetchLatest(ctx context.Context, api string) (updateInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return updateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "crossbreeder-plus/"+version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return updateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return updateInfo{}, fmt.Errorf("github answered %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	// Bounded: a compromised or confused endpoint should not be able to make
	// this allocate without limit.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return updateInfo{}, err
	}
	tag := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if _, ok := parseVersion(tag); !ok {
		return updateInfo{}, fmt.Errorf("unrecognised tag %q", body.TagName)
	}
	return updateInfo{Latest: tag, URL: body.HTMLURL}, nil
}

// newer reports whether latest is a strictly higher version than current.
func newer(latest, current string) bool {
	l, ok := parseVersion(latest)
	if !ok {
		return false
	}
	c, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseVersion reads "1.2.3" into its three numbers. Anything else — "dev", a
// pre-release suffix, an empty string — is not comparable, and the caller
// treats that as "say nothing" rather than guessing.
func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crossbreeder-plus", "update.json"), nil
}

func cachedResult() (updateInfo, bool) {
	p, err := cachePath()
	if err != nil {
		return updateInfo{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return updateInfo{}, false
	}
	var c cachedCheck
	if err := json.Unmarshal(b, &c); err != nil {
		return updateInfo{}, false
	}
	if time.Since(c.Checked) > updateCheckTTL || c.Checked.After(time.Now()) {
		return updateInfo{}, false
	}
	return c.Info, true
}

// storeResult is best-effort: the tool is often run from a read-only share or a
// download folder somebody cannot write to, and that is not worth a word.
func storeResult(info updateInfo) {
	p, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(cachedCheck{Checked: time.Now(), Info: info})
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o644)
}

// updateNotice is the one line the operator sees.
func updateNotice(info updateInfo) string {
	return fmt.Sprintf("A newer version is available: %s (you have %s) — %s", info.Latest, version, info.URL)
}

// startUpdateCheck runs the check alongside whatever the tool was actually
// asked to do. The channel is buffered so the goroutine can finish and be
// collected even if nobody ever reads it — a run cut short by Ctrl-C must not
// leak it.
func startUpdateCheck(opt options) <-chan updateInfo {
	ch := make(chan updateInfo, 1)
	go func() {
		defer close(ch)
		if info, ok := checkForUpdate(context.Background(), releasesAPI, version, !opt.noUpdate); ok {
			ch <- info
		}
	}()
	return ch
}

// reportUpdate prints the notice if one arrived, and otherwise says nothing at
// all. It waits only for a check that has already had the whole run to finish.
func reportUpdate(ch <-chan updateInfo, w io.Writer) {
	select {
	case info, ok := <-ch:
		if ok {
			fmt.Fprintln(w, updateNotice(info))
		}
	case <-time.After(250 * time.Millisecond):
	}
}
