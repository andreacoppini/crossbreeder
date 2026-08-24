package main

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

//go:embed web
var webAssets embed.FS

// uiServer is the browser console. It binds loopback only: the firmware server
// has to be reachable by the APs, but nothing should be able to drive a fleet
// of access points from off this machine.
type uiServer struct {
	opt options

	mu       sync.Mutex
	subs     map[chan Event]struct{}
	history  []Event // replayed to a client that connects mid-run
	running  bool
	cancel   context.CancelFunc
	results  []ap.Result
	dead     []string
	lastCfg  string
	finished bool
}

func serveUI(opt options) error {
	u := &uiServer{opt: opt, subs: map[chan Event]struct{}{}}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opt.uiPort))
	if err != nil {
		return fmt.Errorf("cannot start the console: %w", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	content, err := fs.Sub(webAssets, "web")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(content)))
	mux.HandleFunc("/api/events", u.handleEvents)
	mux.HandleFunc("/api/run", u.handleRun)
	mux.HandleFunc("/api/stop", u.handleStop)
	mux.HandleFunc("/api/state", u.handleState)
	mux.HandleFunc("/api/hosts", u.handleHosts)
	mux.HandleFunc("/api/export", u.handleExport)
	mux.HandleFunc("/api/defaults", u.handleDefaults)

	fmt.Printf("Crossbreeder console: %s\n(leave this window open; close it or press Ctrl-C to quit)\n", url)
	openBrowser(url)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

// ---- event fan-out ----

func (u *uiServer) publish(e Event) {
	u.mu.Lock()
	u.history = append(u.history, e)
	if e.Kind == EvResult && e.Result != nil {
		u.results = append(u.results, *e.Result)
	}
	if e.Kind == EvSweep {
		u.dead = e.Dead
	}
	subs := make([]chan Event, 0, len(u.subs))
	for c := range u.subs {
		subs = append(subs, c)
	}
	u.mu.Unlock()

	for _, c := range subs {
		select {
		case c <- e:
		default: // a slow browser must not stall the run
		}
	}
}

func (u *uiServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Event, 256)
	u.mu.Lock()
	u.subs[ch] = struct{}{}
	backlog := append([]Event(nil), u.history...)
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		delete(u.subs, ch)
		u.mu.Unlock()
	}()

	send := func(e Event) bool {
		b, err := json.Marshal(e)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Replay so a reload mid-run does not lose the table.
	for _, e := range backlog {
		if !send(e) {
			return
		}
	}

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if !send(e) {
				return
			}
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ---- run control ----

// runRequest is the console form. It mirrors the flags rather than inventing a
// second vocabulary, so anything learned in one surface transfers to the other.
type runRequest struct {
	Hosts []string `json:"hosts"`

	User        string `json:"user"`
	Pass        string `json:"pass"`
	AlsoDefault bool   `json:"alsoDefault"`
	Concurrency int    `json:"concurrency"`

	Probe           string `json:"probe"`
	PingTimeoutMS   int    `json:"pingTimeoutMs"`
	PingRetries     int    `json:"pingRetries"`
	PingConcurrency int    `json:"pingConcurrency"`

	Firmware bool   `json:"firmware"`
	Factory  bool   `json:"factory"`
	Reboot   bool   `json:"reboot"`
	Command  string `json:"command"`

	Serve     bool   `json:"serve"`
	ServeDir  string `json:"serveDir"`
	ServeIP   string `json:"serveIp"`
	ServePort int    `json:"servePort"`
	FwProto   string `json:"fwProto"`
	FwHost    string `json:"fwHost"`
	FwPort    string `json:"fwPort"`
	FwUser    string `json:"fwUser"`
	FwPass    string `json:"fwPass"`
	FwFile    string `json:"fwFile"`
	FwWaitS   int    `json:"fwWaitS"`

	SSHPort    string `json:"sshPort"`
	TimeoutS   int    `json:"timeoutS"`
	Legacy     bool   `json:"legacy"`
	ServeWaitS int    `json:"serveWaitS"`
}

func (u *uiServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req runRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		httpErr(w, err)
		return
	}
	if len(req.Hosts) == 0 {
		httpErr(w, fmt.Errorf("no addresses to work on"))
		return
	}

	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		httpErr(w, fmt.Errorf("a run is already in progress"))
		return
	}
	u.running = true
	u.finished = false
	u.history = nil
	u.results = nil
	u.dead = nil
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.mu.Unlock()

	opt := u.opt.merge(req)
	cfg, notes, err := buildConfig(opt, req.Pass)
	if err != nil {
		u.finish()
		httpErr(w, err)
		return
	}

	go func() {
		defer u.finish()
		defer cancel()
		for _, n := range notes {
			u.publish(Event{Kind: EvLog, Message: "note: " + n})
		}
		out, err := runJob(ctx, opt, req.Hosts, cfg, u.publish)
		if err != nil {
			u.publish(Event{Kind: EvError, Message: err.Error()})
			return
		}
		u.mu.Lock()
		u.results = out.Results
		u.dead = out.Dead
		u.mu.Unlock()
		u.publish(Event{Kind: EvDone, Total: len(out.Results),
			Done: countContacted(out.Results), Elapsed: out.Elapsed.Round(time.Millisecond).String()})
	}()

	writeJSON(w, map[string]any{"ok": true})
}

func (u *uiServer) finish() {
	u.mu.Lock()
	u.running = false
	u.finished = true
	u.mu.Unlock()
}

func (u *uiServer) handleStop(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	cancel := u.cancel
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	u.publish(Event{Kind: EvLog, Message: "Stopping..."})
	writeJSON(w, map[string]any{"ok": true})
}

func (u *uiServer) handleState(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	writeJSON(w, map[string]any{"running": u.running, "finished": u.finished, "results": len(u.results)})
}

// handleDefaults seeds the form from the flags the process was started with, so
// the console and the command line agree about what "default" means.
func (u *uiServer) handleDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"user":            u.opt.user,
		"concurrency":     u.opt.concurrency,
		"probe":           u.opt.probe,
		"pingTimeoutMs":   int(u.opt.pingTimeout / time.Millisecond),
		"pingRetries":     u.opt.pingRetries,
		"pingConcurrency": u.opt.pingConcurrency,
		"sshPort":         u.opt.sshPort,
		"timeoutS":        int(u.opt.timeout / time.Second),
		"legacy":          u.opt.legacy,
		"fwProto":         u.opt.fwProto,
		"fwPort":          u.opt.fwPort,
		"servePort":       u.opt.servePort,
		"serveWaitS":      int(u.opt.serveWait / time.Second),
		"serveDir":        workingDir(),
		"version":         version,
	})
}

// handleHosts turns pasted text or an uploaded CSV into an address list, using
// the same parser the -csv flag uses.
func (u *uiServer) handleHosts(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		httpErr(w, err)
		return
	}
	hosts, skipped := parseHostsText(string(body))
	writeJSON(w, map[string]any{"hosts": hosts, "skipped": skipped})
}

func (u *uiServer) handleExport(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	results := append([]ap.Result(nil), u.results...)
	dead := append([]string(nil), u.dead...)
	u.mu.Unlock()

	switch r.URL.Query().Get("kind") {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="crossbreeder-results.json"`)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	case "dead":
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `attachment; filename="crossbreeder-dead.txt"`)
		_, _ = io.WriteString(w, strings.Join(dead, "\n")+"\n")
	default:
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="crossbreeder-results.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"IP Address", "MAC Address", "Model", "Fw Version", "Ping (ms)", "Result", "Firmware Push", "Error"})
		for _, res := range results {
			ping := "Timeout"
			if res.Reachable {
				ping = fmt.Sprintf("%.1f", res.PingMS)
			}
			_ = cw.Write([]string{res.IP, res.MAC, res.Model, res.Firmware, ping, res.Status, res.FwStatus, res.Error})
		}
		cw.Flush()
	}
}

// merge folds the console form onto the options the process started with.
func (o options) merge(r runRequest) options {
	out := o
	out.user = r.User
	out.pass = r.Pass
	out.alsoDefault = r.AlsoDefault
	out.fw = r.Firmware
	out.factory = r.Factory
	out.reboot = r.Reboot
	out.command = r.Command
	out.fwProto, out.fwHost, out.fwPort = r.FwProto, r.FwHost, r.FwPort
	out.fwUser, out.fwPass, out.fwFile = r.FwUser, r.FwPass, r.FwFile
	out.serveIP = r.ServeIP
	out.serveDir = ""
	if r.Serve {
		out.serveDir = r.ServeDir
		if out.serveDir == "" {
			out.serveDir = workingDir()
		}
	}
	if r.Probe != "" {
		out.probe = r.Probe
	}
	if r.SSHPort != "" {
		out.sshPort = r.SSHPort
	}
	out.legacy = r.Legacy
	setIfPositive(&out.concurrency, r.Concurrency)
	setIfPositive(&out.pingRetries, r.PingRetries+1) // 0 is a legitimate choice
	out.pingRetries = max(0, r.PingRetries)
	setIfPositive(&out.pingConcurrency, r.PingConcurrency)
	setIfPositive(&out.servePort, r.ServePort+1)
	out.servePort = max(0, r.ServePort)
	setDurIfPositive(&out.pingTimeout, time.Duration(r.PingTimeoutMS)*time.Millisecond)
	setDurIfPositive(&out.timeout, time.Duration(r.TimeoutS)*time.Second)
	setDurIfPositive(&out.serveWait, time.Duration(r.ServeWaitS)*time.Second)
	setDurIfPositive(&out.fwWait, time.Duration(r.FwWaitS)*time.Second)
	return out
}

func setIfPositive(dst *int, v int) {
	if v > 0 {
		*dst = v
	}
}

func setDurIfPositive(dst *time.Duration, v time.Duration) {
	if v > 0 {
		*dst = v
	}
}

// parseHostsText accepts a pasted list or a whole CSV and returns the addresses
// it recognised plus the lines it could not use.
func parseHostsText(text string) (hosts []string, skipped []string) {
	seen := map[string]bool{}
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if line == "" {
			continue
		}
		field := strings.TrimSpace(strings.Trim(strings.Split(line, ",")[0], `"`))
		if net.ParseIP(field) == nil {
			skipped = append(skipped, line)
			continue
		}
		if seen[field] {
			continue
		}
		seen[field] = true
		hosts = append(hosts, field)
	}
	return hosts, skipped
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// openBrowser is best-effort: the URL is printed either way.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
