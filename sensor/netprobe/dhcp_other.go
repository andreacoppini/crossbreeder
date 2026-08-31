//go:build !linux

package netprobe

import (
	"errors"
	"net"
	"runtime"
)

// OpenDHCP is Linux-only. The sensor is a Linux appliance; the rest of the
// tool builds and its tests run everywhere, so this reports the limitation
// rather than failing the build on a developer's laptop.
func OpenDHCP(string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("DHCP tests need a Linux host (this is " + runtime.GOOS + ")")
}
