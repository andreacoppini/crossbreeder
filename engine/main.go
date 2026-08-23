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
	alsoDefault bool
	concurrency int

	fw       bool
	fwProto  string
	fwHost   string
	fwPort   string
	fwUser   string
	fwPass   string
	fwFile   string
	factory  bool
	reboot   bool
	command  string
	sshPort  string
	timeout  time.Duration
	legacy   bool
	verbose  bool
	showVers bool
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
	hosts, err := loadHosts(opt.csvPath)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no IP addresses found in %s", opt.csvPath)
	}

	creds := []ap.Credentials{}
	if opt.user != "" {
		creds = append(creds, ap.Credentials{User: opt.user, Password: opt.pass})
	}
	if opt.alsoDefault || len(creds) == 0 {
		creds = append(creds, ap.Credentials{User: "super", Password: "sp-admin"})
	}

	cfg := ap.Config{
		Credentials: creds,
		Actions: ap.Actions{
			UpdateFirmware: opt.fw,
			FactoryReset:   opt.factory,
			CustomCommand:  opt.command,
			Reboot:         opt.reboot,
		},
		Firmware: ap.Firmware{
			Proto: opt.fwProto, Host: opt.fwHost, Port: opt.fwPort,
			User: opt.fwUser, Password: opt.fwPass, Filename: opt.fwFile,
		},
		Port:             opt.sshPort,
		ConnectTimeout:   opt.timeout,
		DialogTimeout:    opt.timeout,
		Deadline:         opt.timeout * 12,
		LegacyAlgorithms: opt.legacy,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var mu sync.Mutex
	done := 0
	start := time.Now()

	rn := &Runner{
		Concurrency: opt.concurrency,
		Config:      cfg,
		OnResult: func(_ int, r ap.Result) {
			mu.Lock()
			done++
			n := done
			mu.Unlock()
			fmt.Fprintf(os.Stderr, "[%d/%d] %-15s %-12s %-10s %-14s %s\n",
				n, len(hosts), r.IP, r.Status, r.Model, r.Firmware, r.Error)
			if opt.verbose && r.Transcript != "" {
				fmt.Fprintf(os.Stderr, "----- %s -----\n%s\n", r.IP, r.Transcript)
			}
		},
	}

	results := rn.Run(ctx, hosts)
	elapsed := time.Since(start)

	fmt.Fprintf(os.Stderr, "\n%d APs in %s (%d workers)\n", len(hosts), elapsed.Round(time.Millisecond), opt.concurrency)

	return writeResults(opt.out, results)
}

func loadHosts(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // rows may carry extra columns; only the first matters
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
		h := strings.TrimSpace(strings.Trim(rec[0], `"`))
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
	if err := w.Write([]string{"IP Address", "MAC Address", "Model", "Fw Version", "Probe (ms)", "Result", "Error"}); err != nil {
		return err
	}
	for _, r := range results {
		probe := ""
		if r.Reachable {
			probe = fmt.Sprintf("%.1f", r.ProbeMS)
		}
		if err := w.Write([]string{r.IP, r.MAC, r.Model, r.Firmware, probe, r.Status, r.Error}); err != nil {
			return err
		}
	}
	return w.Error()
}
