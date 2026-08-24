package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// ipCandidate is one local address the firmware server could advertise.
type ipCandidate struct {
	ip      net.IP
	network *net.IPNet
	iface   string
	ifaceNo int

	covered   int  // how many target APs sit inside this address's own subnet
	private   bool // RFC 1918, which notably excludes Tailscale's 100.64/10
	routePick bool // what the routing table would have chosen on its own
}

func (c ipCandidate) String() string {
	kind := "other"
	switch {
	case c.covered > 0:
		kind = fmt.Sprintf("covers %d AP(s)", c.covered)
	case c.private:
		kind = "RFC1918"
	}
	if c.routePick {
		kind += ", default route"
	}
	ones, _ := c.network.Mask.Size()
	return fmt.Sprintf("%-15s /%-2d %-20s %s", c.ip, ones, c.iface, kind)
}

// chooseServeIP picks which of this machine's addresses the APs should be told
// to fetch firmware from.
//
// Asking the routing table how it would reach one AP is the obvious approach
// and the wrong one: with a VPN up it happily answers with the VPN's address,
// which the APs cannot reach. Preferring an address that is *on the same
// subnet* as the APs themselves is both more reliable and easier to explain.
//
// The order is: the subnet holding the most APs, then any subnet holding an AP,
// then RFC 1918, then anything else.
func chooseServeIP(targets []string) (ip string, reason string, considered []string, err error) {
	cands, err := localCandidates()
	if err != nil {
		return "", "", nil, err
	}
	if len(cands) == 0 {
		return "", "", nil, fmt.Errorf("no usable local IPv4 address found; pass -serve-ip")
	}

	ips := make([]net.IP, 0, len(targets))
	for _, t := range targets {
		if v4 := net.ParseIP(t).To4(); v4 != nil {
			ips = append(ips, v4)
		}
	}

	var routeIP string
	if len(targets) > 0 {
		routeIP = localIPFor(targets[0])
	}

	for i := range cands {
		for _, t := range ips {
			if cands[i].network.Contains(t) {
				cands[i].covered++
			}
		}
		cands[i].private = cands[i].ip.IsPrivate()
		cands[i].routePick = cands[i].ip.String() == routeIP
	}

	sortCandidates(cands)

	best := cands[0]
	for _, c := range cands {
		considered = append(considered, c.String())
	}

	ones, _ := best.network.Mask.Size()
	switch {
	case best.covered > 0:
		reason = fmt.Sprintf("%s/%d on %s covers %d of %d AP(s)",
			best.ip, ones, best.iface, best.covered, len(ips))
	case best.private:
		reason = fmt.Sprintf("%s on %s (no interface shares a subnet with the APs; picked the RFC1918 address)",
			best.ip, best.iface)
	default:
		reason = fmt.Sprintf("%s on %s (nothing better available)", best.ip, best.iface)
	}
	return best.ip.String(), reason, considered, nil
}

// parseIP4 returns the IPv4 form of s, or nil.
func parseIP4(s string) net.IP { return net.ParseIP(s).To4() }

// sortCandidates puts the best choice first.
func sortCandidates(c []ipCandidate) {
	sort.SliceStable(c, func(i, j int) bool {
		a, b := c[i], c[j]
		// 1 & 2. Most APs on this address's subnet. A subnet holding any AP
		// beats one holding none, and the busiest beats the rest.
		if a.covered != b.covered {
			return a.covered > b.covered
		}
		// 3. RFC 1918 before anything else.
		if a.private != b.private {
			return a.private
		}
		// Within a tier, defer to what the routing table would have done.
		if a.routePick != b.routePick {
			return a.routePick
		}
		// Deterministic from here, so repeated runs pick the same address.
		if a.ifaceNo != b.ifaceNo {
			return a.ifaceNo < b.ifaceNo
		}
		return bytesLess(a.ip, b.ip)
	})
}

func bytesLess(a, b net.IP) bool { return strings.Compare(a.String(), b.String()) < 0 }

// localCandidates lists this machine's usable IPv4 addresses.
func localCandidates() ([]ipCandidate, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []ipCandidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := n.IP.To4()
			// APs are IPv4, and a 169.254 address means DHCP failed.
			if v4 == nil || v4.IsLinkLocalUnicast() || v4.IsUnspecified() {
				continue
			}
			out = append(out, ipCandidate{
				ip:      v4,
				network: &net.IPNet{IP: v4.Mask(n.Mask), Mask: n.Mask},
				iface:   iface.Name,
				ifaceNo: iface.Index,
			})
		}
	}
	return out, nil
}
