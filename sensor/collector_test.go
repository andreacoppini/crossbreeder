package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func collectorForTest(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	c := NewCollector(t.TempDir(), map[string]string{
		"a-long-shared-secret": "*",
		"lobby-only-token":     "lobby-1",
	}, "", "test", Storage{Keep: Duration(24 * time.Hour), MaxMiB: 8}, nil)
	srv := httptest.NewServer(c.Handler())
	t.Cleanup(srv.Close)
	return c, srv
}

func postReport(t *testing.T, srv *httptest.Server, token string, report Report) (*http.Response, Reply) {
	t.Helper()
	body, _ := json.Marshal(report)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var reply Reply
	json.NewDecoder(resp.Body).Decode(&reply)
	resp.Body.Close()
	return resp, reply
}

func TestCollectorTakesAReportAndShowsTheFleet(t *testing.T) {
	c, srv := collectorForTest(t)
	pass := passAt("Corp", time.Now(), 84)
	resp, reply := postReport(t, srv, "a-long-shared-secret", Report{
		Sensor: "lobby-1", Site: "Head office", Version: "1.0.0",
		Results: []SuiteResult{pass},
		Issues:  []Issue{{Network: "Corp", Title: "DNS is slow", Severity: SeverityWarning}},
	})
	if resp.StatusCode != 200 || reply.Accepted != 1 {
		t.Fatalf("ingest = %d, accepted %d", resp.StatusCode, reply.Accepted)
	}

	fleet := c.Fleet()
	if len(fleet) != 1 || fleet[0].Name != "lobby-1" || !fleet[0].Online() {
		t.Fatalf("fleet = %+v", fleet)
	}
	if fleet[0].Overall != 84 || len(fleet[0].Issues) != 1 {
		t.Errorf("sensor = %+v", fleet[0])
	}

	// The history is kept per sensor and readable back.
	resp2, err := http.Get(srv.URL + "/api/sensors/lobby-1/results?from=-1h")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var stored []SuiteResult
	json.NewDecoder(resp2.Body).Decode(&stored)
	if len(stored) != 1 || stored[0].Network != "Corp" {
		t.Fatalf("stored = %+v", stored)
	}

	// The fleet page is what somebody opens on a phone in a car park.
	page, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	html, err := io.ReadAll(page.Body)
	if err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != 200 {
		t.Fatalf("fleet page = %d", page.StatusCode)
	}
	for _, want := range []string{"lobby-1", "Head office", "84", "DNS is slow"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("the fleet page does not mention %q", want)
		}
	}
}

// One sensor's token must not let it write another sensor's history.
func TestCollectorRefusesAWrongToken(t *testing.T) {
	_, srv := collectorForTest(t)
	if resp, _ := postReport(t, srv, "not-a-token", Report{Sensor: "lobby-1"}); resp.StatusCode != 401 {
		t.Errorf("an unknown token got %d", resp.StatusCode)
	}
	if resp, _ := postReport(t, srv, "lobby-only-token", Report{Sensor: "roof-2"}); resp.StatusCode != 403 {
		t.Errorf("a sensor reporting under another's name got %d", resp.StatusCode)
	}
	if resp, _ := postReport(t, srv, "lobby-only-token", Report{Sensor: "lobby-1"}); resp.StatusCode != 200 {
		t.Errorf("a sensor's own token was refused: %d", resp.StatusCode)
	}
}

func TestCollectorAdminTokenGatesTheFleetViews(t *testing.T) {
	c := NewCollector(t.TempDir(), map[string]string{"a-long-shared-secret": "*"}, "admin-token", "test", Storage{}, nil)
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/fleet")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("the fleet was readable without the admin token: %d", resp.StatusCode)
	}
	resp, err = http.Get(srv.URL + "/api/fleet?token=admin-token")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("the admin token was refused: %d", resp.StatusCode)
	}
	// Ingest is authenticated by the sensor's own token, not the admin one.
	if resp, _ := postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"}); resp.StatusCode != 200 {
		t.Errorf("ingest = %d", resp.StatusCode)
	}
}

// The whole point of the sensor connecting out is that the collector never has
// to reach it: work is handed back on the next report.
func TestCollectorHandsWorkBackOnTheNextReport(t *testing.T) {
	_, srv := collectorForTest(t)
	postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"})

	resp, err := http.Post(srv.URL+"/api/sensors/lobby-1/command?action=run", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("queueing a command = %d", resp.StatusCode)
	}

	_, reply := postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"})
	if len(reply.Commands) != 1 || reply.Commands[0].Action != "run" {
		t.Fatalf("commands = %+v", reply.Commands)
	}
	id := reply.Commands[0].ID

	// Still outstanding until the sensor says it has done it.
	_, reply = postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"})
	if len(reply.Commands) != 1 {
		t.Fatalf("the command was dropped before being carried out: %+v", reply.Commands)
	}
	_, reply = postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1", Finished: []string{id}})
	if len(reply.Commands) != 0 {
		t.Fatalf("an acknowledged command came back: %+v", reply.Commands)
	}
}

func TestCollectorPushesConfigurationOnce(t *testing.T) {
	_, srv := collectorForTest(t)
	cfg := DefaultConfig()
	cfg.Sensor.Name = "lobby-1"
	cfg.Sensor.Interval = Duration(2 * time.Minute)
	body, _ := json.Marshal(cfg)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/sensors/lobby-1/config", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pushing a config = %d", resp.StatusCode)
	}

	_, reply := postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"})
	if reply.Config == nil || reply.Config.Sensor.Interval.D() != 2*time.Minute {
		t.Fatalf("the config was not handed down: %+v", reply.Config)
	}
	// It is handed down once; the sensor keeps it from there.
	_, reply = postReport(t, srv, "a-long-shared-secret", Report{Sensor: "lobby-1"})
	if reply.Config != nil {
		t.Error("the config was pushed a second time")
	}
}

func TestCollectorRefusesABrokenConfig(t *testing.T) {
	_, srv := collectorForTest(t)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/sensors/lobby-1/config",
		strings.NewReader(`{"networks":[{"name":"a","kind":"wifi","profile":{"SSID":"a","PSK":"short"}}]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a broken config was queued for a sensor: %d", resp.StatusCode)
	}
}

// A sensor's name arrives over the network, so it must not be able to write
// outside the fleet directory.
func TestSafeNameCannotEscapeTheDirectory(t *testing.T) {
	for _, name := range []string{"../../etc/passwd", "..", "", "a/b/c", strings.Repeat("x", 200)} {
		got := safeName(name)
		if strings.ContainsAny(got, `/\`) || got == "" || got == ".." || len(got) > 64 {
			t.Errorf("safeName(%q) = %q", name, got)
		}
	}
	if safeName("lobby-1") != "lobby-1" {
		t.Error("an ordinary name was mangled")
	}
}

// The sensor's uplink and the collector have to agree, so the test drives the
// real client against the real server.
func TestUplinkReportsToACollector(t *testing.T) {
	c, srv := collectorForTest(t)
	store, _ := NewStore(Storage{})
	store.Append(passAt("Corp", time.Now().Add(-time.Minute), 90))
	store.Append(passAt("Corp", time.Now(), 70))

	uplink := NewUplink(
		Upstream{URL: srv.URL, Token: "a-long-shared-secret", AcceptCfg: true},
		SensorConfig{Name: "lobby-1", Site: "Head office"}, "1.0.0", store, nil, nil)

	if err := uplink.Report(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}
	fleet := c.Fleet()
	if len(fleet) != 1 || fleet[0].Overall != 70 {
		t.Fatalf("fleet = %+v", fleet)
	}

	// A second report must not send the same passes again.
	store.Append(passAt("Corp", time.Now().Add(time.Minute), 95))
	before := c.Fleet()[0].LastSeen
	if err := uplink.Report(context.Background()); err != nil {
		t.Fatalf("second report: %v", err)
	}
	if !c.Fleet()[0].LastSeen.After(before) {
		t.Error("the second report did not arrive")
	}

	stored, err := c.storeFor("lobby-1").Query(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 {
		t.Fatalf("the collector holds %d passes, want 3 with none repeated", len(stored))
	}
}

func TestUplinkAppliesWhatComesBack(t *testing.T) {
	_, srv := collectorForTest(t)
	store, _ := NewStore(Storage{})

	var applied *Config
	restarted := false
	uplink := NewUplink(
		Upstream{URL: srv.URL, Token: "a-long-shared-secret", AcceptCfg: true},
		SensorConfig{Name: "lobby-1"}, "1.0.0", store, nil, nil)
	uplink.OnConfig = func(c Config) error { applied = &c; return nil }
	uplink.OnRestart = func() { restarted = true }

	cfg := DefaultConfig()
	cfg.Sensor.Interval = Duration(90 * time.Second)
	body, _ := json.Marshal(cfg)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/sensors/lobby-1/config", bytes.NewReader(body))
	http.DefaultClient.Do(req)
	resp, _ := http.Post(srv.URL+"/api/sensors/lobby-1/command?action=restart", "", nil)
	resp.Body.Close()

	if err := uplink.Report(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}
	if applied == nil || applied.Sensor.Interval.D() != 90*time.Second {
		t.Fatalf("the configuration was not applied: %+v", applied)
	}
	if !restarted {
		t.Error("the restart command was not carried out")
	}
}

// A sensor that has not been told to accept configuration must not take one.
func TestUplinkIgnoresConfigWhenNotAllowed(t *testing.T) {
	_, srv := collectorForTest(t)
	store, _ := NewStore(Storage{})
	applied := false
	uplink := NewUplink(
		Upstream{URL: srv.URL, Token: "a-long-shared-secret"}, // AcceptCfg is off
		SensorConfig{Name: "lobby-1"}, "1.0.0", store, nil, nil)
	uplink.OnConfig = func(Config) error { applied = true; return nil }

	body, _ := json.Marshal(DefaultConfig())
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/sensors/lobby-1/config", bytes.NewReader(body))
	http.DefaultClient.Do(req)

	if err := uplink.Report(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("a pushed configuration was applied by a sensor that does not accept them")
	}
}
