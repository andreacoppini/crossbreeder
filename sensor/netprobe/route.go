package netprobe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Interface is what the sensor knows about one of its own ports, and the
// first thing an operator wants when a sensor stops reporting.
type Interface struct {
	Name    string
	MAC     string
	MTU     int
	Up      bool
	Addrs   []string
	Gateway string
}

// DescribeInterface reads the current state of one interface.
func DescribeInterface(name string) (Interface, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return Interface{}, err
	}
	out := Interface{
		Name: iface.Name,
		MAC:  iface.HardwareAddr.String(),
		MTU:  iface.MTU,
		Up:   iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0,
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return out, err
	}
	for _, a := range addrs {
		out.Addrs = append(out.Addrs, a.String())
	}
	if gw, err := DefaultGateway(name); err == nil {
		out.Gateway = gw.String()
	}
	return out, nil
}

// IPv4Of returns the first IPv4 address on an interface, which is what the
// sensor reports as its address on that network.
func IPv4Of(name string) (net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			if v4 := ipnet.IP.To4(); v4 != nil && !v4.IsLinkLocalUnicast() {
				return v4, nil
			}
		}
	}
	return nil, fmt.Errorf("%s has no IPv4 address", name)
}

// DefaultGateway reads the routing table for the gateway on one interface.
// Reading the table rather than asking a resolver matters on a sensor with two
// networks up at once: the gateway being tested is the one on the interface
// the test belongs to, not whichever the OS prefers.
func DefaultGateway(iface string) (net.IP, error) {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, errors.New("the routing table is only readable on Linux: " + err.Error())
	}
	return parseProcRoute(string(b), iface)
}

// parseProcRoute finds the default route for an interface in the kernel's
// table. The addresses there are little-endian hexadecimal.
func parseProcRoute(text, iface string) (net.IP, error) {
	for i, line := range strings.Split(text, "\n") {
		if i == 0 {
			continue // header
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if iface != "" && f[0] != iface {
			continue
		}
		if f[1] != "00000000" { // destination 0.0.0.0: the default route
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x2 == 0 { // RTF_GATEWAY
			continue
		}
		gw, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil {
			continue
		}
		ip := make(net.IP, 4)
		binary.LittleEndian.PutUint32(ip, uint32(gw))
		return ip, nil
	}
	if iface != "" {
		return nil, fmt.Errorf("no default route on %s", iface)
	}
	return nil, errors.New("no default route")
}

// NeighbourMAC reports the MAC an address resolved to, which identifies the
// gateway — and shows it changing, which is what a spoofed or failed-over
// gateway looks like from the client side.
func NeighbourMAC(ip, iface string) (string, error) {
	b, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return "", err
	}
	return parseProcARP(string(b), ip, iface)
}

func parseProcARP(text, ip, iface string) (string, error) {
	for i, line := range strings.Split(text, "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 || f[0] != ip {
			continue
		}
		if iface != "" && f[5] != iface {
			continue
		}
		if f[3] == "00:00:00:00:00:00" {
			return "", fmt.Errorf("%s is in the neighbour table but unresolved", ip)
		}
		return f[3], nil
	}
	return "", fmt.Errorf("%s is not in the neighbour table", ip)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
