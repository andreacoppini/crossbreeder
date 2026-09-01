package main

import (
	"strings"
	"testing"
	"time"
)

func passWith(network string, ms ...Measurement) SuiteResult {
	r := SuiteResult{
		Sensor: "lobby-1", Network: network, Kind: "wifi",
		Start: time.Now().Add(-time.Second), Duration: time.Second,
	}
	for _, m := range ms {
		r.Add(m)
	}
	r.Scores, r.Overall = Score(r.Measurements)
	return r
}

// When a layer near the bottom fails, everything above it fails too. Reporting
// nine issues when one explains the other eight is how a monitoring tool
// teaches people to ignore it.
func TestDetectIssuesMarksTheRootCause(t *testing.T) {
	r := passWith("Corp",
		Measurement{Test: "DHCP", Service: ServiceDHCP, Status: StatusFail, Error: "no DHCP offer"},
		Measurement{Test: "DNS example.com", Service: ServiceDNS, Status: StatusFail, Error: "i/o timeout"},
		Measurement{Test: "Microsoft 365", Service: ServiceApplications, Status: StatusFail, Error: "no route to host"},
	)
	issues := DetectIssues(r)
	if len(issues) != 3 {
		t.Fatalf("issues = %d: %+v", len(issues), issues)
	}
	if issues[0].Service != ServiceDHCP || !issues[0].RootCause {
		t.Fatalf("the root cause was not the DHCP failure: %+v", issues[0])
	}
	for _, i := range issues[1:] {
		if i.RootCause {
			t.Errorf("%s was also marked as a root cause", i.Service)
		}
		if !strings.Contains(i.Detail, "consequence of dhcp") {
			t.Errorf("%s does not point at the cause: %q", i.Service, i.Detail)
		}
	}
	if !strings.Contains(issues[0].Title, "DHCP") {
		t.Errorf("title = %q", issues[0].Title)
	}
}

func TestDetectIssuesGroupsBySeverityAndService(t *testing.T) {
	r := passWith("Corp",
		Measurement{Test: "DNS a", Service: ServiceDNS, Status: StatusWarn, Value: 220, Unit: "ms"},
		Measurement{Test: "DNS b", Service: ServiceDNS, Status: StatusWarn, Value: 240, Unit: "ms"},
		Measurement{Test: "gateway", Service: ServiceGateway, Status: StatusOK},
	)
	issues := DetectIssues(r)
	if len(issues) != 1 {
		t.Fatalf("two slow resolvers produced %d issues", len(issues))
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %s", issues[0].Severity)
	}
	if len(issues[0].Evidence) != 2 {
		t.Errorf("evidence = %v", issues[0].Evidence)
	}
	if issues[0].RootCause {
		t.Error("a warning was marked as a root cause")
	}
}

func TestDetectIssuesOnAnAbortedPass(t *testing.T) {
	r := passWith("Corp", Measurement{
		Test: "association", Service: ServiceWireless, Status: StatusFail,
		Error: "the passphrase was rejected",
	})
	r.Aborted = "the passphrase was rejected"
	issues := DetectIssues(r)
	if len(issues) != 1 || issues[0].Severity != SeverityCritical {
		t.Fatalf("issues = %+v", issues)
	}
	if !strings.Contains(issues[0].Detail, "passphrase") {
		t.Errorf("detail = %q", issues[0].Detail)
	}
}

func TestIssueTrackerOpensAndClears(t *testing.T) {
	tracker := NewIssueTracker()

	failing := passWith("Corp", Measurement{
		Test: "DNS", Service: ServiceDNS, Status: StatusFail, Error: "timeout",
	})
	opened, closed := tracker.Update(failing)
	if len(opened) != 1 || len(closed) != 0 {
		t.Fatalf("first pass: opened %d, closed %d", len(opened), len(closed))
	}

	// The same failure again is not news: it must not be reported a second
	// time, but it must stay open.
	opened, closed = tracker.Update(failing)
	if len(opened) != 0 || len(closed) != 0 {
		t.Fatalf("repeat pass: opened %d, closed %d", len(opened), len(closed))
	}
	if open := tracker.Open(); len(open) != 1 || open[0].Occurrences != 2 {
		t.Fatalf("open issues = %+v", open)
	}

	healthy := passWith("Corp", Measurement{Test: "DNS", Service: ServiceDNS, Status: StatusOK})
	opened, closed = tracker.Update(healthy)
	if len(opened) != 0 || len(closed) != 1 {
		t.Fatalf("recovery: opened %d, closed %d", len(opened), len(closed))
	}
	if !closed[0].Resolved || closed[0].ResolvedAt.IsZero() {
		t.Errorf("the cleared issue was not marked resolved: %+v", closed[0])
	}
	if len(tracker.Open()) != 0 {
		t.Error("a cleared issue is still open")
	}
}

// One network recovering must not clear another network's issues.
func TestIssueTrackerKeepsNetworksApart(t *testing.T) {
	tracker := NewIssueTracker()
	tracker.Update(passWith("Corp", Measurement{Test: "DNS", Service: ServiceDNS, Status: StatusFail}))
	tracker.Update(passWith("Guest", Measurement{Test: "DNS", Service: ServiceDNS, Status: StatusFail}))
	if len(tracker.Open()) != 2 {
		t.Fatalf("open = %+v", tracker.Open())
	}
	_, closed := tracker.Update(passWith("Guest", Measurement{Test: "DNS", Service: ServiceDNS, Status: StatusOK}))
	if len(closed) != 1 || closed[0].Network != "Guest" {
		t.Fatalf("closed = %+v", closed)
	}
	if open := tracker.Open(); len(open) != 1 || open[0].Network != "Corp" {
		t.Fatalf("the other network's issue was disturbed: %+v", open)
	}
}

func TestOpenIssuesAreSortedWorstFirst(t *testing.T) {
	tracker := NewIssueTracker()
	tracker.Update(passWith("Corp",
		Measurement{Test: "app", Service: ServiceApplications, Status: StatusWarn, Value: 2000, Unit: "ms"},
		Measurement{Test: "DHCP", Service: ServiceDHCP, Status: StatusFail, Error: "no offer"},
	))
	open := tracker.Open()
	if len(open) != 2 {
		t.Fatalf("open = %d", len(open))
	}
	if open[0].Severity != SeverityCritical || !open[0].RootCause {
		t.Errorf("the critical root cause is not first: %+v", open[0])
	}
}

func TestSeverityOrdering(t *testing.T) {
	if !SeverityCritical.AtLeast(SeverityWarning) {
		t.Error("critical does not clear a warning threshold")
	}
	if SeverityInfo.AtLeast(SeverityWarning) {
		t.Error("info cleared a warning threshold")
	}
}

func TestIssueDurationAndRendering(t *testing.T) {
	i := Issue{
		Network: "Corp", Severity: SeverityCritical, Title: "DHCP is not handing out addresses",
		Detail: "no DHCP offer", RootCause: true,
		FirstSeen: time.Now().Add(-40 * time.Minute), LastSeen: time.Now(),
	}
	if d := i.Duration(); d < 39*time.Minute {
		t.Errorf("duration = %v", d)
	}
	line := i.String()
	if !strings.Contains(line, "root cause") || !strings.Contains(line, "Corp") {
		t.Errorf("rendered as %q", line)
	}
}
