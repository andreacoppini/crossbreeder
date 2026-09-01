package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A collector is what turns a shelf of sensors into a fleet: it takes their
// results, keeps the history, shows one page for all of them, and hands back
// the work they should do next. It is the same binary — a sensor with no
// radio and a different job.
//
// Sensors reach it over HTTPS with a token. Nothing else is exposed: the
// collector never connects to a sensor, which is what lets sensors sit behind
// NAT on a customer's network with nothing forwarded.
type Collector struct {
	dir     string
	tokens  map[string]string // token -> the sensor it may report as, or "*"
	admin   string            // token for the fleet views; empty leaves them open
	version string
	log     func(string, ...any)

	mu       sync.Mutex
	sensors  map[string]*FleetSensor
	stores   map[string]*Store
	commands map[string][]Command
	pending  map[string]*Config
	storage  Storage
}

// FleetSensor is what the collector knows about one sensor.
type FleetSensor struct {
	Name     string                 `json:"name"`
	Site     string                 `json:"site,omitempty"`
	Group    string                 `json:"group,omitempty"`
	Version  string                 `json:"version,omitempty"`
	Address  string                 `json:"address,omitempty"`
	LastSeen time.Time              `json:"last_seen"`
	Overall  int                    `json:"overall"`
	Networks map[string]SuiteResult `json:"networks"`
	Issues   []Issue                `json:"issues,omitempty"`
}

// Online reports whether the sensor has been heard from recently enough to
// believe. A sensor that has gone quiet is itself a finding — usually the
// site's power or its uplink.
func (f FleetSensor) Online() bool { return time.Since(f.LastSeen) < 20*time.Minute }

// Command is work the collector hands back to a sensor on its next report.
// The sensor asks; the collector never connects to it.
type Command struct {
	ID     string            `json:"id"`
	Action string            `json:"action"` // run, config, update, restart
	Params map[string]string `json:"params,omitempty"`
	Issued time.Time         `json:"issued"`
}

// Report is what a sensor sends.
type Report struct {
	Sensor   string        `json:"sensor"`
	Site     string        `json:"site,omitempty"`
	Group    string        `json:"group,omitempty"`
	Version  string        `json:"version,omitempty"`
	Results  []SuiteResult `json:"results,omitempty"`
	Issues   []Issue       `json:"issues,omitempty"`
	Finished []string      `json:"finished,omitempty"` // command IDs the sensor has carried out
}

// Reply is what it gets back.
type Reply struct {
	Commands []Command `json:"commands,omitempty"`
	Config   *Config   `json:"config,omitempty"`
	Accepted int       `json:"accepted"`
}

// NewCollector builds one. tokens maps a shared secret to the sensor name it
// may report as; "*" lets one token serve a whole fleet, which is what a site
// deploying twenty sensors from one image needs.
func NewCollector(dir string, tokens map[string]string, admin, version string, storage Storage, log func(string, ...any)) *Collector {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Collector{
		dir: dir, tokens: tokens, admin: admin, version: version, storage: storage, log: log,
		sensors: map[string]*FleetSensor{}, stores: map[string]*Store{},
		commands: map[string][]Command{}, pending: map[string]*Config{},
	}
}

// Handler is the collector's whole interface.
func (c *Collector) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/ingest", c.handleIngest)
	mux.HandleFunc("GET /api/fleet", c.adminOnly(c.handleFleet))
	mux.HandleFunc("GET /api/sensors/{name}", c.adminOnly(c.handleSensor))
	mux.HandleFunc("GET /api/sensors/{name}/results", c.adminOnly(c.handleSensorResults))
	mux.HandleFunc("POST /api/sensors/{name}/command", c.adminOnly(c.handleCommand))
	mux.HandleFunc("PUT /api/sensors/{name}/config", c.adminOnly(c.handlePushConfig))
	mux.HandleFunc("GET /metrics", c.adminOnly(c.handleMetrics))
	mux.HandleFunc("GET /", c.adminOnly(c.handleFleetPage))
	return mux
}

// adminOnly gates the fleet views. With no admin token set they are open,
// which is right for a collector bound to localhost or behind a reverse proxy
// that has already authenticated, and wrong for anything else — so the flag
// that leaves it empty says so.
func (c *Collector) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.admin != "" && bearer(r) != c.admin {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func bearer(r *http.Request) string {
	if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); token != "" {
		return strings.TrimSpace(token)
	}
	return r.URL.Query().Get("token")
}

func (c *Collector) handleIngest(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	allowed, ok := c.tokens[token]
	if !ok {
		http.Error(w, "unauthorised", http.StatusUnauthorized)
		return
	}
	var report Report
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&report); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if report.Sensor == "" {
		http.Error(w, "the report does not say which sensor it is from", http.StatusBadRequest)
		return
	}
	// A token tied to one sensor may only report as that sensor: otherwise one
	// compromised sensor could overwrite the whole fleet's history.
	if allowed != "*" && allowed != report.Sensor {
		http.Error(w, "this token may not report as "+report.Sensor, http.StatusForbidden)
		return
	}

	c.mu.Lock()
	sensor, known := c.sensors[report.Sensor]
	if !known {
		sensor = &FleetSensor{Name: report.Sensor, Networks: map[string]SuiteResult{}}
		c.sensors[report.Sensor] = sensor
	}
	sensor.Site, sensor.Group, sensor.Version = report.Site, report.Group, report.Version
	sensor.LastSeen = time.Now()
	sensor.Address = clientAddr(r)
	sensor.Issues = report.Issues
	for _, result := range report.Results {
		if existing, ok := sensor.Networks[result.Network]; !ok || result.Start.After(existing.Start) {
			sensor.Networks[result.Network] = result
		}
	}
	sensor.Overall = worstScore(sensor.Networks)
	store := c.storeFor(report.Sensor)

	// Anything the sensor says it has done is off the list.
	if len(report.Finished) > 0 {
		done := map[string]bool{}
		for _, id := range report.Finished {
			done[id] = true
		}
		var left []Command
		for _, cmd := range c.commands[report.Sensor] {
			if !done[cmd.ID] {
				left = append(left, cmd)
			}
		}
		c.commands[report.Sensor] = left
	}
	reply := Reply{Commands: c.commands[report.Sensor], Accepted: len(report.Results)}
	if cfg, ok := c.pending[report.Sensor]; ok {
		reply.Config = cfg
		delete(c.pending, report.Sensor)
	}
	c.mu.Unlock()

	if store != nil {
		for _, result := range report.Results {
			if err := store.Append(result); err != nil {
				c.log("recording %s: %v", report.Sensor, err)
			}
		}
	}
	c.log("%s reported %d pass(es)", report.Sensor, len(report.Results))
	writeJSON(w, reply)
}

// storeFor opens (once) the history for one sensor. The caller holds the lock.
func (c *Collector) storeFor(name string) *Store {
	if c.dir == "" {
		return nil
	}
	if store, ok := c.stores[name]; ok {
		return store
	}
	storage := c.storage
	storage.Dir = filepath.Join(c.dir, safeName(name))
	store, err := NewStore(storage)
	if err != nil {
		c.log("history for %s: %v", name, err)
		return nil
	}
	c.stores[name] = store
	return store
}

// safeName keeps a sensor's name from escaping the history directory. The
// name arrives from the sensor, so it is not to be trusted with a path.
func safeName(name string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '-'
	}, name)
	clean = strings.Trim(clean, ".-")
	if clean == "" {
		return "sensor"
	}
	if len(clean) > 64 {
		clean = clean[:64]
	}
	return clean
}

func clientAddr(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func worstScore(networks map[string]SuiteResult) int {
	worst := 100
	for _, r := range networks {
		if r.Overall < worst {
			worst = r.Overall
		}
	}
	return worst
}

func (c *Collector) handleFleet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, c.Fleet())
}

// Fleet lists every sensor, worst first — which is the only order a fleet
// page is useful in.
func (c *Collector) Fleet() []FleetSensor {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]FleetSensor, 0, len(c.sensors))
	for _, s := range c.sensors {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online() != out[j].Online() {
			return !out[i].Online()
		}
		if out[i].Overall != out[j].Overall {
			return out[i].Overall < out[j].Overall
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *Collector) handleSensor(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	sensor, ok := c.sensors[r.PathValue("name")]
	var copied FleetSensor
	if ok {
		copied = *sensor
	}
	c.mu.Unlock()
	if !ok {
		http.Error(w, "no such sensor", http.StatusNotFound)
		return
	}
	writeJSON(w, copied)
}

func (c *Collector) handleSensorResults(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	store := c.storeFor(r.PathValue("name"))
	c.mu.Unlock()
	if store == nil {
		http.Error(w, "this collector is not keeping history", http.StatusNotFound)
		return
	}
	from, to := window(r)
	results, err := store.Query(from, to, r.URL.Query().Get("network"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, results)
}

func (c *Collector) handleCommand(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	action := r.URL.Query().Get("action")
	switch action {
	case "run", "update", "restart", "capture":
	default:
		http.Error(w, "unknown action "+action, http.StatusBadRequest)
		return
	}
	params := map[string]string{}
	for key, values := range r.URL.Query() {
		if key != "action" && key != "token" && len(values) > 0 {
			params[key] = values[0]
		}
	}
	cmd := Command{
		ID:     fmt.Sprintf("%s-%d", action, time.Now().UnixNano()),
		Action: action, Params: params, Issued: time.Now(),
	}
	c.mu.Lock()
	c.commands[name] = append(c.commands[name], cmd)
	c.mu.Unlock()
	// The sensor picks this up on its next report, so the answer is "queued",
	// not "done".
	writeJSON(w, map[string]any{"queued": cmd})
}

func (c *Collector) handlePushConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	c.mu.Lock()
	c.pending[name] = &cfg
	c.mu.Unlock()
	writeJSON(w, map[string]string{"status": "the configuration will be handed to " + name + " when it next reports"})
}

func (c *Collector) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP fleet_sensor_online whether a sensor has reported recently")
	fmt.Fprintln(w, "# TYPE fleet_sensor_online gauge")
	fmt.Fprintln(w, "# HELP fleet_sensor_score the worst network score on a sensor")
	fmt.Fprintln(w, "# TYPE fleet_sensor_score gauge")
	for _, s := range c.Fleet() {
		online := 0
		if s.Online() {
			online = 1
		}
		fmt.Fprintf(w, "fleet_sensor_online{sensor=%q,site=%q,group=%q} %d\n", s.Name, s.Site, s.Group, online)
		fmt.Fprintf(w, "fleet_sensor_score{sensor=%q,site=%q,group=%q} %d\n", s.Name, s.Site, s.Group, s.Overall)
		fmt.Fprintf(w, "fleet_sensor_issues{sensor=%q,site=%q,group=%q} %d\n", s.Name, s.Site, s.Group, len(s.Issues))
	}
}

// fleetPage is deliberately one page of plain HTML: the collector's job is to
// be readable from a phone in a car park, and that does not need a framework.
var fleetPage = template.Must(template.New("fleet").Funcs(template.FuncMap{
	"health": Health,
	"since": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return time.Since(t).Round(time.Second).String() + " ago"
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Crossbreeder Sensor fleet</title>
<style>
 :root { color-scheme: dark; }
 body { margin:0; background:#0e1116; color:#dde5ee; font:14px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif; }
 header { padding:12px 16px; background:#151a21; border-bottom:1px solid #262e39; font-weight:600; }
 header span { color:#8794a5; font-weight:400; margin-left:8px; font-size:12px; }
 table { width:100%; border-collapse:collapse; }
 th { text-align:left; color:#8794a5; font-size:11px; text-transform:uppercase; letter-spacing:.5px;
      padding:8px 12px; border-bottom:1px solid #262e39; }
 td { padding:8px 12px; border-bottom:1px solid #1d232c; vertical-align:top; }
 .pill { display:inline-block; padding:1px 8px; border-radius:10px; font-size:11px; font-weight:600; }
 .good { background:#10331f; color:#3ecf8e; } .fair { background:#33280c; color:#f0b429; }
 .poor, .down { background:#3a1e22; color:#ff6b6b; }
 .muted { color:#8794a5; } .issue { color:#ff9c9c; display:block; font-size:12px; }
 .root { color:#4da3ff; }
</style></head><body>
<header>Crossbreeder Sensor fleet <span>{{len .Sensors}} sensor(s) · {{.Version}}</span></header>
<table>
<tr><th>Sensor</th><th>Site</th><th>Health</th><th>Last seen</th><th>Networks</th><th>Open issues</th></tr>
{{range .Sensors}}
<tr>
  <td><b>{{.Name}}</b><br><span class="muted">{{.Address}}</span></td>
  <td>{{.Site}}{{if .Group}} <span class="muted">· {{.Group}}</span>{{end}}</td>
  <td>{{if .Online}}<span class="pill {{health .Overall}}">{{.Overall}}</span>{{else}}<span class="pill down">offline</span>{{end}}</td>
  <td class="muted">{{since .LastSeen}}</td>
  <td class="muted">{{range $name, $r := .Networks}}{{$name}} {{$r.Overall}}<br>{{end}}</td>
  <td>{{range .Issues}}<span class="issue">{{if .RootCause}}<span class="root">root cause</span> {{end}}{{.Network}}: {{.Title}}</span>{{else}}<span class="muted">none</span>{{end}}</td>
</tr>
{{end}}
</table>
</body></html>`))

func (c *Collector) handleFleetPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := fleetPage.Execute(w, map[string]any{"Sensors": c.Fleet(), "Version": c.version}); err != nil {
		c.log("fleet page: %v", err)
	}
}
