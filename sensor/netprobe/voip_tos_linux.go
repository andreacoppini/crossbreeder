//go:build linux

package netprobe

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// tosConn reads datagrams along with the TOS byte the packet arrived with, so
// a reflector can tell the sensor what marking survived the path.
type tosConn struct {
	raw syscall.RawConn
}

func newTOSReceiver(pc net.PacketConn) (*tosConn, bool) {
	uc, ok := pc.(*net.UDPConn)
	if !ok {
		return nil, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, false
	}
	var setErr error
	if err := raw.Control(func(fd uintptr) {
		setErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVTOS, 1)
	}); err != nil || setErr != nil {
		return nil, false
	}
	return &tosConn{raw: raw}, true
}

// ReadFromTOS is ReadFrom with the received TOS byte, or -1 when the kernel
// did not attach one.
func (t *tosConn) ReadFromTOS(b []byte) (int, int, net.Addr, error) {
	oob := make([]byte, 128)
	var (
		n, oobn int
		from    unix.Sockaddr
		readErr error
	)
	err := t.raw.Read(func(fd uintptr) bool {
		n, oobn, _, from, readErr = unix.Recvmsg(int(fd), b, oob, 0)
		// The runtime poller only wakes us when the socket is readable, but a
		// spurious wake-up is still possible; ask to be called again.
		return readErr != unix.EAGAIN && readErr != unix.EWOULDBLOCK
	})
	if err != nil {
		return 0, -1, nil, err
	}
	if readErr != nil {
		return 0, -1, nil, readErr
	}

	tos := -1
	if msgs, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil {
		for _, m := range msgs {
			if m.Header.Level == unix.IPPROTO_IP && m.Header.Type == unix.IP_TOS && len(m.Data) > 0 {
				tos = int(m.Data[0])
			}
		}
	}
	return n, tos, sockaddrToUDP(from), nil
}

func sockaddrToUDP(sa unix.Sockaddr) net.Addr {
	switch a := sa.(type) {
	case *unix.SockaddrInet4:
		return &net.UDPAddr{IP: net.IP(a.Addr[:]), Port: a.Port}
	case *unix.SockaddrInet6:
		return &net.UDPAddr{IP: net.IP(a.Addr[:]), Port: a.Port}
	}
	return nil
}
