// Command crossbreeder-engine is a proof-of-concept replacement for the SSH
// core of the Xojo Crossbreeder tool. Same job — walk a CSV of standalone
// Ruckus APs and collect inventory, push firmware, reset or reboot them — but
// the APs are worked in parallel instead of one at a time.
//
//	crossbreeder-engine -csv aps.csv -user admin -pass Ruckus123 -c 50 -out results.csv
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

var version = "0.1.0-poc"

type options struct {
	csvPath     string
	out         string
	user        string
	pass        string
	askPass     bool
	passEnv     string
	alsoDefault bool
	concurrency int

	fw      bool
	fwProto string
	fwHost  string
	fwPort  string
	fwUser  string
	fwPass  string
	fwFile  string
	fwWait  time.Duration

	serve     serveFlag
	serveDir  string
	servePort int
	serveIP   string
	serveWait time.Duration
	factory   bool
	reboot    bool
	command   string
	sshPort   string
	timeout   time.Duration
	legacy    bool
	verbose   bool
	showVers  bool

	probe           string
	deadOut         string
	pingTimeout     time.Duration
	pingRetries     int
	pingConcurrency int
}

func main() {
	opt := parseFlags()
	if opt.showVers {
		fmt.Println("crossbreeder-engine", version)
		return
	}
	if err := run(opt); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opt options) error {
	serveDir, rest, err := resolveServeDir(opt.serve, flag.Args(), workingDir())
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q", rest[0])
	}
	opt.serveDir = serveDir

	hosts, err := loadHosts(opt.csvPath)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no IP addresses found in %s", opt.csvPath)
	}

	password, err := resolvePassword(opt)
	if err != nil {
		return err
	}

	creds := []ap.Credentials{}
	if opt.user != "" {
		creds = append(creds, ap.Credentials{User: opt.user, Password: password})
	}
	if opt.alsoDefault || len(creds) == 0 {
		creds = append(creds, ap.Credentials{User: "super", Password: "sp-admin"})
	}

	if opt.serveDir != "" && !opt.fw {
		return fmt.Errorf("-serve hosts the images for a firmware push; add -fw")
	}

	// "set factory" stages the reset; the AP does not act on it until it
	// reboots, so a factory reset on its own leaves the AP exactly as it was.
	reboot := opt.reboot
	if opt.factory && !reboot {
		fmt.Fprintln(os.Stderr, "note: a factory reset only takes effect on reboot, so the APs will be rebooted")
		reboot = true
	}

	cfg := ap.Config{
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
		FirmwareWait:     opt.fwWait,
		Port:             opt.sshPort,
		ConnectTimeout:   opt.timeout,
		DialogTimeout:    opt.timeout,
		Deadline:         opt.timeout * 12,
		LegacyAlgorithms: opt.legacy,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The built-in image server, if asked for. It is started before anything
	// touches an AP, because "fw update" can have the AP fetching within
	// milliseconds, and it outlives the SSH phase for the same reason.
	var srv *fileServer
	if opt.serveDir != "" {
		srv, err = startFileServer(opt.serveDir, opt.serveIP, hosts, opt.servePort)
		if err != nil {
			return err
		}
		defer srv.Close()
		cfg.Firmware.Proto = "http"
		cfg.Firmware.Host = srv.Host()
		cfg.Firmware.Port = srv.Port()
		if cfg.Firmware.Filename == "" {
			name, err := pickFirmwareFile(opt.serveDir)
			if err != nil {
				return err
			}
			cfg.Firmware.Filename = name
			fmt.Fprintf(os.Stderr, "Pushing %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "Serving %s on http://%s\n", opt.serveDir, srv.addr)
		if srv.reason != "" {
			fmt.Fprintf(os.Stderr, "  address chosen: %s\n", srv.reason)
		}
		if opt.verbose {
			for _, c := range srv.considered {
				fmt.Fprintf(os.Stderr, "    %s\n", c)
			}
		}
	}

	overall := time.Now()

	// Phase 1 — reachability. On a site list where most addresses are dead this
	// is what keeps the run short: a dead address costs one unanswered echo
	// request, not a TCP connect and an SSH handshake against a timeout.
	alive, dead, pings := sweep(ctx, hosts, opt)

	// Phase 2 — SSH, over the survivors only.
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
				fmt.Fprintf(os.Stderr, "[%d/%d] %-15s %-12s %-10s %-14s %s\n",
					n, len(alive), r.IP, r.Status, r.Model, r.Firmware, r.Error)
				if opt.verbose && r.Transcript != "" {
					fmt.Fprintf(os.Stderr, "----- %s -----\n%s\n", r.IP, r.Transcript)
				}
			},
		}
		for _, r := range rn.Run(ctx, alive) {
			byHost[r.IP] = r
		}
	}

	// Rebuild the full table in input order, so the output still has a row per
	// address the operator supplied — dead ones included, as the GUI grid does.
	results := make([]ap.Result, len(hosts))
	for i, h := range hosts {
		p := pings[h]
		if r, ok := byHost[h]; ok {
			r.PingMS = float64(p.RTT.Microseconds()) / 1000.0
			results[i] = r
			continue
		}
		results[i] = ap.Result{IP: h, Status: noReplyStatus(opt.probe), PingMS: float64(p.RTT.Microseconds()) / 1000.0}
	}

	fmt.Fprintf(os.Stderr, "\n%d addresses, %d alive, %d contacted in %s\n",
		len(hosts), len(alive), len(byHost), time.Since(overall).Round(time.Millisecond))

	// Hold the server up until the APs that accepted the push have taken the
	// image; they download long after their SSH session closed.
	if srv != nil {
		var pushed []string
		for _, r := range results {
			if r.FwStatus != "" && r.Status == "Done" {
				pushed = append(pushed, r.IP)
			}
		}
		srv.Wait(ctx, pushed, opt.serveWait, os.Stderr)
	}

	if err := writeDeadList(opt.deadOut, dead); err != nil {
		return err
	}
	if opt.deadOut != "" && len(dead) > 0 {
		fmt.Fprintf(os.Stderr, "%d silent addresses written to %s\n", len(dead), opt.deadOut)
	}

	return writeResults(opt.out, results)
}

// noReplyStatus names why an address was skipped, in the terms of the check
// that skipped it.
func noReplyStatus(probe string) string {
	switch probe {
	case "icmp":
		return "No ping reply"
	case "tcp":
		return "No SSH port"
	default:
		return "No response"
	}
}

// sweep runs the reachability pass and returns the addresses worth an SSH
// session, plus the raw result for every address so dead rows keep their timing.
func sweep(ctx context.Context, hosts []string, opt options) ([]string, []string, map[string]ap.PingResult) {
	mode := ap.ProbeMode(opt.probe)
	opts := ap.SweepOptions{
		Mode:        mode,
		Timeout:     opt.pingTimeout,
		Retries:     opt.pingRetries,
		Concurrency: opt.pingConcurrency,
		SSHPort:     opt.sshPort,
	}
	if mode == ap.ProbeNone {
		fmt.Fprintf(os.Stderr, "Skipping reachability check, trying all %d addresses\n", len(hosts))
		return hosts, nil, map[string]ap.PingResult{}
	}

	start := time.Now()
	fmt.Fprintf(os.Stderr, "Probing %d addresses (%s, %v timeout, %d retries, %d at a time)...\n",
		len(hosts), mode, opt.pingTimeout, opt.pingRetries, opt.pingConcurrency)
	res := ap.Sweep(ctx, hosts, opts)

	// If ICMP could not be opened at all, every host looks dead, which would
	// silently skip the whole run. Say so and fall back rather than report a
	// site full of dead APs.
	if mode == ap.ProbeICMP {
		if err := ap.ICMPUnavailable(); err != nil {
			fmt.Fprintf(os.Stderr, "ICMP unavailable (%v)\n  falling back to a TCP probe on port %s; use -probe tcp to silence this\n", err, opt.sshPort)
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
	fmt.Fprintf(os.Stderr, "%d of %d answered in %s; %d skipped\n\n",
		len(alive), len(hosts), time.Since(start).Round(time.Millisecond), len(dead))
	reportDead(os.Stderr, dead, string(opts.Mode), opt.verbose)
	return alive, dead, res
}

func loadHosts(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // rows may carry extra columns; only the first matters
	// Exported AP lists routinely carry stray quotes mid-field. Nothing here
	// depends on strict quoting, and refusing the whole file over one is worse
	// than reading it.
	r.LazyQuotes = true
	var hosts []string
	seen := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) == 0 {
			continue
		}
		// Excel and Notepad write a UTF-8 BOM. On a file with a header row it
		// lands on the header and is discarded with it; on a bare list of
		// addresses it lands on the first address and used to reject the row.
		h := strings.TrimSpace(strings.Trim(strings.TrimPrefix(rec[0], "\ufeff"), `"`))
		// Accept anything that parses as an IP; that also skips the header row.
		if net.ParseIP(h) == nil || seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func writeResults(path string, results []ap.Result) error {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(strings.ToLower(path), ".json") {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"IP Address", "MAC Address", "Model", "Fw Version", "Ping (ms)", "Result", "Firmware Push", "Error"}); err != nil {
		return err
	}
	for _, r := range results {
		ping := "Timeout"
		if r.Reachable {
			ping = fmt.Sprintf("%.1f", r.PingMS)
		}
		if err := w.Write([]string{r.IP, r.MAC, r.Model, r.Firmware, ping, r.Status, r.FwStatus, r.Error}); err != nil {
			return err
		}
	}
	return w.Error()
}
