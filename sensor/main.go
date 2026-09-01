// Command crossbreeder-sensor is a network experience sensor: a small box —
// a Raspberry Pi, in the usual case — that sits where the users are, joins the
// networks they join, and keeps testing what they depend on. It reports what
// it finds on its own dashboard, to a collector, to Prometheus, or into
// whatever a site already runs.
//
// It is a companion to Crossbreeder Plus, which works access points; this
// works the other end of the same problem — what the network is actually like
// from the floor.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/andreacoppini/crossbreeder/sensor/netprobe"
	"github.com/andreacoppini/crossbreeder/sensor/wifi"
)

// version is stamped by the release build.
var version = "dev"

func main() {
	opt := parseFlags()
	log := logger()

	switch {
	case opt.showVersion:
		fmt.Println("Crossbreeder Sensor", version)
		return
	case opt.listApps:
		listApps()
		return
	case opt.example:
		printExample()
		return
	case opt.update:
		if err := SelfUpdate(context.Background(), version, log); err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			os.Exit(1)
		}
		return
	case opt.collector:
		if err := runCollector(opt, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	case opt.reflectorOnly != "":
		if err := runReflectorOnly(opt.reflectorOnly, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, path, err := loadOrDefault(opt.config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	applyOverrides(&cfg, opt)
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		os.Exit(1)
	}

	if opt.scan {
		if err := runScan(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(cfg, path, opt, log); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	config        string
	listen        string
	once          bool
	asJSON        bool
	scan          bool
	verbose       bool
	interval      time.Duration
	showVersion   bool
	listApps      bool
	example       bool
	update        bool
	collector     bool
	collectorDir  string
	tokens        string
	adminToken    string
	reflectorOnly string
}

func parseFlags() options {
	var opt options
	flag.StringVar(&opt.config, "config", "", "configuration file (default: the first of $CROSSBREEDER_SENSOR_CONFIG, /etc/crossbreeder-sensor/config.json, ./sensor.json)")
	flag.StringVar(&opt.listen, "listen", "", "address for the dashboard, overriding the configuration")
	flag.BoolVar(&opt.once, "once", false, "run one pass over every network, print the result and exit")
	flag.BoolVar(&opt.asJSON, "json", false, "with -once, write the result as JSON")
	flag.BoolVar(&opt.scan, "scan", false, "scan the air, print what the radio can hear and exit")
	flag.BoolVar(&opt.verbose, "v", false, "log every measurement, not only the summary")
	flag.DurationVar(&opt.interval, "interval", 0, "rest between passes, overriding the configuration")
	flag.BoolVar(&opt.showVersion, "version", false, "print the version and exit")
	flag.BoolVar(&opt.listApps, "apps", false, "list the applications this sensor knows how to test")
	flag.BoolVar(&opt.example, "example", false, "print an example configuration file and exit")
	flag.BoolVar(&opt.update, "update", false, "replace this binary with the latest release and exit")
	flag.BoolVar(&opt.collector, "collector", false, "run as a collector for a fleet of sensors")
	flag.StringVar(&opt.collectorDir, "collector-dir", "", "where a collector keeps the fleet's history")
	flag.StringVar(&opt.tokens, "tokens", "", "collector: sensor tokens, as name=token[,name=token] or *=token for a shared one")
	flag.StringVar(&opt.adminToken, "admin-token", "", "collector: token for the fleet views (empty leaves them open, for a collector behind a proxy)")
	flag.StringVar(&opt.reflectorOnly, "reflector", "", "answer other sensors' voice and throughput tests on this address, and nothing else")
	flag.Parse()
	return opt
}

func logger() func(string, ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	}
}

// loadOrDefault finds the configuration. A sensor with no file yet still
// starts — it tests the wired port it is plugged into — because a box that
// refuses to boot until somebody writes JSON is a box that gets sent back.
func loadOrDefault(explicit string) (Config, string, error) {
	candidates := []string{explicit, os.Getenv("CROSSBREEDER_SENSOR_CONFIG"),
		"/etc/crossbreeder-sensor/config.json", "sensor.json"}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		cfg, err := LoadConfig(path)
		if err == nil {
			return cfg, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return cfg, path, err
		}
		if path == explicit {
			return cfg, path, fmt.Errorf("%s: %w", path, err)
		}
	}
	return DefaultConfig(), "", nil
}

func applyOverrides(cfg *Config, opt options) {
	if opt.listen != "" {
		cfg.Sensor.Listen = opt.listen
	}
	if opt.interval > 0 {
		cfg.Sensor.Interval = Duration(opt.interval)
	}
}

// run is the sensor proper: the loop, the dashboard, the reflector for other
// sensors, and the link to a collector, all until it is asked to stop.
func run(cfg Config, path string, opt options, log func(string, ...any)) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := NewStore(cfg.Storage)
	if err != nil {
		return err
	}
	alerter := NewAlerter(cfg.Alerts, cfg.Sensor.Name, log)
	runner := NewRunner(cfg, DefaultDeps(), log)
	sched := NewScheduler(cfg, runner, store, alerter, log)

	if opt.once {
		results := sched.RunOnce(ctx)
		return report(results, opt.asJSON)
	}

	log("Crossbreeder Sensor %s — %s", version, cfg.Sensor.Name)
	if path != "" {
		log("configuration: %s", path)
	} else {
		log("no configuration file; testing the wired port with the defaults")
	}

	var wg sync.WaitGroup
	serve := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && ctx.Err() == nil {
				log("%s: %v", name, err)
			}
		}()
	}

	// The dashboard.
	api := NewAPI(sched, store, &cfg, path, version, log)
	api.onConfig = func(Config) error {
		// The loop is built from the configuration at start-up, so a change
		// takes effect on restart. Saying so is better than pretending a
		// half-applied configuration is in force.
		log("the configuration was saved; restart the sensor for it to take effect")
		return nil
	}
	server := &http.Server{Addr: cfg.Sensor.Listen, Handler: api.Handler()}
	listener, err := net.Listen("tcp", cfg.Sensor.Listen)
	if err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}
	log("dashboard: http://%s", listener.Addr())
	serve("dashboard", func() error {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	// Answering other sensors' tests, so a pair of sites can measure the path
	// between them without anything else being installed.
	if addr := cfg.Sensor.ReflectorListen; addr != "" {
		conn, err := netprobe.ListenReflector(addr)
		if err != nil {
			return fmt.Errorf("voice reflector: %w", err)
		}
		log("voice reflector: %s", conn.LocalAddr())
		serve("voice reflector", func() error { return netprobe.Reflect(ctx, conn) })
	}
	if addr := cfg.Sensor.ThroughputListen; addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("throughput peer: %w", err)
		}
		log("throughput peer: %s", ln.Addr())
		serve("throughput peer", func() error { return netprobe.ServeThroughput(ctx, ln) })
	}

	// The link to a collector.
	if cfg.Upstream.URL != "" {
		uplink := NewUplink(cfg.Upstream, cfg.Sensor, version, store, sched, log)
		uplink.OnConfig = func(incoming Config) error {
			if path == "" {
				return errors.New("this sensor has no configuration file to write")
			}
			if err := incoming.Save(path); err != nil {
				return err
			}
			log("the new configuration is saved; restarting to take it up")
			go func() { time.Sleep(2 * time.Second); stop() }()
			return nil
		}
		uplink.OnUpdate = func(ctx context.Context) error { return SelfUpdate(ctx, version, log) }
		uplink.OnRestart = func() { go func() { time.Sleep(time.Second); stop() }() }
		log("reporting to %s every %s", cfg.Upstream.URL, cfg.Upstream.Every.D())
		serve("collector link", func() error { uplink.Run(ctx); return nil })
	}

	// Housekeeping: the history is pruned daily rather than at start-up only,
	// since a sensor runs for months at a time.
	serve("history", func() error {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := store.Prune(); err != nil {
					log("pruning the history: %v", err)
				}
			}
		}
	})

	sched.Run(ctx)

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdown)
	wg.Wait()
	log("stopped")
	return nil
}

// report prints the result of a one-off run, in the shape somebody running it
// over SSH wants: the failures first, then everything.
func report(results []SuiteResult, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	failed := false
	for _, r := range results {
		fmt.Printf("\n%s (%s) — health %d/100, %s, %s\n", r.Network, r.Kind, r.Overall,
			Health(r.Overall), r.Duration.Round(time.Millisecond))
		if r.Radio != nil {
			fmt.Printf("  %s on channel %d (%s), %d dBm", r.Radio.BSSID, r.Radio.Channel, r.Radio.Band, r.Radio.RSSI)
			if r.Radio.SNR != 0 {
				fmt.Printf(", SNR %d dB", r.Radio.SNR)
			}
			fmt.Println()
		}
		if r.Lease != nil {
			fmt.Printf("  %s from %s, gateway %s, resolvers %s\n",
				r.Lease.Address, r.Lease.Server, r.Lease.Router, strings.Join(r.Lease.DNS, ", "))
		}
		if r.Neighbour != "" {
			fmt.Printf("  %s\n", r.Neighbour)
		}
		fmt.Println()
		for _, m := range r.Measurements {
			fmt.Printf("  %s\n", m)
		}
		if len(r.Issues) > 0 {
			fmt.Println()
			for _, issue := range r.Issues {
				fmt.Printf("  %s\n", issue)
			}
		}
		if r.Status() == StatusFail {
			failed = true
		}
	}
	if failed {
		// A one-off run is the thing a commissioning script calls, so the
		// exit status has to mean something.
		os.Exit(2)
	}
	return nil
}

func runScan(cfg Config) error {
	iface := cfg.Sensor.MonitorInterface
	if iface == "" {
		iface = cfg.Sensor.WirelessInterface
	}
	ctrl, err := wifi.Dial(cfg.Sensor.CtrlDir, iface)
	if err != nil {
		return err
	}
	defer ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bsses, err := ctrl.Scan(ctx, 5*time.Second)
	if err != nil {
		return err
	}
	if survey, err := wifi.ChannelSurvey(ctx, iface); err == nil {
		for _, s := range survey {
			if s.InUse {
				fmt.Printf("channel %d: %.0f%% of the air time in use, noise %d dBm\n",
					s.Channel, s.Utilisation(), s.Noise)
			}
		}
		fmt.Println()
	}
	fmt.Printf("%-5s %-4s %-8s %-18s %-18s %s\n", "dBm", "ch", "band", "security", "bssid", "ssid")
	for _, b := range bsses {
		ssid := b.SSID
		if ssid == "" {
			ssid = "(hidden)"
		}
		fmt.Printf("%-5d %-4d %-8s %-18s %-18s %s\n", b.Signal, b.Channel, b.Band, b.Security, b.BSSID, ssid)
	}
	return nil
}

func runReflectorOnly(addr string, log func(string, ...any)) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := netprobe.ListenReflector(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	log("answering voice tests on %s", conn.LocalAddr())

	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err == nil {
			defer ln.Close()
			log("answering throughput tests on %s", ln.Addr())
			go netprobe.ServeThroughput(ctx, ln)
		}
	}
	return netprobe.Reflect(ctx, conn)
}

func runCollector(opt options, log func(string, ...any)) error {
	tokens, err := parseTokens(opt.tokens)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return errors.New("a collector needs -tokens: name=token, or *=token for a whole fleet")
	}
	listen := opt.listen
	if listen == "" {
		listen = "127.0.0.1:52415"
	}
	dir := opt.collectorDir
	if dir == "" {
		dir = "fleet"
	}
	storage := Storage{Dir: dir, Keep: Duration(90 * 24 * time.Hour), MaxMiB: 8192}
	collector := NewCollector(dir, tokens, opt.adminToken, version, storage, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// A collector answers the fleet's voice and throughput tests too, so a
	// site can measure the path to it without another server.
	if conn, err := netprobe.ListenReflector(reflectorAddrFor(listen)); err == nil {
		defer conn.Close()
		log("voice reflector: %s", conn.LocalAddr())
		go netprobe.Reflect(ctx, conn)
	}

	server := &http.Server{Addr: listen, Handler: collector.Handler()}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log("collector: http://%s (%d token(s))", listener.Addr(), len(tokens))
	if opt.adminToken == "" {
		log("the fleet views are open: bind to localhost or put a proxy in front, or set -admin-token")
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// reflectorAddrFor puts the reflector on the same host as the collector, one
// port along, so a fleet needs one address to be told about rather than two.
func reflectorAddrFor(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return ":52416"
	}
	n := 52416
	if parsed, err := net.LookupPort("udp", port); err == nil {
		n = parsed + 1
	}
	return net.JoinHostPort(host, fmt.Sprint(n))
}

func parseTokens(spec string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, token, ok := strings.Cut(pair, "=")
		if !ok || token == "" {
			return nil, fmt.Errorf("%q is not name=token", pair)
		}
		if len(token) < 16 {
			return nil, fmt.Errorf("the token for %s is too short to be worth having", name)
		}
		out[token] = strings.TrimSpace(name)
	}
	return out, nil
}

func listApps() {
	byCategory := AppsByCategory()
	categories := make([]string, 0, len(byCategory))
	for c := range byCategory {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	for _, category := range categories {
		fmt.Printf("%s\n", category)
		for _, name := range byCategory[category] {
			fmt.Printf("  %s\n", name)
		}
	}
}

func printExample() {
	yes := true
	cfg := DefaultConfig()
	cfg.Sensor.Site = "Head office"
	cfg.Sensor.Group = "Ground floor"
	cfg.Sensor.ReflectorListen = ":52416"
	cfg.Networks = append(cfg.Networks, Network{
		Name: "Corporate", Kind: "wifi",
		Profile: wifi.Profile{
			SSID: "Campus-Secure", EAP: "PEAP", Identity: "sensor@example.com",
			Password: "the RADIUS password", Phase2: "auth=MSCHAPV2",
			CACert:       "/etc/crossbreeder-sensor/radius-ca.pem",
			SubjectMatch: "CN=radius.example.com",
		},
		Tests: TestPlan{
			DHCP: &yes, Gateway: &yes, CaptivePortal: &yes, Roaming: &yes,
			DNS: []DNSTarget{
				{Query: "intranet.example.com", Expect: "10.20.0.40"},
				{Query: "outlook.office365.com"},
			},
			Internet:   []string{"1.1.1.1", "8.8.8.8"},
			Apps:       []string{"Microsoft 365", "Zoom", "Salesforce"},
			Web:        []WebTarget{{Name: "Intranet", URL: "https://intranet.example.com/health", ExpectStatus: 200}},
			Ports:      []PortTarget{{Name: "File server", Address: "files.example.com:445"}},
			Traceroute: []string{"outlook.office365.com"},
			VoIP:       &VoIPTarget{Reflector: "collector.example.com:52416", DSCP: 46, Codec: "G.711"},
			Throughput: &ThroughputTarget{
				Mode: "peer", Peer: "collector.example.com:52415",
				Every: Duration(6 * time.Hour), ExpectMbps: 100,
			},
			Certificates: []string{"intranet.example.com:443"},
		},
	})
	cfg.Networks = append(cfg.Networks, Network{
		Name: "Guest", Kind: "wifi",
		Profile: wifi.Profile{SSID: "Campus-Guest"},
		Tests: TestPlan{
			DHCP: &yes, Gateway: &yes, CaptivePortal: &yes,
			DNS:      []DNSTarget{{Query: "www.google.com"}},
			Internet: []string{"1.1.1.1"},
			Apps:     []string{"Internet"},
		},
	})
	cfg.Alerts = AlertConfig{
		Enabled: true, MinSeverity: "warning", Repeat: Duration(time.Hour),
		Webhooks: []string{"https://example.com/hooks/network"},
	}
	cfg.Upstream = Upstream{
		URL: "https://collector.example.com", Token: "a-long-shared-secret",
		Every: Duration(time.Minute), AcceptCfg: true,
	}
	cfg.Storage = Storage{Dir: "/var/lib/crossbreeder-sensor", Keep: Duration(14 * 24 * time.Hour), MaxMiB: 512}

	blob, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Println(string(blob))
}
