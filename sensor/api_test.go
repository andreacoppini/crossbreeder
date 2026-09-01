package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func apiForTest(t *testing.T) (*API, *Scheduler, *Store) {
	t.Helper()
	runner, cfg := testRunner(t, nil, func(string) (net.PacketConn, net.Addr, error) {
		return nil, nil, errors.New("no DHCP here")
	})
	cfg.Networks = []Network{{
		Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}},
	}}
	runner.cfg = *cfg
	store, err := NewStore(Storage{})
	if err != nil {
		t.Fatal(err)
	}
	sched := NewScheduler(*cfg, runner, store, nil, nil)
	return NewAPI(sched, store, cfg, "", "test", nil), sched, store
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestAPIServesStateAndResults(t *testing.T) {
	api, sched, _ := apiForTest(t)
	h := api.Handler()
	sched.RunOnce(context.Background())

	var state struct {
		Sensor   string `json:"sensor"`
		Version  string `json:"version"`
		Networks []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"networks"`
	}
	w := get(t, h, "/api/state")
	if w.Code != 200 {
		t.Fatalf("state = %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Sensor != "lobby-1" || len(state.Networks) != 1 || !state.Networks[0].Enabled {
		t.Fatalf("state = %+v", state)
	}

	var latest []SuiteResult
	if err := json.Unmarshal(get(t, h, "/api/latest").Body.Bytes(), &latest); err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 || latest[0].Network != "Wired" {
		t.Fatalf("latest = %+v", latest)
	}

	var results []SuiteResult
	json.Unmarshal(get(t, h, "/api/results?from=-1h").Body.Bytes(), &results)
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
}

func TestAPIExportsCSV(t *testing.T) {
	api, sched, _ := apiForTest(t)
	sched.RunOnce(context.Background())

	w := get(t, api.Handler(), "/api/export?from=-1h")
	if w.Code != 200 {
		t.Fatalf("export = %d", w.Code)
	}
	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 {
		t.Fatalf("export has no rows:\n%s", body)
	}
	if !strings.HasPrefix(lines[0], "time,sensor,site,network") {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(body, "reach 1.1.1.1") {
		t.Error("the measurement is missing from the export")
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, "lobby-1") {
		t.Errorf("disposition = %q", got)
	}
}

func TestCSVQuoting(t *testing.T) {
	if got := csv(`answered 10.0.0.1, 10.0.0.2`); got != `"answered 10.0.0.1, 10.0.0.2"` {
		t.Errorf("a field with a comma came out as %s", got)
	}
	if got := csv(`he said "no"`); got != `"he said ""no"""` {
		t.Errorf("quoting = %s", got)
	}
	if got := csv("plain"); got != "plain" {
		t.Errorf("a plain field was quoted: %s", got)
	}
}

func TestAPIMetricsAreScrapable(t *testing.T) {
	api, sched, _ := apiForTest(t)
	sched.RunOnce(context.Background())

	w := get(t, api.Handler(), "/metrics")
	body := w.Body.String()
	for _, want := range []string{
		`sensor_up{sensor="lobby-1"`,
		`sensor_score{sensor="lobby-1",network="Wired",service="overall"}`,
		`sensor_measurement{sensor="lobby-1",network="Wired"`,
		`sensor_test_status{`,
		"# TYPE sensor_score gauge",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics do not contain %q:\n%s", want, body)
		}
	}
	// Every non-comment line has to end in a number, or a scrape fails.
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Errorf("unscrapable line: %q", line)
		}
	}
}

func TestAPIRunTriggersAPass(t *testing.T) {
	api, sched, store := apiForTest(t)
	h := api.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.cfg.Sensor.Interval = Duration(time.Hour)
	go sched.Run(ctx)

	deadline := time.After(10 * time.Second)
	for len(store.Recent(0)) == 0 {
		select {
		case <-deadline:
			t.Fatal("the first pass never ran")
		case <-time.After(10 * time.Millisecond):
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/run", nil))
	if w.Code != 200 {
		t.Fatalf("run = %d", w.Code)
	}
	for len(store.Recent(0)) < 2 {
		select {
		case <-deadline:
			t.Fatal("the triggered pass never ran")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// The dashboard is only ever shown the redacted configuration, so a save from
// it must not turn a passphrase into asterisks.
func TestAPIConfigRoundTripKeepsSecrets(t *testing.T) {
	api, _, _ := apiForTest(t)
	api.cfg.Networks = []Network{{
		Name: "Corp", Kind: "wifi", Profile: wifiProfile("Corp", "a-passphrase"),
	}}
	var saved Config
	api.onConfig = func(c Config) error { saved = c; return nil }
	h := api.Handler()

	shown := get(t, h, "/api/config").Body.Bytes()
	if strings.Contains(string(shown), "a-passphrase") {
		t.Fatal("the API handed out a passphrase")
	}
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(shown)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("save = %d: %s", w.Code, w.Body)
	}
	if saved.Networks[0].Profile.PSK != "a-passphrase" {
		t.Errorf("the passphrase became %q", saved.Networks[0].Profile.PSK)
	}
}

func TestAPIRefusesABrokenConfig(t *testing.T) {
	api, _, _ := apiForTest(t)
	called := false
	api.onConfig = func(Config) error { called = true; return nil }

	w := httptest.NewRecorder()
	body := `{"networks":[{"name":"Corp","kind":"wifi","profile":{"SSID":"Corp","PSK":"short"}}]}`
	api.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a broken config was accepted: %d", w.Code)
	}
	if called {
		t.Error("the broken config reached the sensor anyway")
	}
}

func TestAPIEventsStreamAPass(t *testing.T) {
	api, sched, _ := apiForTest(t)
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		sched.RunOnce(ctx)
	}()

	buf := make([]byte, 4096)
	deadline := time.Now().Add(10 * time.Second)
	var seen string
	for time.Now().Before(deadline) && !strings.Contains(seen, "event: pass") {
		n, err := resp.Body.Read(buf)
		if err != nil {
			break
		}
		seen += string(buf[:n])
	}
	if !strings.Contains(seen, "event: pass") || !strings.Contains(seen, `"network":"Wired"`) {
		t.Fatalf("the stream did not carry the pass:\n%s", seen)
	}
}

func TestAPIServesTheDashboard(t *testing.T) {
	api, _, _ := apiForTest(t)
	h := api.Handler()
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		w := get(t, h, path)
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Errorf("%s = %d, %d bytes", path, w.Code, w.Body.Len())
		}
	}
}

func TestWindowParsing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/results?from=-2h&to=2026-01-02T03:04:05Z", nil)
	from, to := window(r)
	if time.Since(from) < 110*time.Minute || time.Since(from) > 130*time.Minute {
		t.Errorf("relative from = %v", from)
	}
	if to.Year() != 2026 || to.Month() != time.January {
		t.Errorf("absolute to = %v", to)
	}
	if parsed := parseTime("nonsense"); !parsed.IsZero() {
		t.Errorf("nonsense parsed as %v", parsed)
	}
}
