package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Severity is how loudly a finding is reported.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

var severityRank = map[Severity]int{SeverityInfo: 0, SeverityWarning: 1, SeverityCritical: 2}

// AtLeast reports whether s is as loud as minimum.
func (s Severity) AtLeast(minimum Severity) bool {
	return severityRank[s] >= severityRank[minimum]
}

// Issue is a finding: a service that is failing or degraded, named in the
// terms whoever has to fix it would use. A measurement is a number; an issue
// is the sentence you put in a ticket.
type Issue struct {
	Key         string    `json:"key"`
	Sensor      string    `json:"sensor,omitempty"`
	Network     string    `json:"network"`
	Service     Service   `json:"service"`
	Severity    Severity  `json:"severity"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail,omitempty"`
	Evidence    []string  `json:"evidence,omitempty"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Occurrences int       `json:"occurrences"`
	Resolved    bool      `json:"resolved,omitempty"`
	ResolvedAt  time.Time `json:"resolved_at,omitzero"`
	// RootCause marks the failure furthest down the stack. Everything above a
	// broken layer fails too, and reporting nine issues when one of them
	// explains the other eight is how a monitoring tool trains people to
	// ignore it.
	RootCause bool `json:"root_cause,omitempty"`
}

// Duration is how long the issue has been open.
func (i Issue) Duration() time.Duration {
	end := i.LastSeen
	if i.Resolved && !i.ResolvedAt.IsZero() {
		end = i.ResolvedAt
	}
	return end.Sub(i.FirstSeen)
}

// String renders an issue as one line.
func (i Issue) String() string {
	prefix := strings.ToUpper(string(i.Severity))
	if i.RootCause {
		prefix += " (root cause)"
	}
	if i.Detail == "" {
		return fmt.Sprintf("%s: %s — %s", prefix, i.Network, i.Title)
	}
	return fmt.Sprintf("%s: %s — %s: %s", prefix, i.Network, i.Title, i.Detail)
}

// DetectIssues reads a pass and reports what is wrong with it, one issue per
// service rather than one per measurement.
func DetectIssues(r SuiteResult) []Issue {
	byService := map[Service][]Measurement{}
	for _, m := range r.Measurements {
		if m.Status == StatusFail || m.Status == StatusWarn {
			byService[m.Service] = append(byService[m.Service], m)
		}
	}
	if r.Aborted != "" && len(byService) == 0 {
		// A pass that could not start at all still has to say so.
		byService[ServiceWireless] = nil
	}

	// An issue is dated to the pass that found it, not to the moment it was
	// folded into the tracker.
	now := time.Now()
	if !r.Start.IsZero() {
		now = r.Start.Add(r.Duration)
	}

	var issues []Issue
	for _, service := range ServiceOrder {
		ms, ok := byService[service]
		if !ok {
			continue
		}
		failing, warning := split(ms)
		if len(failing) == 0 && len(warning) == 0 && r.Aborted == "" {
			continue
		}

		severity := SeverityWarning
		subject := warning
		if len(failing) > 0 || r.Aborted != "" {
			severity = SeverityCritical
			subject = failing
		}
		issue := Issue{
			Key:         issueKey(r.Network, service, severity),
			Sensor:      r.Sensor,
			Network:     r.Network,
			Service:     service,
			Severity:    severity,
			Title:       titleFor(service, severity, subject, r.Aborted),
			Detail:      detailFor(subject, r.Aborted),
			Evidence:    evidence(subject),
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: 1,
		}
		issues = append(issues, issue)
	}

	// The first failing service down the dependency order is the one to fix;
	// the rest are consequences of it.
	for i := range issues {
		if issues[i].Severity == SeverityCritical {
			issues[i].RootCause = true
			for j := i + 1; j < len(issues); j++ {
				if issues[j].Severity == SeverityCritical {
					issues[j].Detail = appendReason(issues[j].Detail,
						"probably a consequence of "+string(issues[i].Service))
				}
			}
			break
		}
	}
	return issues
}

func split(ms []Measurement) (failing, warning []Measurement) {
	for _, m := range ms {
		if m.Status == StatusFail {
			failing = append(failing, m)
		} else {
			warning = append(warning, m)
		}
	}
	return
}

func issueKey(network string, service Service, severity Severity) string {
	return fmt.Sprintf("%s/%s/%s", network, service, severity)
}

// titleFor is the sentence that goes in the ticket. Each service gets its own
// wording because "dns is degraded" tells nobody what to do and "the resolver
// is answering slowly" does.
func titleFor(service Service, severity Severity, ms []Measurement, aborted string) string {
	failing := severity == SeverityCritical
	switch service {
	case ServiceWireless:
		if aborted != "" {
			return "the sensor could not get onto the network"
		}
		if failing {
			return "the radio could not associate"
		}
		return "the wireless link is weak or busy"
	case ServiceAuth:
		if failing {
			return "authentication is failing"
		}
		return "authentication is slow"
	case ServiceDHCP:
		if failing {
			return "DHCP is not handing out addresses"
		}
		return "DHCP is slow to answer"
	case ServiceGateway:
		if failing {
			return "the gateway is not answering"
		}
		return "the gateway is answering slowly"
	case ServiceDNS:
		if failing {
			return "name resolution is failing"
		}
		return "name resolution is slow"
	case ServiceInternet:
		if failing {
			return "the internet is unreachable from this network"
		}
		return "the path to the internet is slow"
	case ServiceApplications:
		if failing {
			return "an application is unreachable"
		}
		return "an application is responding slowly"
	case ServiceVoice:
		if failing {
			return "this network would not carry a call"
		}
		return "call quality is marginal"
	case ServiceThroughput:
		if failing {
			return "throughput is far below what this site expects"
		}
		return "throughput is below what this site expects"
	case ServiceLAN:
		if failing {
			return "the wired port is not where it should be"
		}
		return "the wired port is not as expected"
	}
	if len(ms) > 0 {
		return ms[0].Test + " failed"
	}
	return string(service) + " is degraded"
}

func detailFor(ms []Measurement, aborted string) string {
	if aborted != "" {
		return aborted
	}
	if len(ms) == 0 {
		return ""
	}
	first := ms[0]
	reason := first.Error
	if reason == "" {
		reason = first.Detail
	}
	if reason == "" {
		reason = fmt.Sprintf("%s at %s", first.Test, formatValue(first.Value, first.Unit))
	}
	if len(ms) > 1 {
		return fmt.Sprintf("%s (and %d more)", reason, len(ms)-1)
	}
	return reason
}

func evidence(ms []Measurement) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		line := m.Test
		if m.Unit != "" {
			line += " " + formatValue(m.Value, m.Unit)
		}
		if m.Error != "" {
			line += ": " + m.Error
		} else if m.Detail != "" {
			line += ": " + m.Detail
		}
		out = append(out, line)
	}
	sort.Strings(out)
	return out
}

func appendReason(detail, reason string) string {
	if detail == "" {
		return reason
	}
	return detail + "; " + reason
}

// IssueTracker keeps issues open across passes, so the dashboard shows "DNS
// has been failing for forty minutes" rather than a fresh alert every five.
type IssueTracker struct {
	mu   sync.Mutex
	open map[string]Issue
}

// NewIssueTracker returns an empty tracker.
func NewIssueTracker() *IssueTracker { return &IssueTracker{open: map[string]Issue{}} }

// Update folds a pass into the tracker and reports what changed: issues that
// are new, and issues that have cleared. Only these two lists are worth
// telling anybody about — an issue that is simply still there is not news.
func (t *IssueTracker) Update(r SuiteResult) (opened, closed []Issue) {
	found := DetectIssues(r)
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	seen := map[string]bool{}
	for _, issue := range found {
		seen[issue.Key] = true
		if existing, ok := t.open[issue.Key]; ok {
			existing.LastSeen = issue.LastSeen
			existing.Occurrences++
			existing.Detail = issue.Detail
			existing.Evidence = issue.Evidence
			existing.RootCause = issue.RootCause
			t.open[issue.Key] = existing
			continue
		}
		t.open[issue.Key] = issue
		opened = append(opened, issue)
	}

	// Anything for this network that was not found this time has cleared.
	for key, issue := range t.open {
		if seen[key] || issue.Network != r.Network {
			continue
		}
		issue.Resolved = true
		issue.ResolvedAt = now
		closed = append(closed, issue)
		delete(t.open, key)
	}
	return opened, closed
}

// Open lists the issues currently open, worst first.
func (t *IssueTracker) Open() []Issue {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Issue, 0, len(t.open))
	for _, i := range t.open {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Severity != out[b].Severity {
			return severityRank[out[a].Severity] > severityRank[out[b].Severity]
		}
		if out[a].RootCause != out[b].RootCause {
			return out[a].RootCause
		}
		if out[a].Network != out[b].Network {
			return out[a].Network < out[b].Network
		}
		return serviceRank(out[a].Service) < serviceRank(out[b].Service)
	})
	return out
}
