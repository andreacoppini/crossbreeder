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
		{IP: "10.0.0.1", MAC: "AA:BB", Model: "R550", Firmware: "7.2", Reachable: true, PingMS: 1.25,
			Status: "Done", FwStatus: "In progress", Note: "Upgraded from 7.1"},
		{IP: "10.0.0.2", Status: "No ping reply"},
	}

	w := httptest.NewRecorder()
	u.handleExport(w, httptest.NewRequest("GET", "/api/export?kind=csv", nil))
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines:\n%s", len(lines), w.Body.String())
	}
	if !strings.HasPrefix(lines[0], "IP Address,MAC Address,Model,Fw Version,Ping (ms),Result,Firmware Push,Watch,Error") {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "1.2") || !strings.Contains(lines[1], "In progress") {
		t.Errorf("row = %q", lines[1])
	}
	// What the watch phase concluded has to survive into the exported file.
	if !strings.Contains(lines[1], "Upgraded from 7.1") {
		t.Errorf("watch note missing from the export: %q", lines[1])
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

// The console form must be able to turn watching on, and leave it off when
// unticked. The console has a Stop button, so watching there is open-ended
// rather than capped.
func TestMergeAppliesWatchSettings(t *testing.T) {
	base := options{watchInterval: 30 * time.Second}

	if got := base.merge(runRequest{}); got.watchEnabled {
		t.Error("watching enabled with the box unticked")
	}

	got := base.merge(runRequest{Watch: true, WatchInterval: 45})
	if !got.watchEnabled {
		t.Error("watching not enabled with the box ticked")
	}
	if got.watchInterval != 45*time.Second {
		t.Errorf("interval = %v, want 45s", got.watchInterval)
	}
	if got.watch != 0 {
		t.Errorf("watch cap = %v; the console stops on demand, not on a timer", got.watch)
	}

	// A blank interval must fall back to the process default, not to zero.
	if got := base.merge(runRequest{Watch: true}); got.watchInterval != 30*time.Second {
		t.Errorf("interval = %v, want the default", got.watchInterval)
	}
}

// A zero counter is a real state — "0 of 40 downloaded" — so it must survive
// JSON. omitempty here made it arrive in the browser as undefined.
func TestProgressCountersSurviveZero(t *testing.T) {
	b, err := json.Marshal(Event{Kind: EvProgress, Phase: "download", Done: 0, Total: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["done"]; !ok {
		t.Errorf("done was dropped from %s", b)
	}
	if got["total"] != float64(1) {
		t.Errorf("total = %v", got["total"])
	}
}

// Watch re-emits a row per pass; the exported table must stay one line per AP.
func TestRepeatedResultsDoNotDuplicateRows(t *testing.T) {
	u := &uiServer{subs: map[chan Event]struct{}{}, resultIdx: map[string]int{}}
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.1", Status: "Done", Firmware: "7.1"}})
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.2", Status: "Done", Firmware: "7.1"}})
	// three re-scans of the first AP, the last one carrying the upgrade
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.1", Status: "Done", Firmware: "7.1"}})
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.1", Status: "Done", Note: NoteRebooting}})
	u.publish(Event{Kind: EvResult, Result: &ap.Result{IP: "10.0.0.1", Status: "Done", Firmware: "7.2", Note: "Upgraded from 7.1"}})

	if len(u.results) != 2 {
		t.Fatalf("%d rows after 5 events, want 2:\n%+v", len(u.results), u.results)
	}
	if u.results[0].Firmware != "7.2" || u.results[0].Note != "Upgraded from 7.1" {
		t.Errorf("the row kept a stale version: %+v", u.results[0])
	}
}

// The console and the command line are two front ends onto the same options, so
// their defaults have to agree. They have drifted before: "Also try default"
// shipped off in the console while the original had it on, and the same for the
// forced-password-change switch. This pins the console side to what the flags
// document, so a change to one surface that forgets the other fails here.
func TestConsoleDefaultsMatchTheDocumentedOnes(t *testing.T) {
	b, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)

	for _, c := range []struct{ id, want, why string }{
		{"alsoDefault", `id="alsoDefault" checked`, "-default is on"},
		{"changePass", `id="changePass" checked`, "-change-pass is on"},
		{"newPass", `id="newPass" value="` + defaultNewPassword + `"`, "-new-pass defaults to " + defaultNewPassword},
		{"firmware", `id="firmware">`, "-fw is off"},
		{"factory", `id="factory">`, "-factory is off"},
		{"reboot", `id="reboot">`, "-reboot is off"},
	} {
		if !strings.Contains(html, c.want) {
			t.Errorf("console default for %q does not match the command line (%s); wanted to find %q in index.html",
				c.id, c.why, c.want)
		}
	}
}
