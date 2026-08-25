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
	NoteBackUp    = "Back online, firmware unchanged"
)

// watchTarget is one AP being followed after the actions were issued.
type watchTarget struct {
	ip       string
	baseline string // the firmware version it had before we touched it
	wentDown bool   // it stopped answering at some point, so a reboot happened
	settled  bool   // confirmed upgraded; nothing left to watch
}

// watchAPs follows the APs we acted on until they come back with new firmware.
//
// A firmware push only ever *starts* something: the AP downloads in the
// background, reboots, and comes back some minutes later. Without this the run
// ends at "In progress" and the operator is left refreshing by hand. Pinging is
// what makes an AP that has dropped off distinguishable from one that failed,
// and re-reading the version is the only thing that actually proves an upgrade.
func watchAPs(ctx context.Context, opt options, cfg ap.Config, results []ap.Result, emit Emitter) map[string]ap.Result {
	targets := map[string]*watchTarget{}
	var order []string
	for _, r := range results {
		// Only APs we actually reached and acted on are worth following.
		if r.Status != "Done" || !r.Reachable {
			continue
		}
		targets[r.IP] = &watchTarget{ip: r.IP, baseline: r.Firmware}
		order = append(order, r.IP)
	}
	updates := map[string]ap.Result{}
	if len(targets) == 0 {
		return updates
	}

	emit(Event{Kind: EvPhase, Phase: "watch", Total: len(order)})
	emit(Event{Kind: EvLog, Message: fmt.Sprintf(
		"Watching %d AP(s) for up to %s, checking every %s", len(order), opt.watch, opt.watchInterval)})

	// Inventory only: the actions were already issued, and re-issuing them on
	// an AP mid-reboot is the last thing anyone wants.
	look := cfg
	look.Actions = ap.Actions{}

	deadline := time.After(opt.watch)
	tick := time.NewTicker(opt.watchInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			emit(Event{Kind: EvLog, Message: "Stopped watching."})
			return updates
		case <-deadline:
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("Stopped watching after %s.", opt.watch)})
			return updates
		case <-tick.C:
		}

		pending := pendingTargets(targets, order)
		if len(pending) == 0 {
			emit(Event{Kind: EvLog, Message: "Every watched AP came back with new firmware."})
			return updates
		}

		// Ping first: it is cheap, and an AP that does not answer is mid-reboot
		// rather than broken.
		sweep := ap.Sweep(ctx, pending, ap.SweepOptions{
			Mode:        ap.ProbeMode(opt.probe),
			Timeout:     opt.pingTimeout,
			Retries:     0, // a single miss is exactly what we are looking for
			Concurrency: opt.pingConcurrency,
			SSHPort:     opt.sshPort,
		})

		var up []string
		for _, ip := range pending {
			t := targets[ip]
			if sweep[ip].Alive {
				up = append(up, ip)
				continue
			}
			t.wentDown = true
			if u := setNote(updates, results, ip, NoteRebooting); u != nil {
				emit(Event{Kind: EvResult, Result: u})
			}
		}
		if len(up) == 0 {
			continue
		}

		// Re-read the version on whatever is answering.
		rn := &Runner{Concurrency: opt.concurrency, Config: look}
		for _, r := range rn.Run(ctx, up) {
			t := targets[r.IP]
			if t == nil || r.Status != "Done" || r.Firmware == "" {
				continue
			}
			cur := updates[r.IP]
			if cur.IP == "" {
				cur = findResult(results, r.IP)
			}
			cur.MAC, cur.Model, cur.Kind = r.MAC, r.Model, r.Kind
			cur.Firmware = r.Firmware

			switch {
			case r.Firmware != t.baseline:
				t.settled = true
				cur.Note = fmt.Sprintf("Upgraded from %s", t.baseline)
			case t.wentDown:
				cur.Note = NoteBackUp
			default:
				cur.Note = ""
			}
			updates[r.IP] = cur
			c := cur
			emit(Event{Kind: EvResult, Result: &c})
		}

		done := 0
		for _, t := range targets {
			if t.settled {
				done++
			}
		}
		emit(Event{Kind: EvProgress, Phase: "watch", Done: done, Total: len(order)})
	}
}

func pendingTargets(targets map[string]*watchTarget, order []string) []string {
	var out []string
	for _, ip := range order {
		if !targets[ip].settled {
			out = append(out, ip)
		}
	}
	return out
}

func findResult(results []ap.Result, ip string) ap.Result {
	for _, r := range results {
		if r.IP == ip {
			return r
		}
	}
	return ap.Result{IP: ip}
}

// setNote records a note against an AP, returning the row to emit only when the
// note actually changed - otherwise a long reboot would repeat every cycle.
func setNote(updates map[string]ap.Result, results []ap.Result, ip, note string) *ap.Result {
	cur, ok := updates[ip]
	if !ok {
		cur = findResult(results, ip)
	}
	if cur.Note == note {
		return nil
	}
	cur.Note = note
	updates[ip] = cur
	out := cur
	return &out
}
