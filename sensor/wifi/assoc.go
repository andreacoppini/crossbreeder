package wifi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Association is one attempt to get onto a network, broken into the steps a
// client goes through. "It took twelve seconds to connect" is a complaint;
// "the EAP exchange took eleven of them" is a RADIUS ticket.
type Association struct {
	SSID     string
	BSSID    string
	Freq     int
	Channel  int
	Band     string
	KeyMgmt  string
	Security string
	Signal   Signal

	Scan  time.Duration // scan started to scan results
	Auth  time.Duration // association started to associated
	EAP   time.Duration // 802.1X exchange, zero on a personal network
	Key   time.Duration // four-way handshake
	Total time.Duration

	Failure string // a short classification when Err is set
	Err     error
}

// OK reports whether the sensor got onto the network.
func (a Association) OK() bool { return a.Err == nil && a.BSSID != "" }

// Connect joins a network and times each step. The radio is left associated:
// everything after this — DHCP, DNS, the applications — runs over it.
func (c *Ctrl) Connect(ctx context.Context, p Profile, timeout time.Duration) Association {
	res := Association{SSID: p.SSID}
	if err := p.Validate(); err != nil {
		res.Err, res.Failure = err, "misconfigured"
		return res
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	events, err := c.eventsForScan(ctx) // a second, attached connection
	if err != nil {
		res.Err, res.Failure = err, "control interface"
		return res
	}

	// The sensor owns the radio, so it starts from a clean slate every time:
	// a profile left behind by the last test would be a network the supplicant
	// could silently prefer.
	c.Request("REMOVE_NETWORK all")

	idReply, err := c.Request("ADD_NETWORK")
	if err != nil {
		res.Err, res.Failure = err, "control interface"
		return res
	}
	id, err := strconv.Atoi(strings.TrimSpace(idReply))
	if err != nil {
		res.Err, res.Failure = fmt.Errorf("wpa_supplicant answered %q to ADD_NETWORK", idReply), "control interface"
		return res
	}
	for _, cmd := range p.Commands(id) {
		if _, err := c.Request(cmd); err != nil {
			// The command is named in the error, but never its value: these
			// carry passphrases.
			res.Err, res.Failure = err, "misconfigured"
			return res
		}
	}

	start := time.Now()
	if _, err := c.Request(fmt.Sprintf("SELECT_NETWORK %d", id)); err != nil {
		res.Err, res.Failure = err, "control interface"
		return res
	}

	var scanStart, assocStart, eapStart, associated time.Time
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				res.Err, res.Failure = errors.New("the control connection closed mid-association"), "control interface"
				return res
			}
			now := time.Now()
			switch {
			case strings.Contains(ev, "CTRL-EVENT-SCAN-STARTED"):
				if scanStart.IsZero() {
					scanStart = now
				}
			case strings.Contains(ev, "CTRL-EVENT-SCAN-RESULTS"):
				if !scanStart.IsZero() && res.Scan == 0 {
					res.Scan = now.Sub(scanStart)
				}
			case strings.Contains(ev, "Trying to associate"), strings.Contains(ev, "SME: Trying to authenticate"),
				strings.Contains(ev, "CTRL-EVENT-ASSOC-START"):
				if assocStart.IsZero() {
					assocStart = now
				}
			case strings.Contains(ev, "Associated with"):
				associated = now
				if !assocStart.IsZero() {
					res.Auth = now.Sub(assocStart)
				}
				res.BSSID = lastField(ev)
			case strings.Contains(ev, "CTRL-EVENT-EAP-STARTED"):
				eapStart = now
			case strings.Contains(ev, "CTRL-EVENT-EAP-SUCCESS"):
				if !eapStart.IsZero() {
					res.EAP = now.Sub(eapStart)
				}
			case strings.Contains(ev, "WPA: Key negotiation completed"):
				from := associated
				if !eapStart.IsZero() && eapStart.After(from) {
					from = eapStart.Add(res.EAP)
				}
				if !from.IsZero() {
					res.Key = now.Sub(from)
				}
			case strings.Contains(ev, "CTRL-EVENT-CONNECTED"):
				res.Total = now.Sub(start)
				c.fillAssociation(&res)
				return res
			case isFailureEvent(ev):
				res.Total = now.Sub(start)
				res.Failure = classifyFailure(ev)
				res.Err = fmt.Errorf("could not join %s: %s", p.SSID, res.Failure)
				return res
			}
		case <-ctx.Done():
			res.Total = time.Since(start)
			res.Failure = "timed out"
			// A supplicant that never says why is still worth asking: its
			// state names the step it is stuck on.
			if st, err := c.Status(); err == nil && st.State != "" {
				res.Failure = "timed out in " + strings.ToLower(st.State)
			}
			res.Err = fmt.Errorf("could not join %s within %v: %s", p.SSID, timeout, res.Failure)
			return res
		}
	}
}

// fillAssociation reads what the sensor ended up connected to.
func (c *Ctrl) fillAssociation(a *Association) {
	if st, err := c.Status(); err == nil {
		if st.BSSID != "" {
			a.BSSID = st.BSSID
		}
		if st.SSID != "" {
			a.SSID = st.SSID
		}
		a.Freq, a.Channel, a.Band, a.KeyMgmt = st.Freq, st.Channel, st.Band, st.KeyMgmt
		a.Security = SecurityFor(st.KeyMgmt)
	}
	if sig, err := c.SignalPoll(); err == nil {
		a.Signal = sig
		if a.Freq == 0 {
			a.Freq, a.Channel = sig.Freq, sig.Channel
		}
	}
}

func isFailureEvent(ev string) bool {
	for _, marker := range []string{
		"CTRL-EVENT-ASSOC-REJECT", "CTRL-EVENT-AUTH-REJECT", "CTRL-EVENT-EAP-FAILURE",
		"CTRL-EVENT-NETWORK-NOT-FOUND", "CTRL-EVENT-SSID-TEMP-DISABLED",
		"CTRL-EVENT-DISCONNECTED", "CTRL-EVENT-ASSOC-TIMED-OUT",
	} {
		if strings.Contains(ev, marker) {
			return true
		}
	}
	return false
}

// classifyFailure turns a supplicant event into the sentence an operator
// needs. The distinction that matters most is between a network that is not
// there, a passphrase that is wrong and a RADIUS server that said no — three
// completely different tickets that all read as "wifi not working".
func classifyFailure(ev string) string {
	switch {
	case strings.Contains(ev, "CTRL-EVENT-NETWORK-NOT-FOUND"):
		return "the SSID was not on the air"
	case strings.Contains(ev, "WRONG_KEY"):
		return "the passphrase was rejected"
	case strings.Contains(ev, "CTRL-EVENT-EAP-FAILURE"):
		return "802.1X authentication failed"
	case strings.Contains(ev, "CTRL-EVENT-AUTH-REJECT"):
		return "the AP rejected authentication" + statusSuffix(ev, "status_code=")
	case strings.Contains(ev, "CTRL-EVENT-ASSOC-REJECT"):
		return "the AP rejected the association" + statusSuffix(ev, "status_code=")
	case strings.Contains(ev, "CTRL-EVENT-ASSOC-TIMED-OUT"):
		return "the association timed out"
	case strings.Contains(ev, "CTRL-EVENT-SSID-TEMP-DISABLED"):
		return "the supplicant disabled the network after repeated failures"
	case strings.Contains(ev, "CTRL-EVENT-DISCONNECTED"):
		return "disconnected during the exchange" + statusSuffix(ev, "reason=")
	}
	return "association failed"
}

// statusSuffix pulls a numeric code out of an event so the AP's own reason
// travels with the classification. 802.11 status 17 is a full AP, and a site
// where that is the failure has a capacity problem, not a wireless one.
func statusSuffix(ev, key string) string {
	i := strings.Index(ev, key)
	if i < 0 {
		return ""
	}
	value := strings.Fields(ev[i+len(key):])
	if len(value) == 0 {
		return ""
	}
	code := strings.TrimSpace(value[0])
	if code == "" || code == "0" {
		return ""
	}
	if name := statusCodeNames[code]; name != "" {
		return fmt.Sprintf(" (%s, code %s)", name, code)
	}
	return " (code " + code + ")"
}

// The 802.11 status and reason codes a sensor meets in the field.
var statusCodeNames = map[string]string{
	"1":  "unspecified failure",
	"12": "association denied, no bandwidth",
	"15": "four-way handshake timeout",
	"17": "the AP is at its association limit",
	"23": "802.1X authentication failed",
	"24": "cipher rejected by policy",
	"53": "invalid PMKID",
}

func lastField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// Roam moves the association to another radio of the same SSID and times the
// handover. This is the test that catches a site where roaming works but takes
// long enough to drop a call.
func (c *Ctrl) Roam(ctx context.Context, bssid string, timeout time.Duration) (time.Duration, string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	events, err := c.eventsForScan(ctx)
	if err != nil {
		return 0, "", err
	}
	start := time.Now()
	if _, err := c.Request("ROAM " + bssid); err != nil {
		return 0, "", err
	}
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return 0, "", errors.New("the control connection closed mid-roam")
			}
			if strings.Contains(ev, "CTRL-EVENT-CONNECTED") {
				took := time.Since(start)
				landed := bssid
				if st, err := c.Status(); err == nil && st.BSSID != "" {
					landed = st.BSSID
				}
				return took, landed, nil
			}
			if isFailureEvent(ev) && !strings.Contains(ev, "CTRL-EVENT-DISCONNECTED") {
				return time.Since(start), "", errors.New(classifyFailure(ev))
			}
		case <-ctx.Done():
			return time.Since(start), "", fmt.Errorf("the roam to %s did not complete within %v", bssid, timeout)
		}
	}
}

// Disconnect drops the association and clears the profile, so a sensor that
// has finished with one SSID does not sit on it.
func (c *Ctrl) Disconnect() error {
	if _, err := c.Request("DISCONNECT"); err != nil {
		return err
	}
	_, err := c.Request("REMOVE_NETWORK all")
	return err
}
