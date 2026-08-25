package main

import (
	"context"
	"fmt"
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

	tick := time.NewTicker(opt.watchInterval)
	defer tick.Stop()

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
		case <-tick.C:
		}
		pass++

		sweep := ap.Sweep(ctx, order, ap.SweepOptions{
			Mode:        ap.ProbeMode(opt.probe),
			Timeout:     opt.pingTimeout,
			Retries:     0, // a single miss is exactly what we are looking for
			Concurrency: opt.pingConcurrency,
			SSHPort:     opt.sshPort,
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
			rn := &Runner{Concurrency: opt.concurrency, Config: look}
			for _, r := range rn.Run(ctx, up) {
				t := targets[r.IP]
				if t == nil || r.Status != "Done" || r.Firmware == "" {
					continue
				}
				cur := current(updates, results, r.IP)
				cur.MAC, cur.Model, cur.Kind = r.MAC, r.Model, r.Kind
				cur.Firmware = r.Firmware
				// An address that was dead at the start and is answering now
				// joins the table properly rather than staying "No ping reply".
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
				emit(Event{Kind: EvResult, Result: &c})
			}
		}

		upgraded := 0
		for _, t := range targets {
			if t.upgraded {
				upgraded++
			}
		}
		emit(Event{Kind: EvProgress, Phase: "watch", Done: upgraded, Total: len(order)})
		emit(Event{Kind: EvLog, Message: fmt.Sprintf(
			"Re-scan %d: %d up, %d not answering, %d on new firmware.", pass, len(order)-down, down, upgraded)})
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
