package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Uplink is the sensor's side of the fleet link. The sensor connects out and
// asks for work; the collector never connects in. That is what lets a sensor
// live on a customer's network behind NAT with nothing forwarded to it, and
// it is the difference between a fleet somebody will actually deploy and one
// that needs a firewall change per site.
type Uplink struct {
	cfg     Upstream
	sensor  SensorConfig
	version string
	store   *Store
	sched   *Scheduler
	client  *http.Client
	log     func(string, ...any)

	// OnConfig is called when the collector pushes a configuration down and
	// the sensor is configured to accept it.
	OnConfig func(Config) error
	// OnUpdate and OnRestart carry out the two commands that end this
	// process, so the decision about how to do that stays in main.
	OnUpdate  func(context.Context) error
	OnRestart func()

	sent time.Time

	finishedMu sync.Mutex
	finished   []string
}

// NewUplink builds the link. It is safe to build one with no URL: Run then
// returns immediately.
func NewUplink(cfg Upstream, sensor SensorConfig, version string, store *Store, sched *Scheduler, log func(string, ...any)) *Uplink {
	if log == nil {
		log = func(string, ...any) {}
	}
	transport := &http.Transport{}
	if cfg.Insecure {
		// For a collector on a private network with its own certificate. It
		// is a flag rather than the default, and it is named honestly.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Uplink{
		cfg: cfg, sensor: sensor, version: version, store: store, sched: sched, log: log,
		client: &http.Client{Timeout: 60 * time.Second, Transport: transport},
	}
}

// Run reports to the collector until ctx is cancelled.
func (u *Uplink) Run(ctx context.Context) {
	if u.cfg.URL == "" {
		return
	}
	every := u.cfg.Every.D()
	if every <= 0 {
		every = time.Minute
	}
	// Everything already in the store goes up on the first report, so a
	// sensor that has been offline for a day does not lose the day.
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if err := u.Report(ctx); err != nil {
			u.log("collector: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Report sends everything since the last successful report and carries out
// whatever comes back.
func (u *Uplink) Report(ctx context.Context) error {
	results, err := u.store.Query(u.sent, time.Time{}, "")
	if err != nil {
		return err
	}
	// The window is inclusive at its start, so the pass that ended the last
	// report would otherwise be sent twice.
	if !u.sent.IsZero() {
		filtered := results[:0]
		for _, r := range results {
			if r.Start.After(u.sent) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	report := Report{
		Sensor: u.sensor.Name, Site: u.sensor.Site, Group: u.sensor.Group,
		Version: u.version, Results: results,
	}
	if u.sched != nil {
		report.Issues = u.sched.Issues().Open()
	}
	report.Finished = u.takeFinished()

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(u.cfg.URL, "/")+"/api/ingest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+u.cfg.Token)

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the collector answered HTTP %d", resp.StatusCode)
	}
	var reply Reply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return err
	}
	if len(results) > 0 {
		u.sent = results[len(results)-1].Start
	}
	u.apply(ctx, reply)
	return nil
}

// apply carries out what the collector asked for. Each command is
// acknowledged on the next report, so one that is interrupted by a restart is
// simply handed back again.
func (u *Uplink) apply(ctx context.Context, reply Reply) {
	if reply.Config != nil {
		switch {
		case !u.cfg.AcceptCfg:
			u.log("the collector pushed a configuration, but this sensor is not set to accept one")
		case u.OnConfig == nil:
			u.log("a configuration arrived with nowhere to put it")
		default:
			if err := u.OnConfig(*reply.Config); err != nil {
				u.log("the configuration from the collector was refused: %v", err)
			} else {
				u.log("took a new configuration from the collector")
			}
		}
	}
	for _, cmd := range reply.Commands {
		u.log("the collector asked for: %s", cmd.Action)
		switch cmd.Action {
		case "run":
			u.sched.Trigger()
			u.finish(cmd.ID)
		case "update":
			if u.OnUpdate != nil {
				if err := u.OnUpdate(ctx); err != nil {
					u.log("update: %v", err)
					continue
				}
			}
			u.finish(cmd.ID)
		case "restart":
			u.finish(cmd.ID)
			if u.OnRestart != nil {
				// Acknowledged first: a restart the collector never hears
				// about would be asked for again on every report.
				u.Report(ctx)
				u.OnRestart()
			}
		default:
			u.log("the collector asked for %q, which this sensor does not know how to do", cmd.Action)
			u.finish(cmd.ID)
		}
	}
}

func (u *Uplink) finish(id string) {
	u.finishedMu.Lock()
	u.finished = append(u.finished, id)
	u.finishedMu.Unlock()
}

func (u *Uplink) takeFinished() []string {
	u.finishedMu.Lock()
	defer u.finishedMu.Unlock()
	out := u.finished
	u.finished = nil
	return out
}
