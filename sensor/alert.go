package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Alerter sends findings somewhere a person will see them. Every channel here
// is one a site already has: a webhook into whatever they run, Slack, syslog
// into the SIEM, and email for the sites that have neither.
type Alerter struct {
	cfg    AlertConfig
	sensor string
	client *http.Client
	log    func(string, ...any)

	mu   sync.Mutex
	sent map[string]time.Time // when each issue was last announced
}

// NewAlerter builds a dispatcher. It is safe to build one with alerting
// switched off; Dispatch then does nothing.
func NewAlerter(cfg AlertConfig, sensor string, log func(string, ...any)) *Alerter {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Alerter{
		cfg: cfg, sensor: sensor, log: log,
		client: &http.Client{Timeout: 10 * time.Second},
		sent:   map[string]time.Time{},
	}
}

// Dispatch announces new issues and the clearing of old ones. A repeat of an
// issue that is still open is not sent again until the repeat interval has
// passed: a flapping network that alerts every five minutes trains people to
// filter the alerts to a folder nobody reads.
func (a *Alerter) Dispatch(ctx context.Context, opened, closed []Issue) {
	if !a.cfg.Enabled {
		return
	}
	minimum := Severity(strings.ToLower(a.cfg.MinSeverity))
	if minimum == "" {
		minimum = SeverityWarning
	}
	for _, issue := range opened {
		if !issue.Severity.AtLeast(minimum) || !a.shouldSend(issue.Key) {
			continue
		}
		a.send(ctx, issue, false)
	}
	for _, issue := range closed {
		if !issue.Severity.AtLeast(minimum) {
			continue
		}
		a.forget(issue.Key)
		a.send(ctx, issue, true)
	}
}

func (a *Alerter) shouldSend(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	repeat := a.cfg.Repeat.D()
	if repeat <= 0 {
		repeat = time.Hour
	}
	if last, ok := a.sent[key]; ok && time.Since(last) < repeat {
		return false
	}
	a.sent[key] = time.Now()
	return true
}

func (a *Alerter) forget(key string) {
	a.mu.Lock()
	delete(a.sent, key)
	a.mu.Unlock()
}

// Alert is the payload every webhook receives. It is deliberately flat: the
// people wiring this into their own tooling should not have to walk a tree.
type Alert struct {
	Sensor    string    `json:"sensor"`
	Site      string    `json:"site,omitempty"`
	Network   string    `json:"network"`
	Service   Service   `json:"service"`
	Severity  Severity  `json:"severity"`
	State     string    `json:"state"` // opened or cleared
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Evidence  []string  `json:"evidence,omitempty"`
	RootCause bool      `json:"root_cause,omitempty"`
	Since     time.Time `json:"since"`
	At        time.Time `json:"at"`
}

func (a *Alerter) send(ctx context.Context, issue Issue, cleared bool) {
	state := "opened"
	if cleared {
		state = "cleared"
	}
	payload := Alert{
		Sensor: a.sensor, Network: issue.Network, Service: issue.Service,
		Severity: issue.Severity, State: state, Title: issue.Title, Detail: issue.Detail,
		Evidence: issue.Evidence, RootCause: issue.RootCause,
		Since: issue.FirstSeen, At: time.Now(),
	}
	for _, url := range a.cfg.Webhooks {
		a.postJSON(ctx, url, payload)
	}
	if a.cfg.Slack != "" {
		a.postJSON(ctx, a.cfg.Slack, map[string]string{"text": slackText(payload)})
	}
	if a.cfg.Syslog != "" {
		a.syslog(payload)
	}
	if a.cfg.Email != nil {
		a.email(payload)
	}
}

func slackText(a Alert) string {
	icon := "🔴"
	switch {
	case a.State == "cleared":
		icon = "🟢"
	case a.Severity == SeverityWarning:
		icon = "🟠"
	}
	line := fmt.Sprintf("%s *%s* — %s: %s", icon, a.Sensor, a.Network, a.Title)
	if a.State == "cleared" {
		line += " (cleared)"
	}
	if a.Detail != "" {
		line += "\n" + a.Detail
	}
	if a.RootCause && a.State == "opened" {
		line += "\n_This is the failure furthest down the stack; anything above it is a consequence._"
	}
	return line
}

func (a *Alerter) postJSON(ctx context.Context, url string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		a.log("alert: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		a.log("alert: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		a.log("alert to %s: %v", redactURL(url), err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		a.log("alert to %s: HTTP %d", redactURL(url), resp.StatusCode)
	}
}

// redactURL keeps a webhook's secret path out of the log, since a Slack URL
// is a bearer token in the shape of a URL.
func redactURL(raw string) string {
	if i := strings.Index(raw, "://"); i > 0 {
		rest := raw[i+3:]
		if j := strings.IndexByte(rest, '/'); j > 0 {
			return raw[:i+3] + rest[:j] + "/…"
		}
	}
	return raw
}

// syslog sends one RFC 5424 message over UDP. It is written out here rather
// than taken from log/syslog because that package does not build on Windows
// and the sensor's tests run everywhere.
func (a *Alerter) syslog(alert Alert) {
	conn, err := net.DialTimeout("udp", alert2syslogAddr(a.cfg.Syslog), 3*time.Second)
	if err != nil {
		a.log("syslog: %v", err)
		return
	}
	defer conn.Close()

	// facility 16 (local0), severity 3 (error) or 4 (warning).
	severity := 4
	if alert.Severity == SeverityCritical && alert.State == "opened" {
		severity = 3
	}
	priority := 16*8 + severity
	msg := fmt.Sprintf("<%d>1 %s %s crossbreeder-sensor - %s - %s: %s %s",
		priority, time.Now().Format(time.RFC3339), a.sensor, strings.ToUpper(alert.State),
		alert.Network, alert.Title, alert.Detail)
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte(msg)); err != nil {
		a.log("syslog: %v", err)
	}
}

func alert2syslogAddr(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return addr + ":514"
	}
	return addr
}

func (a *Alerter) email(alert Alert) {
	cfg := a.cfg.Email
	if cfg == nil || cfg.Server == "" || len(cfg.To) == 0 {
		return
	}
	subject := fmt.Sprintf("[%s] %s — %s", strings.ToUpper(string(alert.Severity)), alert.Network, alert.Title)
	if alert.State == "cleared" {
		subject = fmt.Sprintf("[CLEARED] %s — %s", alert.Network, alert.Title)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Sensor:  %s\nNetwork: %s\nService: %s\nSince:   %s\n\n%s\n",
		alert.Sensor, alert.Network, alert.Service, alert.Since.Format(time.RFC1123), alert.Detail)
	for _, line := range alert.Evidence {
		fmt.Fprintf(&body, "  %s\n", line)
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		cfg.From, strings.Join(cfg.To, ", "), subject, body.String())

	var auth smtp.Auth
	if cfg.Username != "" {
		host, _, _ := net.SplitHostPort(cfg.Server)
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}
	if err := smtp.SendMail(cfg.Server, auth, cfg.From, cfg.To, []byte(msg)); err != nil {
		a.log("email: %v", err)
	}
}
