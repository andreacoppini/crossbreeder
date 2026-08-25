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
	"path/filepath"
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
	ui        bool
	uiPort    int
	showVers  bool

	watchEnabled  bool
	watch         time.Duration // optional cap; 0 means until stopped
	watchInterval time.Duration

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

	// No arguments at all means somebody double-clicked the exe, so open the
	// console instead of printing a usage error at a window that vanishes.
	// os.Args is the direct question; flag.NFlag() only counts what parsed.
	if opt.ui || len(os.Args) == 1 {
		return serveUI(opt)
	}

	// Without this, an empty -csv reaches os.Open("") and reports "open : The
	// system cannot find the file specified", which names neither the problem
	// nor the way out.
	if opt.csvPath == "" {
		return fmt.Errorf("no address list given: pass -csv <file>, or run %s with no arguments (or -ui) to open the console",
			filepath.Base(os.Args[0]))
	}

	hosts, err := loadHosts(opt.csvPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", opt.csvPath, err)
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no IP addresses found in %s", opt.csvPath)
	}

	password, err := resolvePassword(opt)
	if err != nil {
		return err
	}

	cfg, notes, err := buildConfig(opt, password)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(os.Stderr, "note:", n)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out, err := runJob(ctx, opt, hosts, cfg, cliPrinter(opt), nil)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n%d addresses, %d alive, %d contacted in %s\n",
		len(out.Results), countAlive(out.Results), countContacted(out.Results),
		out.Elapsed.Round(time.Millisecond))

	if err := writeDeadList(opt.deadOut, out.Dead); err != nil {
		return err
	}
	if opt.deadOut != "" && len(out.Dead) > 0 {
		fmt.Fprintf(os.Stderr, "%d silent addresses written to %s\n", len(out.Dead), opt.deadOut)
	}
	return writeResults(opt.out, out.Results)
}

// cliPrinter renders run events the way the command line always has.
func cliPrinter(opt options) Emitter {
	var mu sync.Mutex
	return func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Kind {
		case EvLog, EvTransfer:
			fmt.Fprintln(os.Stderr, e.Message)
		case EvServer:
			fmt.Fprintf(os.Stderr, "Pushing %s\nServing %s on http://%s\n", e.Server.File, e.Server.Dir, e.Server.Addr)
			if e.Server.Reason != "" {
				fmt.Fprintf(os.Stderr, "  address chosen: %s\n", e.Server.Reason)
			}
			if opt.verbose {
				for _, c := range e.Server.Considered {
					fmt.Fprintf(os.Stderr, "    %s\n", c)
				}
			}
		case EvSweep:
			fmt.Fprintln(os.Stderr)
			reportDead(os.Stderr, e.Dead, opt.probe, opt.verbose)
		case EvResult:
			r := e.Result
			detail := firstNonEmpty(r.Error, r.Note, r.FwStatus)
			fmt.Fprintf(os.Stderr, "[%d/%d] %-15s %-12s %-10s %-14s %s\n",
				e.Done, e.Total, r.IP, r.Status, r.Model, r.Firmware, detail)
			if opt.verbose && r.Transcript != "" {
				fmt.Fprintf(os.Stderr, "----- %s -----\n%s\n", r.IP, r.Transcript)
			}
		case EvPhase:
			if e.Phase == "download" {
				fmt.Fprintf(os.Stderr, "\nWaiting for %d AP(s) to download (Ctrl-C to stop)\n", e.Total)
			}
		}
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func countAlive(rs []ap.Result) int {
	n := 0
	for _, r := range rs {
		if r.Reachable {
			n++
		}
	}
	return n
}

func countContacted(rs []ap.Result) int {
	n := 0
	for _, r := range rs {
		if r.Status != "" && r.Reachable {
			n++
		}
	}
	return n
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
	if err := w.Write([]string{"IP Address", "MAC Address", "Model", "Fw Version", "Ping (ms)", "Result", "Firmware Push", "Watch", "Error"}); err != nil {
		return err
	}
	for _, r := range results {
		ping := "Timeout"
		if r.Reachable {
			ping = fmt.Sprintf("%.1f", r.PingMS)
		}
		if err := w.Write([]string{r.IP, r.MAC, r.Model, r.Firmware, ping, r.Status, r.FwStatus, r.Note, r.Error}); err != nil {
			return err
		}
	}
	return w.Error()
}
