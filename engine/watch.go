package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

// Watch states, shown in the console's detail column.
const (
	NoteRebooting = "Rebooting"
	NoteBackUp    = "Back online"
)

// watchTarget is one AP being re-scanned after the first pass.
type watchTarget struct {
	ip       string
	baseline string // the firmware version it had on the first pass
	wasUp    bool   // it answered the first sweep, so dropping off means rebooting
	wentDown bool   // it stopped answering at some point
	upgraded bool   // it has since come back on a different version
}

// watchAPs keeps re-scanning the APs after the actions have been issued, until
// the run is stopped.
//
// The first pass does whatever was asked - firmware, factory, reboot, a command.
// Every pass after that only looks: it pings, and re-reads the version on
// whatever answers. That is what turns "fw update: In progress" into a table
// that eventually says the new version is running, and it is why an AP that has
// dropped off reads as rebooting rather than failed.
//
// It runs until the context is cancelled, which is the Stop button.
func watchAPs(ctx context.Context, opt options, cfg ap.Config, results []ap.Result, emit Emitter) map[string]ap.Result {
	// Every listed address is re-scanned, not just the ones that answered the
	// first sweep: an AP that was already rebooting when Run was pressed would
	// otherwise never be picked up, and a ping costs almost nothing.
	targets := map[string]*watchTarget{}
	var order []string
	for _, r := range results {
		targets[r.IP] = &watchTarget{ip: r.IP, baseline: r.Firmware, wasUp: r.Reachable}
		order = append(order, r.IP)
	}
	updates := map[string]ap.Result{}
	if len(targets) == 0 {
		return updates
	}

	emit(Event{Kind: EvPhase, Phase: "watch", Total: len(order)})
	emit(Event{Kind: EvLog, Message: fmt.Sprintf(
		"Re-scanning %d AP(s) every %s. Press Stop to finish.", len(order), opt.watchInterval)})

	// Inventory only from here. Re-issuing actions against an AP that is
	// halfway through a reboot is the last thing anyone wants.
	look := cfg
	look.Actions = ap.Actions{}
	// A re-read is login plus two commands. The first pass allows for firmware
	// pushes and reboots; giving a re-scan the same budget lets a handful of
	// unresponsive APs stretch a pass out to minutes.
	if d := 6 * cfg.DialogTimeout; d > 0 && d < look.Deadline {
		look.Deadline = d
	}

	// A timer, not a ticker: the interval is the rest between passes, measured
	// from when one ends. A ticker keeps firing during a long pass and leaves a
	// tick queued, so the next pass would start the instant the last finished -
	// which on a large estate means scanning continuously.
	timer := time.NewTimer(opt.watchInterval)
	defer timer.Stop()

	// An optional cap, for the command line; the console leaves it unset and
	// stops on demand instead.
	var deadline <-chan time.Time
	if opt.watch > 0 {
		deadline = time.After(opt.watch)
	}

	pass := 0
	for {
		select {
		case <-ctx.Done():
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("Stopped after %d re-scan(s).", pass)})
			return updates
		case <-deadline:
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("Stopped watching after %s.", opt.watch)})
			return updates
		case <-timer.C:
		}
		pass++
		passStart := time.Now()
		// Say the pass has started before doing any of it. A pass over several
		// hundred APs takes tens of seconds, and reporting only at the end made
		// a working re-scan look like nothing was happening at all.
		emit(Event{Kind: EvPhase, Phase: "rescan", Done: pass, Total: len(order)})
		emit(Event{Kind: EvLog, Message: fmt.Sprintf("Re-scan %d: pinging %d address(es)...", pass, len(order))})

		var swept int
		var smu sync.Mutex
		sweep := ap.Sweep(ctx, order, ap.SweepOptions{
			Mode:        ap.ProbeMode(opt.probe),
			Timeout:     opt.pingTimeout,
			Retries:     0, // a single miss is exactly what we are looking for
			Concurrency: opt.pingConcurrency,
			SSHPort:     opt.sshPort,
			OnResult: func(string, ap.PingResult) {
				smu.Lock()
				swept++
				n := swept
				smu.Unlock()
				emit(Event{Kind: EvProgress, Phase: "rescan-ping", Done: n, Total: len(order)})
			},
		})
		if ctx.Err() != nil {
			continue
		}

		var up []string
		down := 0
		for _, ip := range order {
			t := targets[ip]
			if sweep[ip].Alive {
				up = append(up, ip)
				continue
			}
			down++
			// Only an AP that was up when the run started is rebooting. One that
			// was never there keeps whatever the first sweep said about it.
			if !t.wasUp {
				continue
			}
			t.wentDown = true
			if u := noteChange(updates, results, ip, NoteRebooting); u != nil {
				emit(Event{Kind: EvResult, Result: u})
			}
		}

		if len(up) > 0 {
			emit(Event{Kind: EvLog, Message: fmt.Sprintf(
				"Re-scan %d: %d up, %d not answering; re-reading versions...", pass, len(up), down)})

			var rmu sync.Mutex
			seen := 0
			rn := &Runner{
				Concurrency: opt.concurrency,
				Config:      look,
				// Report each AP as it comes back rather than waiting for the
				// whole pass: on a few hundred APs that wait is the difference
				// between a live table and a frozen one.
				OnResult: func(_ int, r ap.Result) {
					rmu.Lock()
					seen++
					n := seen
					rmu.Unlock()
					emit(Event{Kind: EvProgress, Phase: "rescan-read", Done: n, Total: len(up)})

					t := targets[r.IP]
					if t == nil || r.Status != "Done" || r.Firmware == "" {
						return
					}
					rmu.Lock()
					defer rmu.Unlock()
					cur := current(updates, results, r.IP)
					cur.MAC, cur.Model, cur.Kind = r.MAC, r.Model, r.Kind
					cur.Firmware = r.Firmware
					// This pass's own timing, so the transcript block for it is
					// stamped with when it happened rather than with the first
					// pass's clock.
					cur.Started, cur.Ended = r.Started, r.Ended
					cur.Duration, cur.DurationMS = r.Duration, r.DurationMS
					// An address that was dead at the start and is answering now
					// joins the table properly rather than staying unreachable.
					cur.Reachable, cur.Status = true, "Done"
					if !t.wasUp && t.baseline == "" {
						t.baseline, t.wasUp = r.Firmware, true
					}
					switch {
					case r.Firmware != t.baseline:
						t.upgraded = true
						cur.Note = fmt.Sprintf("Upgraded from %s", t.baseline)
					case t.upgraded:
						// Already recorded; leave the note alone.
					case t.wentDown:
						cur.Note = NoteBackUp
					default:
						cur.Note = ""
					}
					updates[r.IP] = cur
					c := cur
					emit(Event{Kind: EvResult, Result: &c, Transcript: r.Transcript})
				},
			}
			rn.Run(ctx, up)
		}

		upgraded := 0
		for _, t := range targets {
			if t.upgraded {
				upgraded++
			}
		}
		// Ends the pass: the console uses this to clear the "re-scanning" marks
		// from any row that did not report back.
		emit(Event{Kind: EvProgress, Phase: "watch", Done: upgraded, Total: len(order)})
		took := time.Since(passStart).Round(time.Second)
		emit(Event{Kind: EvLog, Message: fmt.Sprintf(
			"Re-scan %d done in %s: %d up, %d not answering, %d on new firmware. Next in %s.",
			pass, took, len(order)-down, down, upgraded, opt.watchInterval)})
		if took > opt.watchInterval {
			emit(Event{Kind: EvLog, Message: fmt.Sprintf(
				"  (that pass took longer than the %s interval; the interval is counted from the end of a pass, so passes never overlap)",
				opt.watchInterval)})
		}

		// Rest, then go again. Draining first keeps a tick that arrived while
		// the pass was running from collapsing the gap to nothing.
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(opt.watchInterval)
	}
}

func current(updates map[string]ap.Result, results []ap.Result, ip string) ap.Result {
	if cur, ok := updates[ip]; ok {
		return cur
	}
	for _, r := range results {
		if r.IP == ip {
			return r
		}
	}
	return ap.Result{IP: ip}
}

// noteChange records a note, returning the row to emit only when the note
// actually changed — otherwise a long reboot would repeat every pass.
func noteChange(updates map[string]ap.Result, results []ap.Result, ip, note string) *ap.Result {
	cur := current(updates, results, ip)
	if cur.Note == note {
		return nil
	}
	cur.Note = note
	updates[ip] = cur
	out := cur
	return &out
}
