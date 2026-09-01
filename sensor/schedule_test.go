package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func schedulerForTest(t *testing.T, networks []Network) (*Scheduler, *Store) {
	t.Helper()
	runner, cfg := testRunner(t, nil, func(string) (net.PacketConn, net.Addr, error) {
		return nil, nil, errors.New("no DHCP on this interface")
	})
	cfg.Networks = networks
	cfg.Sensor.Interval = Duration(50 * time.Millisecond)
	runner.cfg = *cfg
	store, err := NewStore(Storage{})
	if err != nil {
		t.Fatal(err)
	}
	return NewScheduler(*cfg, runner, store, nil, nil), store
}

func TestSchedulerRunOnceCoversEveryEnabledNetwork(t *testing.T) {
	off := false
	s, store := schedulerForTest(t, []Network{
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}}},
		{Name: "Spare", Kind: "wired", Enabled: &off},
	})

	results := s.RunOnce(context.Background())
	if len(results) != 1 || results[0].Network != "Wired" {
		t.Fatalf("results = %+v", results)
	}
	if len(store.Recent(0)) != 1 {
		t.Error("the pass was not recorded")
	}
	if state := s.State(); state.Passes != 1 || state.Running {
		t.Errorf("state = %+v", state)
	}
}

func TestSchedulerTracksIssuesAcrossPasses(t *testing.T) {
	s, _ := schedulerForTest(t, []Network{
		// 192.0.2.1 is TEST-NET-1, which the fake ping never answers for.
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"192.0.2.1"}}},
	})
	s.RunOnce(context.Background())
	if open := s.Issues().Open(); len(open) != 1 {
		t.Fatalf("open issues = %+v", open)
	}
	s.RunOnce(context.Background())
	if open := s.Issues().Open(); len(open) != 1 || open[0].Occurrences != 2 {
		t.Fatalf("a repeated failure did not accumulate: %+v", open)
	}
}

func TestSchedulerPublishesToSubscribers(t *testing.T) {
	s, _ := schedulerForTest(t, []Network{
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}}},
	})
	events, stop := s.Subscribe()
	defer stop()

	go s.RunOnce(context.Background())
	select {
	case r := <-events:
		if r.Network != "Wired" {
			t.Errorf("event = %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event reached the subscriber")
	}
}

// A browser tab that has stopped reading must never hold up the sensor.
func TestSchedulerDoesNotBlockOnASlowSubscriber(t *testing.T) {
	s, _ := schedulerForTest(t, []Network{
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}}},
	})
	_, stop := s.Subscribe() // subscribed and never read
	defer stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			s.RunOnce(context.Background())
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the loop stalled on a subscriber that was not reading")
	}
}

func TestSchedulerRunStopsWhenCancelled(t *testing.T) {
	s, store := schedulerForTest(t, []Network{
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not stop when its context was cancelled")
	}
	if len(store.Recent(0)) < 2 {
		t.Errorf("only %d passes ran in 400ms at a 50ms interval", len(store.Recent(0)))
	}
	if next := s.State().Next; next.IsZero() {
		t.Error("the loop never reported when the next pass was due")
	}
}

func TestSchedulerTriggerRunsEarly(t *testing.T) {
	s, store := schedulerForTest(t, []Network{
		{Name: "Wired", Kind: "wired", Tests: TestPlan{Internet: []string{"1.1.1.1"}}},
	})
	s.cfg.Sensor.Interval = Duration(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	deadline := time.After(10 * time.Second)
	for len(store.Recent(0)) < 1 {
		select {
		case <-deadline:
			t.Fatal("the first pass never ran")
		case <-time.After(10 * time.Millisecond):
		}
	}
	s.Trigger()
	for len(store.Recent(0)) < 2 {
		select {
		case <-deadline:
			t.Fatal("a triggered pass did not run inside the hour-long interval")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
