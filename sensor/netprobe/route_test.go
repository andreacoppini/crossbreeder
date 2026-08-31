package netprobe

import "testing"

const procRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlan0	00000000	0102A8C0	0003	0	0	600	00000000	0	0	0
wlan0	0002A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
eth0	00000000	01001EAC	0003	0	0	100	00000000	0	0	0
eth0	00001EAC	00000000	0001	0	0	100	00FFFFFF	0	0	0
`

func TestParseProcRoutePicksTheRightInterface(t *testing.T) {
	gw, err := parseProcRoute(procRoute, "wlan0")
	if err != nil || gw.String() != "192.168.2.1" {
		t.Fatalf("wlan0 gateway = %v, %v", gw, err)
	}
	// A sensor with both networks up must test each one's own gateway.
	gw, err = parseProcRoute(procRoute, "eth0")
	if err != nil || gw.String() != "172.30.0.1" {
		t.Fatalf("eth0 gateway = %v, %v", gw, err)
	}
	if _, err := parseProcRoute(procRoute, "wlan1"); err == nil {
		t.Error("an interface with no route reported one")
	}
}

func TestParseProcRouteSkipsNonDefaultRoutes(t *testing.T) {
	const onlySubnet = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
wlan0	0002A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
`
	if gw, err := parseProcRoute(onlySubnet, "wlan0"); err == nil {
		t.Errorf("a subnet route was reported as a default gateway: %v", gw)
	}
}

const procARP = `IP address       HW type     Flags       HW address            Mask     Device
192.168.2.1      0x1         0x2         b8:27:eb:aa:bb:cc     *        wlan0
192.168.2.50     0x1         0x0         00:00:00:00:00:00     *        wlan0
172.30.0.1       0x1         0x2         00:1a:1e:11:22:33     *        eth0
`

func TestParseProcARP(t *testing.T) {
	mac, err := parseProcARP(procARP, "192.168.2.1", "wlan0")
	if err != nil || mac != "b8:27:eb:aa:bb:cc" {
		t.Fatalf("mac = %q, err = %v", mac, err)
	}
	if _, err := parseProcARP(procARP, "192.168.2.50", "wlan0"); err == nil {
		t.Error("an incomplete entry was reported as a resolved neighbour")
	}
	if _, err := parseProcARP(procARP, "172.30.0.1", "wlan0"); err == nil {
		t.Error("an entry on another interface was matched")
	}
	if _, err := parseProcARP(procARP, "10.0.0.1", ""); err == nil {
		t.Error("an address that is not in the table was resolved")
	}
}
