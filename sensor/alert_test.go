package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func webhookCollector(t *testing.T) (*httptest.Server, func() []Alert) {
	t.Helper()
	var mu sync.Mutex
	var got []Alert
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var a Alert
		json.Unmarshal(body, &a)
		mu.Lock()
		got = append(got, a)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv, func() []Alert {
		mu.Lock()
		defer mu.Unlock()
		return append([]Alert(nil), got...)
	}
}

func testIssue(severity Severity) Issue {
	return Issue{
		Key: "Corp/dhcp/" + string(severity), Network: "Corp", Service: ServiceDHCP,
		Severity: severity, Title: "DHCP is not handing out addresses", Detail: "no DHCP offer",
		Evidence: []string{"DHCP: no DHCP offer"}, RootCause: true,
		FirstSeen: time.Now(), LastSeen: time.Now(),
	}
}

func TestAlerterSendsAndThenStaysQuiet(t *testing.T) {
	srv, collected := webhookCollector(t)
	a := NewAlerter(AlertConfig{
		Enabled: true, Webhooks: []string{srv.URL}, Repeat: Duration(time.Hour),
	}, "lobby-1", nil)

	issue := testIssue(SeverityCritical)
	a.Dispatch(context.Background(), []Issue{issue}, nil)
	if got := collected(); len(got) != 1 {
		t.Fatalf("alerts sent = %d", len(got))
	}
	// The same issue again inside the repeat window is not news.
	a.Dispatch(context.Background(), []Issue{issue}, nil)
	if got := collected(); len(got) != 1 {
		t.Fatalf("a repeat inside the quiet window was sent: %d alerts", len(got))
	}

	a.Dispatch(context.Background(), nil, []Issue{issue})
	got := collected()
	if len(got) != 2 || got[1].State != "cleared" {
		t.Fatalf("the recovery was not announced: %+v", got)
	}
	if got[0].Sensor != "lobby-1" || !got[0].RootCause {
		t.Errorf("payload = %+v", got[0])
	}

	// Once cleared, the same issue reopening is news again straight away.
	a.Dispatch(context.Background(), []Issue{issue}, nil)
	if len(collected()) != 3 {
		t.Error("an issue that reopened after clearing was suppressed")
	}
}

func TestAlerterRespectsMinimumSeverity(t *testing.T) {
	srv, collected := webhookCollector(t)
	a := NewAlerter(AlertConfig{
		Enabled: true, Webhooks: []string{srv.URL}, MinSeverity: "critical",
	}, "lobby-1", nil)

	a.Dispatch(context.Background(), []Issue{testIssue(SeverityWarning)}, nil)
	if len(collected()) != 0 {
		t.Fatal("a warning was sent to a critical-only destination")
	}
	a.Dispatch(context.Background(), []Issue{testIssue(SeverityCritical)}, nil)
	if len(collected()) != 1 {
		t.Fatal("a critical issue was not sent")
	}
}

func TestAlerterOffSendsNothing(t *testing.T) {
	srv, collected := webhookCollector(t)
	a := NewAlerter(AlertConfig{Webhooks: []string{srv.URL}}, "lobby-1", nil)
	a.Dispatch(context.Background(), []Issue{testIssue(SeverityCritical)}, nil)
	if len(collected()) != 0 {
		t.Error("alerting was off and an alert was sent anyway")
	}
}

func TestSlackTextReadsAsASentence(t *testing.T) {
	text := slackText(Alert{
		Sensor: "lobby-1", Network: "Corp", Severity: SeverityCritical, State: "opened",
		Title: "DHCP is not handing out addresses", Detail: "no DHCP offer", RootCause: true,
	})
	for _, want := range []string{"lobby-1", "Corp", "DHCP", "no DHCP offer", "furthest down"} {
		if !strings.Contains(text, want) {
			t.Errorf("Slack message does not mention %q:\n%s", want, text)
		}
	}
	cleared := slackText(Alert{State: "cleared", Network: "Corp", Title: "DHCP is not handing out addresses"})
	if !strings.Contains(cleared, "cleared") {
		t.Errorf("a recovery does not say so:\n%s", cleared)
	}
}

// A Slack webhook URL is a bearer token in the shape of a URL, and a failing
// send must not paste it into the log.
func TestRedactURLKeepsTheSecretOut(t *testing.T) {
	got := redactURL("https://hooks.slack.com/services/T0001/B0002/XXXXsecret")
	if strings.Contains(got, "secret") {
		t.Fatalf("redacted URL = %q", got)
	}
	if !strings.Contains(got, "hooks.slack.com") {
		t.Errorf("the host was lost too: %q", got)
	}
}

func TestSyslogAddressGetsADefaultPort(t *testing.T) {
	if got := alert2syslogAddr("10.0.0.5"); got != "10.0.0.5:514" {
		t.Errorf("addr = %q", got)
	}
	if got := alert2syslogAddr("10.0.0.5:1514"); got != "10.0.0.5:1514" {
		t.Errorf("an explicit port was overridden: %q", got)
	}
}
