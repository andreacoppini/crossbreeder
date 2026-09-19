package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func withTempCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // unix
	t.Setenv("LocalAppData", t.TempDir())   // windows
	t.Setenv("HOME", t.TempDir())           // darwin
	t.Setenv("CROSSBREEDER_NO_UPDATE_CHECK", "")
}

func releaseServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.invalid/releases/%s"}`, tag, tag)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestUpdateFoundWhenReleaseIsNewer(t *testing.T) {
	withTempCache(t)
	s := releaseServer(t, "v9.9.9")

	info, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true)
	if !ok {
		t.Fatal("a newer release was not reported")
	}
	if info.Latest != "9.9.9" {
		t.Errorf("Latest = %q, want %q", info.Latest, "9.9.9")
	}
}

func TestNoUpdateWhenCurrent(t *testing.T) {
	withTempCache(t)
	s := releaseServer(t, "v1.0.4")
	if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true); ok {
		t.Error("an equal version was reported as an update")
	}
}

func TestNoUpdateWhenLocalIsAhead(t *testing.T) {
	withTempCache(t)
	s := releaseServer(t, "v1.0.3")
	if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true); ok {
		t.Error("an older release was reported as an update")
	}
}

// Every failure mode has to be silent: the check exists to be helpful, and a
// tool that complains about its own update check on a locked-down network is
// worse than one that says nothing.
func TestEveryFailureIsSilent(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"rate limited", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }},
		{"not found", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }},
		{"garbage", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>captive portal</html>") }},
		{"empty", func(w http.ResponseWriter, r *http.Request) {}},
		{"odd tag", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"tag_name":"nightly"}`) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withTempCache(t)
			s := httptest.NewServer(c.handler)
			defer s.Close()
			if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true); ok {
				t.Errorf("%s produced an update notice", c.name)
			}
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		withTempCache(t)
		if _, ok := checkForUpdate(context.Background(), "http://127.0.0.1:1/nothing", "1.0.4", true); ok {
			t.Error("an unreachable host produced an update notice")
		}
	})
}

// A build that is not a release has nothing to compare against, and nagging a
// developer about their own working copy is noise.
func TestDevBuildNeverChecks(t *testing.T) {
	withTempCache(t)
	asked := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
		fmt.Fprint(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer s.Close()

	if _, ok := checkForUpdate(context.Background(), s.URL, "dev", true); ok {
		t.Error("a dev build was told to update")
	}
	if asked {
		t.Error("a dev build still called GitHub")
	}
}

func TestSwitchesOffCompletely(t *testing.T) {
	withTempCache(t)
	asked := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
		fmt.Fprint(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer s.Close()

	if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", false); ok {
		t.Error("the flag did not switch the check off")
	}
	if asked {
		t.Error("the flag was off but GitHub was still called")
	}

	t.Setenv("CROSSBREEDER_NO_UPDATE_CHECK", "1")
	if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true); ok {
		t.Error("the environment variable did not switch the check off")
	}
	if asked {
		t.Error("the environment variable was set but GitHub was still called")
	}
}

// The second launch inside the TTL must not call GitHub again: behind a
// corporate NAT every user shares one address against GitHub's hourly limit.
func TestResultIsCachedBetweenLaunches(t *testing.T) {
	withTempCache(t)
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"tag_name":"v9.9.9","html_url":"https://example.invalid/x"}`)
	}))
	defer s.Close()

	for i := 0; i < 3; i++ {
		if _, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true); !ok {
			t.Fatalf("call %d reported no update", i+1)
		}
	}
	if calls != 1 {
		t.Errorf("called GitHub %d times, want 1 — the cache is not holding", calls)
	}
}

func TestStaleCacheIsRefreshed(t *testing.T) {
	withTempCache(t)
	storeResult(updateInfo{Latest: "1.0.4"})
	p, err := cachePath()
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * updateCheckTTL)
	b := fmt.Sprintf(`{"checked":%q,"info":{"latest":"1.0.4","url":""}}`, old.Format(time.RFC3339))
	if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}

	s := releaseServer(t, "v9.9.9")
	info, ok := checkForUpdate(context.Background(), s.URL, "1.0.4", true)
	if !ok || info.Latest != "9.9.9" {
		t.Errorf("a stale cache was not refreshed: %+v ok=%v", info, ok)
	}
}

func TestVersionOrdering(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"1.0.5", "1.0.4", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.4", "1.0.4", false},
		{"1.0.3", "1.0.4", false},
		{"1.0.10", "1.0.9", true}, // not string ordering
		{"dev", "1.0.4", false},
		{"1.0", "1.0.4", false},
		{"", "1.0.4", false},
	} {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

// The wiring, not just the check: startUpdateCheck through to the printed line.
func TestNoticeReachesTheOperator(t *testing.T) {
	withTempCache(t)
	s := releaseServer(t, "v9.9.9")
	old := releasesAPI
	releasesAPI = s.URL
	t.Cleanup(func() { releasesAPI = old })

	// The test binary's version is "dev", which correctly disables the check;
	// stand in a release version so the wiring itself is what gets exercised.
	oldVer := version
	version = "1.0.4"
	t.Cleanup(func() { version = oldVer })

	var buf strings.Builder
	reportUpdate(startUpdateCheck(options{}), &buf)
	got := buf.String()
	if !strings.Contains(got, "9.9.9") || !strings.Contains(got, "newer version") {
		t.Errorf("notice = %q, want it to name the new version", got)
	}
}

// And says nothing when there is nothing to say, rather than an empty line.
func TestSilentWhenUpToDate(t *testing.T) {
	withTempCache(t)
	s := releaseServer(t, "v0.0.1")
	old := releasesAPI
	releasesAPI = s.URL
	t.Cleanup(func() { releasesAPI = old })

	oldVer := version
	version = "1.0.4"
	t.Cleanup(func() { version = oldVer })

	var buf strings.Builder
	reportUpdate(startUpdateCheck(options{}), &buf)
	if buf.String() != "" {
		t.Errorf("printed %q when up to date", buf.String())
	}
}

// A run that finishes before the check does must not wait for it.
func TestReportDoesNotBlockOnASlowCheck(t *testing.T) {
	ch := make(chan updateInfo) // never written, never closed
	var buf strings.Builder
	start := time.Now()
	reportUpdate(ch, &buf)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v on a check that never answered", elapsed)
	}
	if buf.String() != "" {
		t.Errorf("printed %q with no result", buf.String())
	}
}
