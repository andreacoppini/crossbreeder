//go:build linux

package netprobe

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// OpenDHCP binds the client port on one interface so a DHCP exchange can be
// run there whether or not the interface already holds an address.
//
// It needs privilege: port 68 is below 1024 and SO_BINDTODEVICE is
// CAP_NET_RAW. The sensor runs as a system service for exactly this reason —
// see pi/README.md for the capabilities its unit grants.
func OpenDHCP(iface string) (net.PacketConn, net.Addr, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.IPPROTO_UDP)
	if err != nil {
		return nil, nil, fmt.Errorf("dhcp socket: %w", err)
	}
	// From here on every failure has to close the descriptor: the sensor runs
	// for months, and one leaked fd per failed probe would end the run.
	fail := func(err error) (net.PacketConn, net.Addr, error) {
		unix.Close(fd)
		return nil, nil, err
	}
	for _, opt := range []int{unix.SO_BROADCAST, unix.SO_REUSEADDR} {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, opt, 1); err != nil {
			return fail(fmt.Errorf("dhcp setsockopt: %w", err))
		}
	}
	if iface != "" {
		if err := unix.BindToDevice(fd, iface); err != nil {
			return fail(fmt.Errorf("bind DHCP socket to %s: %w", iface, err))
		}
	}
	if err := unix.Bind(fd, &unix.SockaddrInet4{Port: 68}); err != nil {
		return fail(fmt.Errorf("bind port 68 (the sensor needs privilege for this): %w", err))
	}

	f := os.NewFile(uintptr(fd), "dhcp:"+iface)
	conn, err := net.FilePacketConn(f)
	f.Close() // FilePacketConn dups the descriptor
	if err != nil {
		unix.Close(fd)
		return nil, nil, err
	}
	return conn, &net.UDPAddr{IP: net.IPv4bcast, Port: 67}, nil
}
