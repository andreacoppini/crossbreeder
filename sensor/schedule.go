package main

import (
	"context"
	"sync"
	"time"
)

// Scheduler is the sensor's main loop: a pass over every enabled network, a
// rest, and again, for years. Everything a pass produces — history, issues,
// alerts, whatever the dashboard is showing — hangs off this one place.
type Scheduler struct {
	cfg     Config
	runner  *Runner
	store   *Store
	issues  *IssueTracker
	alerter *Alerter
	log     func(string, ...any)

	mu      sync.Mutex
	running bool
	current string
	last    time.Time
	next    time.Time
	passes  int

	trigger chan struct{}
	subs    map[chan SuiteResult]struct{}
	subMu   sync.Mutex
}

// NewScheduler assembles the loop.
func NewScheduler(cfg Config, runner *Runner, store *Store, alerter *Alerter, log func(string, ...any)) *Scheduler {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Scheduler{
		cfg: cfg, runner: runner, store: store, alerter: alerter, log: log,
		issues:  NewIssueTracker(),
		trigger: make(chan struct{}, 1),
		subs:    map[chan SuiteResult]struct{}{},
	}
}

// Issues exposes the tracker, which the dashboard and the collector both read.
func (s *Scheduler) Issues() *IssueTracker { return s.issues }

// Run keeps testing until ctx is cancelled. The interval is the rest between
// passes rather than a period, so a pass over a slow network can never be
// overtaken by the next one — the same rule the firmware watcher in
// Crossbreeder Plus uses, and for the same reason.
func (s *Scheduler) Run(ctx context.Context) {
	interval := s.cfg.Sensor.Interval.D()
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	for {
		s.RunOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		s.setNext(time.Now().Add(interval))
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.trigger:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// RunOnce performs one pass over every enabled network and returns what it
// found.
func (s *Scheduler) RunOnce(ctx context.Context) []SuiteResult {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running, s.current = false, ""
		s.last = time.Now()
		s.passes++
		s.mu.Unlock()
	}()

	var out []SuiteResult
	for _, network := range s.cfg.Networks {
		if ctx.Err() != nil {
			return out
		}
		if !network.On() {
			continue
		}
		s.mu.Lock()
		s.current = network.Name
		s.mu.Unlock()

		result := s.runner.Run(ctx, network)
		s.log("%s: %s, score %d in %s", result.Network, result.Status(), result.Overall,
			result.Duration.Round(time.Millisecond))

		if err := s.store.Append(result); err != nil {
			s.log("could not record the pass: %v", err)
		}
		opened, closed := s.issues.Update(result)
		if s.alerter != nil {
			s.alerter.Dispatch(ctx, opened, closed)
		}
		for _, issue := range opened {
			s.log("%s", issue)
		}
		for _, issue := range closed {
			s.log("cleared: %s — %s", issue.Network, issue.Title)
		}
		s.publish(result)
		out = append(out, result)
	}
	return out
}

// Trigger asks for a pass now rather than at the next interval. It never
// blocks and never queues more than one.
func (s *Scheduler) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// State is what the dashboard shows about the loop itself.
type State struct {
	Running bool      `json:"running"`
	Current string    `json:"current,omitempty"`
	Last    time.Time `json:"last,omitzero"`
	Next    time.Time `json:"next,omitzero"`
	Passes  int       `json:"passes"`
}

// State reports where the loop is.
func (s *Scheduler) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return State{Running: s.running, Current: s.current, Last: s.last, Next: s.next, Passes: s.passes}
}

func (s *Scheduler) setNext(t time.Time) {
	s.mu.Lock()
	s.next = t
	s.mu.Unlock()
}

// Subscribe returns a channel of results as they are produced, for the
// dashboard's live view. The channel is buffered and lossy: a browser tab
// that has stopped reading must never hold up the sensor.
func (s *Scheduler) Subscribe() (<-chan SuiteResult, func()) {
	ch := make(chan SuiteResult, 8)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	return ch, func() {
		s.subMu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.subMu.Unlock()
	}
}

func (s *Scheduler) publish(r SuiteResult) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- r:
		default:
		}
	}
}
