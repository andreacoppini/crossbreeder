package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

func TestParseHostsTextAcceptsWhatOperatorsPaste(t *testing.T) {
	in := "\ufeffIP Address,Note\r\n" + // a CSV pasted whole, BOM and all
		"192.168.77.115,site A\r\n" +
		"\"172.20.44.10\"\r\n" +
		"  10.0.0.1  \r\n" +
		"192.168.77.115,duplicate\r\n" +
		"\r\n" +
		"not-an-address\r\n"

	hosts, skipped := parseHostsText(in)
	want := []string{"192.168.77.115", "172.20.44.10", "10.0.0.1"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Errorf("hosts = %v, want %v", hosts, want)
	}
	if len(skipped) != 2 { // the header row and the junk line
		t.Errorf("skipped = %v, want the header and the junk line", skipped)
	}
}

// The console form must not be able to silently zero out a sane default.
func TestMergeKeepsDefaultsForUnsetFields(t *testing.T) {
	base := options{
		concurrency: 25, pingConcurrency: 256, pingRetries: 1,
		pingTimeout: 1500 * time.Millisecond, timeout: 8 * time.Second,
		serveWait: 30 * time.Minute, sshPort: "22", probe: "icmp",
	}
	got := base.merge(runRequest{User: "admin"})

	if got.concurrency != 25 || got.pingConcurrency != 256 {
		t.Errorf("concurrency defaults lost: %d / %d", got.concurrency, got.pingConcurrency)
	}
	if got.pingTimeout != 1500*time.Millisecond || got.timeout != 8*time.Second {
		t.Errorf("timeout defaults lost: %v / %v", got.pingTimeout, got.timeout)
	}
	if got.sshPort != "22" || got.probe != "icmp" {
		t.Errorf("string defaults lost: %q / %q", got.sshPort, got.probe)
	}
	if got.user != "admin" {
		t.Errorf("user not applied")
	}
}

func TestMergeAppliesFormValues(t *testing.T) {
	base := options{concurrency: 25, probe: "icmp", timeout: 8 * time.Second}
	got := base.merge(runRequest{
		Concurrency: 100, Probe: "tcp", TimeoutS: 20,
		Firmware: true, Factory: true, Serve: true, ServeDir: "/fw",
	})
	if got.concurrency != 100 || got.probe != "tcp" || got.timeout != 20*time.Second {
		t.Errorf("form values not applied: %d %q %v", got.concurrency, got.probe, got.timeout)
	}
	if !got.fw || !got.factory || got.serveDir != "/fw" {
		t.Errorf("actions not applied: %+v", got)
	}
}

// Serving is only meaningful for a firmware push; without one the directory
// must not be set, or the run would open a port for nothing.
func TestMergeIgnoresServeDirWhenNotServing(t *testing.T) {
	got := options{}.merge(runRequest{Serve: false, ServeDir: "/fw"})
	if got.serveDir != "" {
		t.Errorf("serveDir = %q with serving off", got.serveDir)
	}
}

func TestExportProducesTheSameColumnsAsTheCLI(t *testing.T) {
	u := &uiServer{subs: map[chan Event]struct{}{}}
	u.results = []ap.Result{
		{IP: "10.0.0.1", MAC: "AA:BB", Model: "R550", Firmware: "7.2", Reachable: true, PingMS: 1.25, Status: "Done", FwStatus: "In progress"},
		{IP: "10.0.0.2", Status: "No ping reply"},
	}

	w := httptest.NewRecorder()
	u.handleExport(w, httptest.NewRequest("GET", "/api/export?kind=csv", nil))
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines:\n%s", len(lines), w.Body.String())
	}
	if !strings.HasPrefix(lines[0], "IP Address,MAC Address,Model,Fw Version,Ping (ms),Result,Firmware Push,Error") {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "1.2") || !strings.Contains(lines[1], "In progress") {
		t.Errorf("row = %q", lines[1])
	}
	if !strings.Contains(lines[2], "Timeout") {
		t.Errorf("an unreachable row should show Timeout for the ping: %q", lines[2])
	}
}

// A browser that connects mid-run must receive what it missed.
func TestEventStreamReplaysHistory(t *testing.T) {
	u := &uiServer{subs: map[chan Event]struct{}{}}
	u.publish(Event{Kind: EvLog, Message: "first"})
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.1", Status: "Done"}})

	req := httptest.NewRequest("GET", "/api/events", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	u.handleEvents(w, req.WithContext(ctx))

	body := w.Body.String()
	if !strings.Contains(body, `"first"`) || !strings.Contains(body, `"10.0.0.1"`) {
		t.Errorf("history not replayed:\n%s", body)
	}
	// Each frame must be a single well-formed JSON object.
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
			t.Errorf("bad frame %q: %v", line, err)
		}
	}
}

func TestRunRejectsAnEmptyHostList(t *testing.T) {
	u := &uiServer{subs: map[chan Event]struct{}{}}
	w := httptest.NewRecorder()
	u.handleRun(w, httptest.NewRequest("POST", "/api/run", strings.NewReader(`{"hosts":[]}`)))
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no addresses") {
		t.Errorf("body = %q", w.Body.String())
	}
}
