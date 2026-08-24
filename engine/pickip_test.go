package main

import (
	"net"
	"testing"
)

// cand builds a candidate the way localCandidates would, so the ranking can be
// tested against machines this one does not have.
func cand(cidr, iface string, no int, routePick bool) ipCandidate {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	v4 := ip.To4()
	return ipCandidate{
		ip:        v4,
		network:   n,
		iface:     iface,
		ifaceNo:   no,
		private:   v4.IsPrivate(),
		routePick: routePick,
	}
}

func rank(cands []ipCandidate, targets []string) []ipCandidate {
	for i := range cands {
		for _, t := range targets {
			if v4 := net.ParseIP(t).To4(); v4 != nil && cands[i].network.Contains(v4) {
				cands[i].covered++
			}
		}
	}
	sortCandidates(cands)
	return cands
}

// The reported case: Tailscale's address is what the routing table offers, but
// the LAN address is the one the APs can actually reach.
func TestPrefersTheSubnetTheAPsAreOn(t *testing.T) {
	got := rank([]ipCandidate{
		cand("100.101.102.103/32", "Tailscale", 3, true),
		cand("192.168.77.105/24", "Ethernet", 1, false),
	}, []string{"192.168.77.115"})

	if got[0].ip.String() != "192.168.77.105" {
		t.Errorf("chose %s (%s), want the LAN address", got[0].ip, got[0].iface)
	}
}

// Rule 1 over rule 2: with APs on two subnets, the busier one wins.
func TestPrefersTheSubnetHoldingMostAPs(t *testing.T) {
	targets := []string{}
	for i := 1; i <= 40; i++ {
		targets = append(targets, net.IPv4(172, 20, 44, byte(i)).String())
	}
	targets = append(targets, "10.9.0.5", "10.9.0.6")

	got := rank([]ipCandidate{
		cand("10.9.0.2/24", "VPN", 2, true),
		cand("172.20.44.9/22", "Ethernet", 1, false),
	}, targets)

	if got[0].ip.String() != "172.20.44.9" {
		t.Errorf("chose %s, want the address covering 40 APs not 2", got[0].ip)
	}
	if got[0].covered != 40 || got[1].covered != 2 {
		t.Errorf("coverage counts wrong: %d and %d", got[0].covered, got[1].covered)
	}
}

// Rule 3: nothing shares a subnet with the APs, so RFC1918 wins. Tailscale's
// 100.64/10 is RFC 6598 shared space, not RFC1918, so it ranks below.
func TestFallsBackToRFC1918BeforeCGNAT(t *testing.T) {
	got := rank([]ipCandidate{
		cand("100.101.102.103/32", "Tailscale", 3, true),
		cand("10.8.0.2/24", "Corp VPN", 2, false),
	}, []string{"172.20.44.15"})

	if got[0].ip.String() != "10.8.0.2" {
		t.Errorf("chose %s, want the RFC1918 address", got[0].ip)
	}
	if got[0].covered != 0 {
		t.Errorf("nothing should have covered the target, got %d", got[0].covered)
	}
}

// Rule 4: if that is all there is, use it rather than failing.
func TestUsesWhateverIsLeft(t *testing.T) {
	got := rank([]ipCandidate{
		cand("100.101.102.103/32", "Tailscale", 3, true),
	}, []string{"172.20.44.15"})

	if got[0].ip.String() != "100.101.102.103" {
		t.Errorf("chose %s", got[0].ip)
	}
}

// Within a tier the routing table breaks the tie, and the result is stable.
func TestTieBreaksOnTheRouteThenDeterministically(t *testing.T) {
	got := rank([]ipCandidate{
		cand("10.1.0.5/24", "NIC B", 4, false),
		cand("10.2.0.5/24", "NIC A", 2, true),
	}, []string{"172.20.44.15"})
	if got[0].ip.String() != "10.2.0.5" {
		t.Errorf("chose %s, want the routing table's own answer", got[0].ip)
	}

	for i := 0; i < 5; i++ {
		again := rank([]ipCandidate{
			cand("10.3.0.5/24", "NIC C", 5, false),
			cand("10.1.0.5/24", "NIC B", 4, false),
		}, []string{"172.20.44.15"})
		if again[0].ip.String() != "10.1.0.5" {
			t.Fatalf("unstable choice on run %d: %s", i, again[0].ip)
		}
	}
}

// The real enumeration must at least run and return sane values here.
func TestLocalCandidatesAreUsable(t *testing.T) {
	cands, err := localCandidates()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.ip.To4() == nil {
			t.Errorf("%s is not IPv4", c.ip)
		}
		if c.ip.IsLoopback() || c.ip.IsLinkLocalUnicast() {
			t.Errorf("%s should have been filtered out", c.ip)
		}
		if c.network == nil {
			t.Errorf("%s has no network", c.ip)
		}
	}
}
