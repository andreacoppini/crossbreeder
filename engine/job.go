package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

// Event is one thing that happened during a run. The CLI prints them and the
// UI streams them to the browser, so both surfaces see the same run through the
// same events rather than each re-implementing the orchestration.
type Event struct {
	Kind string `json:"kind"`

	Message string `json:"message,omitempty"`
	Phase   string `json:"phase,omitempty"`

	// Counters, for progress. Not omitempty: "0 of 40 downloaded" is a real
	// state, and omitting the zero makes it arrive in the browser as undefined.
	Done  int `json:"done"`
	Total int `json:"total"`

	Result *ap.Result `json:"result,omitempty"`
	// Transcript rides alongside Result because ap.Result deliberately keeps it
	// out of JSON: it is far too big for the exported results file, but it is
	// exactly what the console's transcript pane needs.
	Transcript string      `json:"transcript,omitempty"`
	Dead       []string    `json:"dead,omitempty"`
	Server     *ServerInfo `json:"server,omitempty"`
	Elapsed    string      `json:"elapsed,omitempty"`
}

// ServerInfo describes the built-in image server once it is up.
type ServerInfo struct {
	Addr       string   `json:"addr"`
	Dir        string   `json:"dir"`
	File       string   `json:"file"`
	Reason     string   `json:"reason"`
	Considered []string `json:"considered,omitempty"`
}

// Event kinds.
const (
	EvLog      = "log"      // free text for the log pane
	EvPhase    = "phase"    // a phase started: "sweep", "ssh", "download"
	EvProgress = "progress" // Done/Total moved
	EvSweep    = "sweep"    // the sweep finished; Dead holds the silent addresses
	EvResult   = "result"   // one AP finished
	EvServer   = "server"   // the image server is up
	EvTransfer = "transfer" // one HTTP request served
	EvDone     = "done"     // the run finished
	EvError    = "error"    // the run failed
)

// Emitter receives events. It must be safe to call from several goroutines.
type Emitter func(Event)

// JobResult is everything a finished run produced.
type JobResult struct {
	Results []ap.Result
	Dead    []string
	Elapsed time.Duration
}

// buildConfig turns the flag/form options into an engine config, and returns
// any notes the operator should see about choices made on their behalf.
// minNewPasswordLen is the AP's own rule, taken from the original Crossbreeder:
// "The new password must be 8 characters or longer".
const minNewPasswordLen = 8

// defaultNewPassword is what the original Crossbreeder set on an AP that
// demanded a change (varMigrateNewPassword). Keeping it matters for more than
// familiarity: APs already flashed by that tool are sitting on this password.
const defaultNewPassword = "Crossbreeder"

func buildConfig(opt options, password, newPassword string) (ap.Config, []string, error) {
	var notes []string

	// A password with no username used to be discarded in silence, and the run
	// would then try super/sp-admin and report a login failure that looked like
	// the AP's fault. Refuse instead of guessing.
	if opt.user == "" && password != "" {
		return ap.Config{}, nil, fmt.Errorf("a password was given with no username")
	}

	creds := []ap.Credentials{}
	if opt.user != "" {
		creds = append(creds, ap.Credentials{User: opt.user, Password: password})
	}
	if opt.alsoDefault || len(creds) == 0 {
		creds = append(creds, ap.Credentials{User: "super", Password: "sp-admin"})
	}

	// Changing the password is a separate switch from what to change it to, as
	// it was in the original: turning it off has to leave the password in place
	// rather than making the operator clear the field and lose the value.
	if !opt.changePass {
		newPassword = ""
	}

	// The AP enforces this itself and answers a short one with a re-prompt, so
	// catching it here turns one config mistake into one message instead of a
	// rejection against every factory AP in the list.
	if newPassword != "" && len(newPassword) < minNewPasswordLen {
		return ap.Config{}, nil, fmt.Errorf("the new password must be %d characters or longer; the AP rejects anything shorter", minNewPasswordLen)
	}

	if opt.serveDir != "" && !opt.fw {
		return ap.Config{}, nil, fmt.Errorf("-serve hosts the images for a firmware push; add -fw")
	}

	// "fw update" only starts the download; the AP fetches the image in the
	// background after this session ends. A reboot or a factory reset restarts
	// it before that finishes and throws the image away, so the two cannot be
	// combined — the original allowed it and quietly lost the push. Refusing is
	// better than silently dropping one: on a list of several hundred APs,
	// doing most of what was asked is worse than doing none of it.
	if opt.fw && (opt.factory || opt.reboot) {
		return ap.Config{}, nil, fmt.Errorf(
			"a firmware change cannot be combined with a reboot or factory reset: " +
				"the AP downloads the image after the run, and restarting it discards the download")
	}

	// "set factory" stages the reset; the AP does not act on it until it
	// reboots, so a factory reset on its own leaves the AP exactly as it was.
	reboot := opt.reboot
	if opt.factory && !reboot {
		notes = append(notes, "a factory reset only takes effect on reboot, so the APs will be rebooted")
		reboot = true
	}

	return ap.Config{
		Credentials: creds,
		Actions: ap.Actions{
			UpdateFirmware: opt.fw,
			FactoryReset:   opt.factory,
			CustomCommand:  opt.command,
			Reboot:         reboot,
		},
		Firmware: ap.Firmware{
			Proto: opt.fwProto, Host: opt.fwHost, Port: opt.fwPort,
			User: opt.fwUser, Password: opt.fwPass, Filename: opt.fwFile,
		},
		NewPassword:      newPassword,
		FirmwareWait:     opt.fwWait,
		Port:             opt.sshPort,
		ConnectTimeout:   opt.timeout,
		DialogTimeout:    opt.timeout,
		Deadline:         opt.timeout * 12,
		LegacyAlgorithms: opt.legacy,
	}, notes, nil
}

// runJob is the whole run: image server, ping sweep, SSH phase, then waiting for
// the downloads. Everything worth watching goes out through emit.
// onServer, if set, is handed the image server as soon as it is listening, so a
// caller can report on it while the run is in flight.
func runJob(ctx context.Context, opt options, hosts []string, cfg ap.Config, emit Emitter, onServer func(*fileServer)) (JobResult, error) {
	start := time.Now()

	// Say which accounts are about to be tried. A run that quietly fell back to
	// the factory defaults, or one carrying an empty password, is otherwise
	// indistinguishable from the AP rejecting good credentials.
	emit(Event{Kind: EvLog, Message: credentialSummary(cfg.Credentials)})

	// The image server, if asked for. It starts before anything touches an AP,
	// because "fw update" can have the AP fetching within milliseconds, and it
	// outlives the SSH phase for the same reason.
	var srv *fileServer
	if opt.serveDir != "" {
		var err error
		srv, err = startFileServer(opt.serveDir, opt.serveIP, hosts, opt.servePort)
		if err != nil {
			return JobResult{}, err
		}
		defer srv.Close()
		if onServer != nil {
			onServer(srv)
		}

		cfg.Firmware.Proto = "http"
		cfg.Firmware.Host = srv.Host()
		cfg.Firmware.Port = srv.Port()
		if cfg.Firmware.Filename == "" {
			name, err := pickFirmwareFile(opt.serveDir)
			if err != nil {
				return JobResult{}, err
			}
			cfg.Firmware.Filename = name
		}
		srv.file = cfg.Firmware.Filename
		emit(Event{Kind: EvServer, Server: &ServerInfo{
			Addr: srv.addr, Dir: opt.serveDir, File: cfg.Firmware.Filename,
			Reason: srv.reason, Considered: srv.considered,
		}})
	}

	// Phase 1 — reachability. On a site list where most addresses are dead this
	// is what keeps the run short: a dead address costs one unanswered echo
	// request, not a TCP connect and an SSH handshake against a timeout.
	emit(Event{Kind: EvPhase, Phase: "sweep", Total: len(hosts)})
	alive, dead, pings := sweepHosts(ctx, hosts, opt, emit)
	// Message carries the label the dead rows should show, so the console and
	// the results file describe a silent address the same way.
	emit(Event{Kind: EvSweep, Done: len(alive), Total: len(hosts), Dead: dead,
		Message: noReplyStatus(opt.probe)})

	// Phase 2 — SSH, over the survivors only.
	emit(Event{Kind: EvPhase, Phase: "ssh", Total: len(alive)})
	byHost := make(map[string]ap.Result, len(alive))
	if len(alive) > 0 {
		var mu sync.Mutex
		done := 0
		rn := &Runner{
			Concurrency: opt.concurrency,
			Config:      cfg,
			OnResult: func(_ int, r ap.Result) {
				mu.Lock()
				done++
				n := done
				mu.Unlock()
				res := r
				res.PingMS = msOf(pings[r.IP].RTT)
				emit(Event{Kind: EvResult, Done: n, Total: len(alive), Result: &res, Transcript: res.Transcript})
			},
		}
		for _, r := range rn.Run(ctx, alive) {
			byHost[r.IP] = r
		}
	}

	// Rebuild the full table in input order, so the output still has a row per
	// address the operator supplied - dead ones included, as the GUI grid does.
	results := make([]ap.Result, len(hosts))
	for i, h := range hosts {
		if r, ok := byHost[h]; ok {
			r.PingMS = msOf(pings[h].RTT)
			results[i] = r
			continue
		}
		results[i] = ap.Result{IP: h, Status: noReplyStatus(opt.probe), PingMS: msOf(pings[h].RTT)}
	}

	// Follow the APs we changed until they come back on new firmware. This runs
	// before the download wait only when nothing is being served, since a push
	// has to finish downloading before there is anything to reboot into.
	var pushed []string
	if srv != nil {
		for _, r := range results {
			if r.FwStatus != "" && r.Status == "Done" {
				pushed = append(pushed, r.IP)
			}
		}
	}

	switch {
	case opt.watchEnabled:
		// The image server stays up for as long as the run does, so downloads
		// are reported alongside the re-scan rather than blocking it. Waiting
		// for the downloads first meant an AP that never finished one held the
		// run in the download phase and the re-scan never started.
		if len(pushed) > 0 {
			go streamTransfers(ctx, srv, emit)
		}
		for ip, u := range watchAPs(ctx, opt, cfg, results, emit) {
			for i := range results {
				if results[i].IP == ip {
					results[i].Firmware = u.Firmware
					results[i].Note = u.Note
					if u.MAC != "" {
						results[i].MAC, results[i].Model = u.MAC, u.Model
					}
				}
			}
		}

	case len(pushed) > 0:
		// Not watching, so the only reason to stay alive is the downloads.
		emit(Event{Kind: EvPhase, Phase: "download", Total: len(pushed)})
		waitForDownloads(ctx, srv, pushed, opt.serveWait, emit)
	}

	return JobResult{Results: results, Dead: dead, Elapsed: time.Since(start)}, nil
}

// streamTransfers reports what the image server is serving, without waiting for
// anything. Completion is visible in the server panel; the re-scan is what says
// whether the upgrade actually landed.
func streamTransfers(ctx context.Context, srv *fileServer, emit Emitter) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	shown := 0
	for {
		for _, line := range srv.Transfers()[shown:] {
			emit(Event{Kind: EvTransfer, Message: line})
			shown++
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// waitForDownloads streams the image server's transfer log while the APs fetch.
func waitForDownloads(ctx context.Context, srv *fileServer, pushed []string, timeout time.Duration, emit Emitter) {
	deadline := time.After(timeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	shown := 0
	for {
		for _, line := range srv.Transfers()[shown:] {
			emit(Event{Kind: EvTransfer, Message: line})
			shown++
		}
		done, pending := srv.Completed(pushed)
		emit(Event{Kind: EvProgress, Phase: "download", Done: len(done), Total: len(pushed)})
		if len(pending) == 0 {
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("All %d AP(s) took the image.", len(done))})
			return
		}
		select {
		case <-ctx.Done():
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("Stopped: %d of %d downloaded.", len(done), len(pushed))})
			return
		case <-deadline:
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("Gave up after %s: %d of %d downloaded; still pending: %s",
				timeout, len(done), len(pushed), joinTrim(pending, 10))})
			return
		case <-tick.C:
		}
	}
}

// sweepHosts is the reachability pass, reporting through emit.
func sweepHosts(ctx context.Context, hosts []string, opt options, emit Emitter) ([]string, []string, map[string]ap.PingResult) {
	mode := ap.ProbeMode(opt.probe)
	opts := ap.SweepOptions{
		Mode:        mode,
		Timeout:     opt.pingTimeout,
		Retries:     opt.pingRetries,
		Concurrency: opt.pingConcurrency,
		SSHPort:     opt.sshPort,
	}
	if mode == ap.ProbeNone {
		emit(Event{Kind: EvLog, Message: fmt.Sprintf("Skipping reachability check, trying all %d addresses", len(hosts))})
		return hosts, nil, map[string]ap.PingResult{}
	}

	start := time.Now()
	emit(Event{Kind: EvLog, Message: fmt.Sprintf("Probing %d addresses (%s, %v timeout, %d retries, %d at a time)...",
		len(hosts), mode, opt.pingTimeout, opt.pingRetries, opt.pingConcurrency)})

	var seen int
	var mu sync.Mutex
	opts.OnResult = func(string, ap.PingResult) {
		mu.Lock()
		seen++
		n := seen
		mu.Unlock()
		emit(Event{Kind: EvProgress, Phase: "sweep", Done: n, Total: len(hosts)})
	}
	res := ap.Sweep(ctx, hosts, opts)

	// If ICMP could not be opened at all, every host looks dead, which would
	// silently skip the whole run. Say so and fall back rather than report a
	// site full of dead APs.
	if mode == ap.ProbeICMP {
		if err := ap.ICMPUnavailable(); err != nil {
			emit(Event{Kind: EvLog, Message: fmt.Sprintf("ICMP unavailable (%v); falling back to a TCP probe on port %s", err, opt.sshPort)})
			opts.Mode = ap.ProbeTCP
			res = ap.Sweep(ctx, hosts, opts)
		}
	}

	alive := make([]string, 0, len(hosts))
	dead := make([]string, 0)
	for _, h := range hosts {
		if res[h].Alive {
			alive = append(alive, h)
		} else {
			dead = append(dead, h)
		}
	}
	emit(Event{Kind: EvLog, Message: fmt.Sprintf("%d of %d answered in %s; %d skipped",
		len(alive), len(hosts), time.Since(start).Round(time.Millisecond), len(dead))})
	return alive, dead, res
}

// credentialSummary names the accounts and the length of each password. The
// length is enough to spot an empty or truncated password without ever putting
// the password itself in a log.
func credentialSummary(creds []ap.Credentials) string {
	parts := make([]string, 0, len(creds))
	for _, c := range creds {
		if c.Password == "" {
			parts = append(parts, fmt.Sprintf("%s (no password)", c.User))
		} else {
			parts = append(parts, fmt.Sprintf("%s (%d-character password)", c.User, len(c.Password)))
		}
	}
	return "Trying " + strings.Join(parts, ", then ")
}

func msOf(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

func joinTrim(s []string, n int) string {
	t := trimList(s, n)
	out := ""
	for i, v := range t {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
