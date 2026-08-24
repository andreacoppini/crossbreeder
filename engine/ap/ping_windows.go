//go:build windows

package ap

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows reaches ICMP through iphlpapi, which is the same unprivileged path
// ping.exe uses. A raw socket would need administrator rights, and this tool
// has to run as whatever the field engineer is logged in as.
var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

// ICMP_ECHO_REPLY begins Address(4) Status(4) RoundTripTime(4); the fields after
// that contain a pointer whose offset differs between 32- and 64-bit, so we
// read the three we need by offset instead of declaring the struct.
const (
	replyStatusOffset = 4
	replyRTTOffset    = 8
	ipSuccess         = 0
	replyBufSize      = 4096
)

var pingPayload = []byte("crossbreeder-engine-probe-00000000")

// Ping sends one ICMP echo request and waits up to timeout for the reply.
func Ping(ctx context.Context, host string, timeout time.Duration) PingResult {
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addrs) == 0 {
			return PingResult{Err: fmt.Errorf("resolve %s: %w", host, err)}
		}
		ip = addrs[0].IP
	}
	v4 := ip.To4()
	if v4 == nil {
		// iphlpapi's IPv6 echo is a different entry point; IPv6 APs are not a
		// case this tool has met, so fall back rather than pretend.
		noteICMPUnavailable(fmt.Errorf("ICMP echo for IPv6 address %s is not implemented", host))
		return PingResult{Err: fmt.Errorf("no ICMP for IPv6 %s", host)}
	}

	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 || windows.Handle(h) == windows.InvalidHandle {
		err := fmt.Errorf("IcmpCreateFile: %w", windows.GetLastError())
		noteICMPUnavailable(err)
		return PingResult{Err: err}
	}
	defer procIcmpCloseHandle.Call(h)

	dest := binary.LittleEndian.Uint32(v4) // IPAddr is in network byte order
	reply := make([]byte, replyBufSize)
	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = 1
	}

	start := time.Now()
	n, _, _ := procIcmpSendEcho.Call(
		h,
		uintptr(dest),
		uintptr(unsafe.Pointer(&pingPayload[0])),
		uintptr(uint16(len(pingPayload))),
		0, // no IP_OPTION_INFORMATION
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(uint32(len(reply))),
		uintptr(ms),
	)
	elapsed := time.Since(start)

	if n == 0 {
		// Timeout is the overwhelmingly common answer on a mostly-dead list and
		// is not worth surfacing as an error.
		return PingResult{RTT: elapsed}
	}
	if status := binary.LittleEndian.Uint32(reply[replyStatusOffset:]); status != ipSuccess {
		// A router answering "destination unreachable" on the AP's behalf still
		// counts as a reply, but not as a live AP.
		return PingResult{RTT: elapsed, Err: fmt.Errorf("icmp status %d", status)}
	}
	rtt := time.Duration(binary.LittleEndian.Uint32(reply[replyRTTOffset:])) * time.Millisecond
	if rtt == 0 {
		rtt = elapsed // sub-millisecond replies report 0
	}
	return PingResult{Alive: true, RTT: rtt}
}
