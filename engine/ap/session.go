// Package ap drives the Ruckus standalone-AP CLI over SSH.
//
// One Session is one AP. Sessions share nothing, so the caller is free to run
// as many as it likes concurrently; see Runner in the parent package.
package ap

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Kind is the CLI flavour found on the far end.
type Kind string

const (
	KindZoneFlex  Kind = "zoneflex"  // rkscli
	KindUnleashed Kind = "unleashed" // Unleashed / SmartZone-style shell
	KindUnknown   Kind = "unknown"
)

// Actions is what the operator ticked in the UI.
type Actions struct {
	UpdateFirmware bool
	FactoryReset   bool
	CustomCommand  string
	Reboot         bool
}

// Any reports whether anything beyond inventory collection was requested.
func (a Actions) Any() bool {
	return a.UpdateFirmware || a.FactoryReset || a.CustomCommand != "" || a.Reboot
}

// Firmware describes where the AP should pull its image from.
type Firmware struct {
	Proto    string // http, ftp, tftp
	Host     string
	Port     string
	User     string
	Password string
	// Filename may contain %M, replaced with the detected model.
	Filename string
}

// Credentials are tried in order. The AP CLI login is separate from the SSH
// transport login, and both are attempted with the same pair.
type Credentials struct {
	User     string
	Password string
}

// Config is everything a Session needs. It is read-only and shared by every
// worker, which is what makes the fan-out safe.
type Config struct {
	Credentials []Credentials
	Actions     Actions
	Firmware    Firmware
	Port        string
	// ConnectTimeout bounds the TCP + SSH handshake, DialogTimeout bounds any
	// single expect, and Deadline bounds the whole per-AP session.
	ConnectTimeout time.Duration
	DialogTimeout  time.Duration
	Deadline       time.Duration
	// LegacyAlgorithms re-enables the SHA-1 / CBC primitives that pre-2015
	// ZoneFlex firmware negotiates and modern SSH stacks refuse by default.
	LegacyAlgorithms bool
}

// Result is one row of the output table.
type Result struct {
	IP         string        `json:"ip"`
	Reachable  bool          `json:"reachable"`
	PingMS     float64       `json:"ping_ms"`
	MAC        string        `json:"mac,omitempty"`
	Model      string        `json:"model,omitempty"`
	Firmware   string        `json:"firmware,omitempty"`
	Kind       Kind          `json:"kind,omitempty"`
	Status     string        `json:"status"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"duration_ms"`
	Transcript string        `json:"-"`
}

// dialect folds the ZoneFlex and Unleashed code paths — which in the Xojo
// original are two near-identical 90-line blocks, duplicated again in a second
// class — into a description of what differs: the prompt and the preamble.
type dialect struct {
	kind   Kind
	prompt string
	// enter runs once after login to reach the mode where fw commands work.
	enter []step
	// info gathers inventory; parse pulls the fields out of the transcript.
	info  []string
	parse func(transcript string, r *Result)
}

type step struct {
	send   string
	expect string
}

var zoneFlex = dialect{
	kind:   KindZoneFlex,
	prompt: "rkscli: ",
	info:   []string{"get version", "get boarddata"},
	parse: func(t string, r *Result) {
		r.Model = between(t, "Ruckus ", " Multimedia Hotzone Wireless AP")
		r.Firmware = afterMarker(t, "Version: ")
		r.MAC = normalizeMAC(afterMarker(t, ", base "))
	},
}

var unleashed = dialect{
	kind:   KindUnleashed,
	prompt: "(ap-mode)# ",
	enter: []step{
		{send: "enable force", expect: "# "},
	},
	info: []string{"show sysinfo"},
	parse: func(t string, r *Result) {
		r.Model = afterMarker(t, "Model= ")
		r.Firmware = strings.ReplaceAll(afterMarker(t, "Version= "), " Build ", ".")
		r.MAC = normalizeMAC(afterMarker(t, "MAC Address= "))
	},
}

// Run drives one AP to completion. It never touches shared state, so N of these
// may be in flight at once.
func Run(ctx context.Context, host string, cfg Config) Result {
	start := time.Now()
	r := Result{IP: host, Status: "Skipped"}

	ctx, cancel := context.WithTimeout(ctx, cfg.Deadline)
	defer cancel()

	defer func() {
		r.Duration = time.Since(start)
		r.DurationMS = r.Duration.Milliseconds()
	}()

	// Reachability is settled by the sweep before we get here (see Sweep in
	// ping.go); this is the address list already filtered down to what answered.
	r.Reachable = true
	if err := run(ctx, host, cfg, &r); err != nil {
		if r.Status == "Skipped" {
			r.Status = "Error"
		}
		r.Error = err.Error()
		return r
	}
	r.Status = "Done"
	return r
}

func run(ctx context.Context, host string, cfg Config, r *Result) error {
	client, err := dial(ctx, host, cfg)
	if err != nil {
		r.Status = "SSH Failed"
		return err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		r.Status = "SSH Failed"
		return err
	}
	defer sess.Close()

	if err := sess.RequestPty("dumb", 40, 120, ssh.TerminalModes{}); err != nil {
		r.Status = "SSH Failed"
		return err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	sess.Stderr = nil
	if err := sess.Shell(); err != nil {
		r.Status = "SSH Failed"
		return err
	}

	e := newExpecter(stdin, stdout, cfg.DialogTimeout)
	defer func() { r.Transcript = e.Transcript() }()

	d, err := login(e, cfg.Credentials)
	if err != nil {
		r.Status = "Login Failed"
		return err
	}
	r.Kind = d.kind

	for _, s := range d.enter {
		if err := exchange(e, s.send, s.expect); err != nil {
			return err
		}
	}
	// Unleashed needs one more hop to reach ap-mode, and that hop is also where
	// the prompt changes; ZoneFlex is already there.
	if d.kind == KindUnleashed {
		var info string
		for _, cmd := range d.info {
			out, err := exchangeOut(e, cmd, "# ")
			if err != nil {
				return err
			}
			info += out
		}
		d.parse(info, r)
		if err := exchange(e, "ap-mode", d.prompt); err != nil {
			return err
		}
	} else {
		var info string
		for _, cmd := range d.info {
			out, err := exchangeOut(e, cmd, d.prompt)
			if err != nil {
				return err
			}
			info += out
		}
		d.parse(info, r)
	}

	if !cfg.Actions.Any() {
		return nil // inventory-only run
	}

	if cfg.Actions.UpdateFirmware {
		for _, cmd := range firmwareCommands(cfg.Firmware, r.Model) {
			if err := exchange(e, cmd, d.prompt); err != nil {
				return err
			}
		}
	}
	if cfg.Actions.CustomCommand != "" {
		if err := exchange(e, cfg.Actions.CustomCommand, d.prompt); err != nil {
			return err
		}
	}
	// Factory reset and reboot are terminal: they drop the session, so they run
	// last and a lost prompt is expected rather than an error. The Xojo original
	// issued "set factory" *before* the firmware commands, which on a real AP
	// discards everything sent after it.
	if cfg.Actions.FactoryReset {
		terminal(e, "set factory", d.prompt)
	}
	if cfg.Actions.Reboot {
		terminal(e, "reboot", d.prompt)
	}
	return nil
}

func firmwareCommands(fw Firmware, model string) []string {
	filename := strings.ReplaceAll(fw.Filename, "%M", model)
	return []string{
		"fw auto disable",
		"fw set proto " + fw.Proto,
		"fw set port " + fw.Port,
		"fw set control " + filename,
		"fw set host " + fw.Host,
		"fw set user " + fw.User,
		"fw set password " + fw.Password,
		"fw auto enable",
		"fw update",
	}
}

func dial(ctx context.Context, host string, cfg Config) (*ssh.Client, error) {
	first := Credentials{}
	if len(cfg.Credentials) > 0 {
		first = cfg.Credentials[0]
	}
	conf := &ssh.ClientConfig{
		User: first.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(first.Password),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				answers := make([]string, len(qs))
				for i := range qs {
					answers[i] = first.Password
				}
				return answers, nil
			}),
		},
		// Field APs mid-reset have no stable host key and are reached by IP, so
		// there is nothing to pin against. The fingerprint is recorded in the
		// transcript instead. The Chilkat original did not verify either.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.ConnectTimeout,
	}
	if cfg.LegacyAlgorithms {
		sup, ins := ssh.SupportedAlgorithms(), ssh.InsecureAlgorithms()
		conf.KeyExchanges = append(sup.KeyExchanges, ins.KeyExchanges...)
		conf.Ciphers = append(sup.Ciphers, ins.Ciphers...)
		conf.MACs = append(sup.MACs, ins.MACs...)
		conf.HostKeyAlgorithms = append(sup.HostKeys, ins.HostKeys...)
	}

	d := net.Dialer{Timeout: cfg.ConnectTimeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, cfg.Port))
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(host, cfg.Port), conf)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// login handles the AP's own CLI login, which sits behind the SSH login, and
// falls back through the remaining credential pairs (typically super/sp-admin
// on a factory-default AP).
//
// It waits for every possible next thing at once rather than assuming an order.
// Some builds print a login banner, some drop straight to a prompt because the
// SSH transport login was the only one, and some answer a rejected password
// with a fresh banner; guessing wrong used to cost a full timeout and then lose
// the prompt that had already arrived.
func login(e *expecter, creds []Credentials) (dialect, error) {
	if len(creds) == 0 {
		return dialect{}, fmt.Errorf("no credentials supplied")
	}

	const (
		wantLogin = iota
		wantUser
		wantPassword
		wantIncorrect
		wantDenied
		promptZF
		promptULEnable
		promptUL
	)
	pats := []pat{
		anywhere("ogin:"),
		anywhere("sername:"),
		anywhere("assword"),
		anywhere("Login incorrect"),
		anywhere("Permission denied"),
		atEnd("rkscli: "),
		atEnd("(ap-mode)# "),
		atEnd("> "),
	}

	cred := 0
	// Bounded so a device that loops its banner cannot spin here forever.
	for step := 0; step < 4*len(creds)+8; step++ {
		i, _, err := e.ExpectPats(pats...)
		if err != nil {
			return dialect{}, fmt.Errorf("%w; last seen: %s", err, tail(e.Pending(), 160))
		}

		switch i {
		case wantLogin, wantUser:
			if err := e.Send(creds[cred].User); err != nil {
				return dialect{}, err
			}
		case wantPassword:
			if err := e.Send(creds[cred].Password); err != nil {
				return dialect{}, err
			}
		case wantIncorrect, wantDenied:
			cred++
			if cred >= len(creds) {
				return dialect{}, fmt.Errorf("AP rejected %s", credSummary(creds))
			}
		case promptZF:
			return zoneFlex, nil
		case promptULEnable, promptUL:
			return unleashed, nil
		}
	}
	return dialect{}, fmt.Errorf("could not reach a CLI prompt; last seen: %s", tail(e.Pending(), 160))
}

func credSummary(creds []Credentials) string {
	names := make([]string, len(creds))
	for i, c := range creds {
		names[i] = c.User
	}
	if len(names) == 1 {
		return fmt.Sprintf("user %q", names[0])
	}
	return fmt.Sprintf("users %s", strings.Join(names, ", "))
}

// tail returns the last n bytes of s with whitespace collapsed, so a failure
// reason fits on one line of output next to the AP it came from.
func tail(s string, n int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) > n {
		s = "..." + s[len(s)-n:]
	}
	if s == "" {
		return "(nothing)"
	}
	return s
}

func exchange(e *expecter, send, want string) error {
	_, err := exchangeOut(e, send, want)
	return err
}

func exchangeOut(e *expecter, send, want string) (string, error) {
	if err := e.Send(send); err != nil {
		return "", err
	}
	_, out, err := e.Expect(want)
	if err != nil {
		return out, fmt.Errorf("after %q: %w", send, err)
	}
	return out, nil
}

// terminal sends a command that is expected to tear the session down.
func terminal(e *expecter, send, want string) {
	_ = e.Send(send)
	_, _, _ = e.Expect(want)
}

func between(s, prefix, suffix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	j := strings.Index(rest, suffix)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

// afterMarker returns the remainder of the line following marker. The original
// built a regular expression out of the marker for this, which broke on any
// device output containing regex metacharacters.
func afterMarker(s, marker string) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	if j := strings.IndexAny(rest, "\r\n"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

func normalizeMAC(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
