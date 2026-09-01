package wifi

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BSS is one radio the sensor can hear. A sensor that reports only its own
// association cannot explain a slow network; the neighbours, their channels
// and how busy the air is are half the answer.
type BSS struct {
	BSSID    string
	SSID     string
	Freq     int
	Channel  int
	Band     string
	Signal   int // dBm
	Flags    string
	Security string
}

// ChannelFor maps a centre frequency to its channel number, across all three
// bands a current AP might use.
func ChannelFor(freq int) int {
	switch {
	case freq == 2484:
		return 14
	case freq >= 2412 && freq <= 2472:
		return (freq - 2407) / 5
	case freq == 5935:
		return 2 // 6 GHz channel 2 sits below the rest of the band
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5
	case freq >= 5160 && freq <= 5885:
		return (freq - 5000) / 5
	}
	return 0
}

// BandFor names the band a frequency belongs to.
func BandFor(freq int) string {
	switch {
	case freq == 0:
		return ""
	case freq < 2500:
		return "2.4 GHz"
	case freq < 5925:
		return "5 GHz"
	case freq < 7200:
		return "6 GHz"
	}
	return ""
}

// SecurityFor reads wpa_supplicant's flag string into the name an operator
// would use for it.
func SecurityFor(flags string) string {
	up := strings.ToUpper(flags)
	switch {
	case strings.Contains(up, "WPA2-EAP") && strings.Contains(up, "SAE"):
		return "WPA3-Enterprise"
	case strings.Contains(up, "WPA3-EAP") || strings.Contains(up, "EAP-SUITE-B"):
		return "WPA3-Enterprise"
	case strings.Contains(up, "SAE"):
		return "WPA3-Personal"
	case strings.Contains(up, "OWE"):
		return "Enhanced Open"
	case strings.Contains(up, "EAP"):
		return "WPA2-Enterprise"
	case strings.Contains(up, "WPA2-PSK") || strings.Contains(up, "RSN-PSK"):
		return "WPA2-Personal"
	case strings.Contains(up, "WPA-PSK"):
		return "WPA-Personal"
	case strings.Contains(up, "WEP"):
		return "WEP"
	case strings.Contains(up, "ESS"):
		return "Open"
	}
	return "Unknown"
}

// Scan triggers a scan and reads the results back. wpa_supplicant refuses a
// scan while it is busy, which is normal rather than an error: the previous
// results are still the truth about what is on the air.
func (c *Ctrl) Scan(ctx context.Context, wait time.Duration) ([]BSS, error) {
	if wait <= 0 {
		wait = 4 * time.Second
	}
	// The event connection belongs to this scan and nothing else: without a
	// scope of its own it would live as long as the caller's context, which
	// for a scheduled pass is the life of the process — one leaked socket per
	// scan, for months.
	ctx, cancel := context.WithTimeout(ctx, wait+5*time.Second)
	defer cancel()
	events, evErr := c.eventsForScan(ctx)
	_, err := c.Request("SCAN")
	if err != nil && !strings.Contains(err.Error(), "FAIL-BUSY") {
		return c.ScanResults()
	}
	if evErr == nil && events != nil {
		deadline := time.After(wait)
	waiting:
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					break waiting
				}
				if strings.Contains(ev, "CTRL-EVENT-SCAN-RESULTS") {
					break waiting
				}
			case <-deadline:
				break waiting
			case <-ctx.Done():
				break waiting
			}
		}
	} else {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
	}
	return c.ScanResults()
}

// eventsForScan attaches a second control connection for the duration of a
// scan, so waiting for the results does not disturb whatever else is using the
// command socket.
func (c *Ctrl) eventsForScan(ctx context.Context) (<-chan string, error) {
	dir, iface := splitCtrlPath(c.path)
	side, err := Dial(dir, iface)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		side.Close()
	}()
	return side.Events(ctx)
}

// splitCtrlPath splits a control socket path back into directory and
// interface, so a second connection can be opened to the same socket.
func splitCtrlPath(path string) (dir, iface string) {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}

// ScanResults reads the current scan table without triggering a new scan.
func (c *Ctrl) ScanResults() ([]BSS, error) {
	reply, err := c.Request("SCAN_RESULTS")
	if err != nil {
		return nil, err
	}
	return parseScanResults(reply), nil
}

func parseScanResults(reply string) []BSS {
	var out []BSS
	for i, line := range strings.Split(reply, "\n") {
		if i == 0 && strings.HasPrefix(line, "bssid") {
			continue // header
		}
		// The SSID is the last field and may contain spaces, so the split is
		// on tabs and limited to five parts.
		f := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 5)
		if len(f) < 4 {
			continue
		}
		freq, _ := strconv.Atoi(f[1])
		signal, _ := strconv.Atoi(f[2])
		b := BSS{
			BSSID: f[0], Freq: freq, Signal: signal, Flags: f[3],
			Channel: ChannelFor(freq), Band: BandFor(freq), Security: SecurityFor(f[3]),
		}
		if len(f) == 5 {
			b.SSID = f[4]
		}
		if b.BSSID == "" {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Signal > out[j].Signal })
	return out
}

// Neighbourhood summarises what the radio can hear, which is what turns "the
// wifi is slow here" into "there are nine APs on channel 1".
type Neighbourhood struct {
	Total       int
	SameSSID    []BSS // other radios carrying our SSID: where we could roam
	CoChannel   int   // other radios sharing our channel
	Overlapping int   // 2.4 GHz radios whose channel partly covers ours
	Strongest   *BSS  // the best roaming candidate
	Channels    map[int]int
}

// Survey reads the scan table from where the sensor is sitting: own is the
// radio it is associated to, and everything is counted relative to that.
func Survey(bsses []BSS, own BSS) Neighbourhood {
	n := Neighbourhood{Total: len(bsses), Channels: map[int]int{}}
	for i := range bsses {
		b := bsses[i]
		n.Channels[b.Channel]++
		if own.BSSID != "" && b.BSSID == own.BSSID {
			continue // ourselves
		}
		if own.SSID != "" && b.SSID == own.SSID {
			// Another radio carrying our SSID: somewhere to roam to.
			n.SameSSID = append(n.SameSSID, b)
			if n.Strongest == nil || b.Signal > n.Strongest.Signal {
				candidate := b
				n.Strongest = &candidate
			}
		}
		if own.Channel == 0 || b.Channel == 0 {
			continue
		}
		switch {
		case b.Channel == own.Channel && b.Band == own.Band:
			n.CoChannel++
		case own.Band == "2.4 GHz" && b.Band == "2.4 GHz" && abs(b.Channel-own.Channel) < 5:
			// 2.4 GHz channels are 5 MHz apart and 20 MHz wide, so a radio
			// within four channels of ours is transmitting over us — which is
			// worse than sharing a channel, because the two cannot hear each
			// other well enough to take turns.
			n.Overlapping++
		}
	}
	return n
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SurveyEntry is one channel's air time, as the driver measured it.
type SurveyEntry struct {
	Freq     int
	Channel  int
	Noise    int
	ActiveMs int64
	BusyMs   int64
	InUse    bool
}

// Utilisation is the share of the time the channel was busy: the number that
// explains a network that tests healthy and still feels slow.
func (s SurveyEntry) Utilisation() float64 {
	if s.ActiveMs <= 0 {
		return 0
	}
	return 100 * float64(s.BusyMs) / float64(s.ActiveMs)
}

// ChannelSurvey asks the driver for per-channel air time. It shells out to iw
// because the netlink interface it wraps has no stable Go binding worth
// carrying, and iw is on every Raspberry Pi OS image.
func ChannelSurvey(ctx context.Context, iface string) ([]SurveyEntry, error) {
	bin, err := exec.LookPath("iw")
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, bin, "dev", iface, "survey", "dump").Output()
	if err != nil {
		return nil, err
	}
	return parseSurveyDump(string(out)), nil
}

func parseSurveyDump(out string) []SurveyEntry {
	var entries []SurveyEntry
	var cur *SurveyEntry
	flush := func() {
		if cur != nil && cur.Freq != 0 {
			cur.Channel = ChannelFor(cur.Freq)
			entries = append(entries, *cur)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Survey data from"):
			flush()
			cur = &SurveyEntry{}
			continue
		case cur == nil:
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		number := func() int64 {
			fields := strings.Fields(value)
			if len(fields) == 0 {
				return 0
			}
			n, _ := strconv.ParseInt(strings.TrimSuffix(fields[0], "MHz"), 10, 64)
			return n
		}
		switch strings.TrimSpace(key) {
		case "frequency":
			cur.Freq = int(number())
			cur.InUse = strings.Contains(value, "in use")
		case "noise":
			cur.Noise = int(number())
		case "channel active time":
			cur.ActiveMs = number()
		case "channel busy time":
			cur.BusyMs = number()
		}
	}
	flush()
	return entries
}
