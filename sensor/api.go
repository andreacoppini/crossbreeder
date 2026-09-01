package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/andreacoppini/crossbreeder/sensor/l2"
	"github.com/andreacoppini/crossbreeder/sensor/netprobe"
	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

//go:embed web
var webAssets embed.FS

// API is the sensor's own web interface: a dashboard for whoever walks up to
// it, and the same data as JSON for whoever automates it. There is no second
// implementation — the page is drawn from these endpoints, so the two cannot
// drift apart.
type API struct {
	sched   *Scheduler
	store   *Store
	cfgPath string
	cfg     *Config
	version string
	log     func(string, ...any)
	// onConfig is called when the configuration is replaced through the API,
	// so the process can restart the loop with it.
	onConfig func(Config) error
}

// NewAPI builds the handler set.
func NewAPI(sched *Scheduler, store *Store, cfg *Config, cfgPath, version string, log func(string, ...any)) *API {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &API{sched: sched, store: store, cfg: cfg, cfgPath: cfgPath, version: version, log: log}
}

// Handler returns the whole interface, ready to serve.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("GET /api/latest", a.handleLatest)
	mux.HandleFunc("GET /api/results", a.handleResults)
	mux.HandleFunc("GET /api/issues", a.handleIssues)
	mux.HandleFunc("GET /api/series", a.handleSeries)
	mux.HandleFunc("GET /api/apps", a.handleApps)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("PUT /api/config", a.handlePutConfig)
	mux.HandleFunc("POST /api/run", a.handleRun)
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("GET /api/scan", a.handleScan)
	mux.HandleFunc("GET /api/traceroute", a.handleTraceroute)
	mux.HandleFunc("GET /api/capture", a.handleCapture)
	mux.HandleFunc("GET /api/export", a.handleExport)
	mux.HandleFunc("GET /metrics", a.handleMetrics)

	assets, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err) // the assets are embedded at build time; this cannot fail at runtime
	}
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	return mux
}

func (a *API) handleState(w http.ResponseWriter, r *http.Request) {
	type networkView struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Enabled bool   `json:"enabled"`
		SSID    string `json:"ssid,omitempty"`
	}
	var networks []networkView
	for _, n := range a.cfg.Networks {
		networks = append(networks, networkView{
			Name: n.Name, Kind: n.Kind, Enabled: n.On(), SSID: n.Profile.SSID,
		})
	}
	writeJSON(w, map[string]any{
		"sensor":   a.cfg.Sensor.Name,
		"site":     a.cfg.Sensor.Site,
		"group":    a.cfg.Sensor.Group,
		"version":  a.version,
		"interval": a.cfg.Sensor.Interval.D().String(),
		"state":    a.sched.State(),
		"networks": networks,
		"issues":   len(a.sched.Issues().Open()),
	})
}

func (a *API) handleLatest(w http.ResponseWriter, r *http.Request) {
	latest := a.store.Latest()
	out := make([]SuiteResult, 0, len(latest))
	for _, v := range latest {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Network < out[j].Network })
	writeJSON(w, out)
}

func (a *API) handleResults(w http.ResponseWriter, r *http.Request) {
	from, to := window(r)
	results, err := a.store.Query(from, to, r.URL.Query().Get("network"))
	if err != nil {
		httpErr(w, err)
		return
	}
	if limit := intParam(r, "limit", 0); limit > 0 && len(results) > limit {
		results = results[len(results)-limit:]
	}
	writeJSON(w, results)
}

func (a *API) handleIssues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.sched.Issues().Open())
}

func (a *API) handleSeries(w http.ResponseWriter, r *http.Request) {
	from, to := window(r)
	network := r.URL.Query().Get("network")
	test := r.URL.Query().Get("test")
	var (
		points []Point
		err    error
	)
	if test == "" || test == "score" {
		points, err = a.store.ScoreSeries(network, from, to)
	} else {
		points, err = a.store.Series(network, test, from, to)
	}
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, points)
}

func (a *API) handleApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, AppsByCategory())
}

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.cfg.Redacted())
}

// handlePutConfig replaces the configuration. It validates first and saves
// only what validates: a sensor that accepts a broken config and then cannot
// start is a sensor somebody has to drive to.
func (a *API) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var incoming Config
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&incoming); err != nil {
		httpErr(w, err)
		return
	}
	incoming.applyDefaults()
	if err := incoming.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// A redacted secret coming back from the dashboard means "leave it alone",
	// not "set the password to eight asterisks".
	incoming.restoreSecrets(*a.cfg)
	if a.cfgPath != "" {
		if err := incoming.Save(a.cfgPath); err != nil {
			httpErr(w, err)
			return
		}
	}
	if a.onConfig != nil {
		if err := a.onConfig(incoming); err != nil {
			httpErr(w, err)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "saved"})
}

func (a *API) handleRun(w http.ResponseWriter, r *http.Request) {
	a.sched.Trigger()
	writeJSON(w, map[string]string{"status": "a pass has been asked for"})
}

// handleEvents is the live view: one server-sent event per pass.
func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported here", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, stop := a.sched.Subscribe()
	defer stop()

	// An immediate comment gets the browser's EventSource out of "connecting".
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case result, ok := <-events:
			if !ok {
				return
			}
			blob, err := json.Marshal(result)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: pass\ndata: %s\n\n", blob)
			flusher.Flush()
		case <-ticker.C:
			// A keep-alive, so a proxy in between does not time the stream out.
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

// handleScan runs a scan now and reports what the radio can hear. This is the
// screen somebody standing in the room wants: not history, but what is on the
// air at this moment.
func (a *API) handleScan(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("interface")
	if iface == "" {
		iface = a.cfg.Sensor.MonitorInterface
	}
	if iface == "" {
		iface = a.cfg.Sensor.WirelessInterface
	}
	ctrl, err := wifi.Dial(a.cfg.Sensor.CtrlDir, iface)
	if err != nil {
		httpErr(w, err)
		return
	}
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	bsses, err := ctrl.Scan(ctx, 5*time.Second)
	if err != nil {
		httpErr(w, err)
		return
	}
	survey, _ := wifi.ChannelSurvey(ctx, iface)
	writeJSON(w, map[string]any{"radios": bsses, "survey": survey})
}

func (a *API) handleTraceroute(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "no target", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	writeJSON(w, netprobe.Traceroute(ctx, target, intParam(r, "hops", 20), time.Second))
}

// handleCapture streams a packet capture straight back to the browser, so a
// remote capture needs no storage on the sensor and no second request to
// fetch it.
func (a *API) handleCapture(w http.ResponseWriter, r *http.Request) {
	opts := l2.CaptureOptions{
		Interface: r.URL.Query().Get("interface"),
		Snaplen:   intParam(r, "snaplen", 0),
		MaxPacket: intParam(r, "packets", 0),
		Duration:  time.Duration(intParam(r, "seconds", 30)) * time.Second,
		Filter: l2.Filter{
			Host:  r.URL.Query().Get("host"),
			Port:  intParam(r, "port", 0),
			Proto: r.URL.Query().Get("proto"),
		},
	}
	if opts.Interface == "" {
		opts.Interface = a.cfg.Sensor.WiredInterface
	}
	name := fmt.Sprintf("%s-%s.pcap", a.cfg.Sensor.Name, time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)

	// The response is streamed, so the header is already on the wire by the
	// time a capture fails. The failure goes to the log rather than a status
	// code, and the download simply ends.
	if _, err := l2.Capture(r.Context(), opts, &flushWriter{w: w}); err != nil {
		a.log("capture on %s: %v", opts.Interface, err)
	}
}

// flushWriter pushes each packet out as it is captured, so a capture that is
// still running is already downloading.
type flushWriter struct{ w io.Writer }

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if flusher, ok := f.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

// handleExport writes the history out as CSV, one row per measurement, which
// is the shape a spreadsheet and a ticketing system both want.
func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	from, to := window(r)
	results, err := a.store.Query(from, to, r.URL.Query().Get("network"))
	if err != nil {
		httpErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+a.cfg.Sensor.Name+"-results.csv")
	fmt.Fprintln(w, "time,sensor,site,network,kind,service,test,target,status,value,unit,detail,error")
	for _, res := range results {
		for _, m := range res.Measurements {
			fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%.3f,%s,%s,%s\n",
				m.At.Format(time.RFC3339), csv(res.Sensor), csv(res.Site), csv(res.Network), csv(res.Kind),
				csv(string(m.Service)), csv(m.Test), csv(m.Target), m.Status, m.Value, csv(m.Unit),
				csv(m.Detail), csv(m.Error))
		}
	}
}

// csv quotes a field for the export. Detail strings hold commas and the
// occasional quotation mark.
func csv(s string) string {
	if !strings.ContainsAny(s, ",\"\n") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// handleMetrics is the Prometheus endpoint, which is how this becomes part of
// monitoring a site already has rather than another screen to watch.
func (a *API) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	sensor := a.cfg.Sensor.Name

	fmt.Fprintln(w, "# HELP sensor_up whether the sensor process is running")
	fmt.Fprintln(w, "# TYPE sensor_up gauge")
	fmt.Fprintf(w, "sensor_up{sensor=%q,site=%q} 1\n", sensor, a.cfg.Sensor.Site)

	fmt.Fprintln(w, "# HELP sensor_score health score, 0 to 100")
	fmt.Fprintln(w, "# TYPE sensor_score gauge")
	fmt.Fprintln(w, "# HELP sensor_measurement the value of one test, in the unit named by the label")
	fmt.Fprintln(w, "# TYPE sensor_measurement gauge")
	fmt.Fprintln(w, "# HELP sensor_test_status 0 ok, 1 warning, 2 failure, 3 skipped")
	fmt.Fprintln(w, "# TYPE sensor_test_status gauge")

	latest := a.store.Latest()
	networks := make([]string, 0, len(latest))
	for name := range latest {
		networks = append(networks, name)
	}
	sort.Strings(networks)

	for _, name := range networks {
		res := latest[name]
		fmt.Fprintf(w, "sensor_score{sensor=%q,network=%q,service=\"overall\"} %d\n", sensor, name, res.Overall)
		services := make([]string, 0, len(res.Scores))
		for s := range res.Scores {
			services = append(services, string(s))
		}
		sort.Strings(services)
		for _, s := range services {
			fmt.Fprintf(w, "sensor_score{sensor=%q,network=%q,service=%q} %d\n",
				sensor, name, s, res.Scores[Service(s)])
		}
		for _, m := range res.Measurements {
			if m.Status == StatusSkipped {
				continue
			}
			fmt.Fprintf(w, "sensor_measurement{sensor=%q,network=%q,service=%q,test=%q,unit=%q} %g\n",
				sensor, name, m.Service, m.Test, m.Unit, m.Value)
			fmt.Fprintf(w, "sensor_test_status{sensor=%q,network=%q,service=%q,test=%q} %d\n",
				sensor, name, m.Service, m.Test, metricStatus(m.Status))
		}
	}

	fmt.Fprintln(w, "# HELP sensor_issue an open issue, 1 while it is open")
	fmt.Fprintln(w, "# TYPE sensor_issue gauge")
	for _, issue := range a.sched.Issues().Open() {
		fmt.Fprintf(w, "sensor_issue{sensor=%q,network=%q,service=%q,severity=%q,root_cause=\"%t\"} 1\n",
			sensor, issue.Network, issue.Service, issue.Severity, issue.RootCause)
	}
}

func metricStatus(s Status) int {
	switch s {
	case StatusOK:
		return 0
	case StatusWarn:
		return 1
	case StatusFail:
		return 2
	}
	return 3
}

// window reads a from/to pair, accepting either an absolute time or a
// relative one like "-6h", which is what a dashboard link carries.
func window(r *http.Request) (from, to time.Time) {
	return parseTime(r.URL.Query().Get("from")), parseTime(r.URL.Query().Get("to"))
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if strings.HasPrefix(s, "-") {
		if d, err := time.ParseDuration(s); err == nil {
			return time.Now().Add(d)
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(secs, 0)
	}
	return time.Time{}
}

func intParam(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func httpErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
