package wifi

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Profile is a network the sensor is asked to join. It covers what a client
// device in the field would meet: open, WPA2 and WPA3 personal, and 802.1X in
// the methods a campus actually deploys.
type Profile struct {
	SSID   string
	Hidden bool
	// Security is one of open, owe, wep, psk, sae, eap. Left empty it is
	// inferred: a passphrase means psk, an identity means eap, neither means
	// open.
	Security string
	// PSK is the passphrase for psk/sae.
	PSK string
	// WEPKey is a hexadecimal or quoted key for the WEP networks that are
	// still out there on plant equipment.
	WEPKey string

	// 802.1X.
	EAP               string // PEAP, TTLS, TLS, PWD
	Identity          string
	AnonymousIdentity string
	Password          string
	Phase2            string // e.g. auth=MSCHAPV2
	CACert            string // path on the sensor
	ClientCert        string
	PrivateKey        string
	PrivateKeyPasswd  string
	// SubjectMatch pins the RADIUS server certificate. Without it an 802.1X
	// test proves the supplicant will talk to anything, which is not the test
	// anyone wanted.
	SubjectMatch string

	// BSSID pins the association to one radio, which is how a per-AP test or
	// a roaming test is driven.
	BSSID string
	// Freq pins the band or channel, so "the 5 GHz SSID" can be tested
	// separately from the same SSID on 2.4 GHz.
	Freq int
	// PMF: 0 disabled, 1 optional, 2 required. WPA3 requires it.
	PMF int
	// Priority orders profiles when more than one is configured.
	Priority int
}

// Validate reports what is missing before the sensor tries to associate, so
// the failure reads as a configuration error rather than an authentication
// one at three in the morning.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.SSID) == "" {
		return errors.New("the network has no SSID")
	}
	switch p.security() {
	case "psk", "sae":
		if len(p.PSK) < 8 && !isHexKey(p.PSK, 64) {
			return fmt.Errorf("%s: a WPA passphrase is 8 characters or more", p.SSID)
		}
	case "eap":
		if p.EAP == "" {
			return fmt.Errorf("%s: 802.1X needs an EAP method", p.SSID)
		}
		if strings.EqualFold(p.EAP, "TLS") {
			if p.ClientCert == "" || p.PrivateKey == "" {
				return fmt.Errorf("%s: EAP-TLS needs a client certificate and a private key", p.SSID)
			}
		} else if p.Identity == "" || p.Password == "" {
			return fmt.Errorf("%s: %s needs an identity and a password", p.SSID, strings.ToUpper(p.EAP))
		}
	case "wep":
		if p.WEPKey == "" {
			return fmt.Errorf("%s: no WEP key", p.SSID)
		}
	}
	return nil
}

func (p Profile) security() string {
	if s := strings.ToLower(strings.TrimSpace(p.Security)); s != "" {
		return s
	}
	switch {
	case p.Identity != "" || p.ClientCert != "" || p.EAP != "":
		return "eap"
	case p.PSK != "":
		return "psk"
	case p.WEPKey != "":
		return "wep"
	}
	return "open"
}

// Commands renders the profile as the SET_NETWORK calls that configure it.
// Building the list separately from sending it keeps the wire format testable,
// and keeps secrets out of anything but the socket write itself.
func (p Profile) Commands(id int) []string {
	set := func(key, value string) string {
		return fmt.Sprintf("SET_NETWORK %d %s %s", id, key, value)
	}
	quoted := func(key, value string) string { return set(key, quote(value)) }

	cmds := []string{quoted("ssid", p.SSID)}
	if p.Hidden {
		cmds = append(cmds, set("scan_ssid", "1"))
	}
	if p.BSSID != "" {
		cmds = append(cmds, set("bssid", p.BSSID))
	}
	if p.Freq > 0 {
		cmds = append(cmds, set("freq_list", strconv.Itoa(p.Freq)))
	}
	if p.Priority != 0 {
		cmds = append(cmds, set("priority", strconv.Itoa(p.Priority)))
	}

	switch p.security() {
	case "open":
		cmds = append(cmds, set("key_mgmt", "NONE"))
	case "owe":
		cmds = append(cmds, set("key_mgmt", "OWE"), set("ieee80211w", "2"))
	case "wep":
		cmds = append(cmds, set("key_mgmt", "NONE"), set("wep_tx_keyidx", "0"))
		if isHexKey(p.WEPKey, len(p.WEPKey)) {
			cmds = append(cmds, set("wep_key0", p.WEPKey))
		} else {
			cmds = append(cmds, quoted("wep_key0", p.WEPKey))
		}
	case "sae":
		cmds = append(cmds,
			set("key_mgmt", "SAE"),
			set("ieee80211w", "2"), // WPA3 requires protected management frames
			quoted("sae_password", p.PSK))
	case "psk":
		cmds = append(cmds, set("key_mgmt", "WPA-PSK"))
		if isHexKey(p.PSK, 64) {
			cmds = append(cmds, set("psk", p.PSK))
		} else {
			cmds = append(cmds, quoted("psk", p.PSK))
		}
	case "eap":
		cmds = append(cmds,
			set("key_mgmt", "WPA-EAP"),
			set("eap", strings.ToUpper(p.EAP)))
		if p.Identity != "" {
			cmds = append(cmds, quoted("identity", p.Identity))
		}
		if p.AnonymousIdentity != "" {
			cmds = append(cmds, quoted("anonymous_identity", p.AnonymousIdentity))
		}
		if p.Password != "" {
			cmds = append(cmds, quoted("password", p.Password))
		}
		if p.Phase2 != "" {
			cmds = append(cmds, quoted("phase2", p.Phase2))
		}
		if p.CACert != "" {
			cmds = append(cmds, quoted("ca_cert", p.CACert))
		}
		if p.ClientCert != "" {
			cmds = append(cmds, quoted("client_cert", p.ClientCert))
		}
		if p.PrivateKey != "" {
			cmds = append(cmds, quoted("private_key", p.PrivateKey))
		}
		if p.PrivateKeyPasswd != "" {
			cmds = append(cmds, quoted("private_key_passwd", p.PrivateKeyPasswd))
		}
		if p.SubjectMatch != "" {
			cmds = append(cmds, quoted("subject_match", p.SubjectMatch))
		}
	}
	if p.PMF > 0 && p.security() != "sae" && p.security() != "owe" {
		cmds = append(cmds, set("ieee80211w", strconv.Itoa(p.PMF)))
	}
	return cmds
}

// quote wraps a value the way wpa_supplicant expects, and refuses to let a
// quote or a newline out of the value and into the command.
func quote(s string) string {
	s = strings.NewReplacer(`"`, "", "\n", "", "\r", "").Replace(s)
	return `"` + s + `"`
}

func isHexKey(s string, length int) bool {
	if len(s) != length || length == 0 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// Redacted returns the profile with its secrets replaced, for logging and for
// the dashboard. Nothing that reaches a screen or a log file should carry a
// PSK or an 802.1X password.
func (p Profile) Redacted() Profile {
	if p.PSK != "" {
		p.PSK = "********"
	}
	if p.Password != "" {
		p.Password = "********"
	}
	if p.WEPKey != "" {
		p.WEPKey = "********"
	}
	if p.PrivateKeyPasswd != "" {
		p.PrivateKeyPasswd = "********"
	}
	return p
}
