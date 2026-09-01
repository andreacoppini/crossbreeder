//go:build linux

package l2

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// openRaw binds a packet socket to one interface. Everything on the wire
// arrives on it, which is what both the capture and the discovery listener
// need; the filtering is done in this process.
func openRaw(iface string) (*os.File, error) {
	proto := int(htons(unix.ETH_P_ALL))
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, proto)
	if err != nil {
		return nil, fmt.Errorf("raw socket (the sensor needs privilege for this): %w", err)
	}
	if iface != "" {
		ifi, err := net.InterfaceByName(iface)
		if err != nil {
			unix.Close(fd)
			return nil, err
		}
		if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: uint16(proto), Ifindex: ifi.Index}); err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("bind to %s: %w", iface, err)
		}
	}
	return os.NewFile(uintptr(fd), "packet:"+iface), nil
}

// htons converts a protocol number to the network order AF_PACKET wants. On a
// Pi that is not the same number the constant is written as.
func htons(v uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return binary.NativeEndian.Uint16(b[:])
}

// Capture writes frames from an interface into w in pcap format until one of
// the bounds in opts is reached or ctx is cancelled.
func Capture(ctx context.Context, opts CaptureOptions, w io.Writer) (CaptureStats, error) {
	opts.withDefaults()
	var stats CaptureStats

	f, err := openRaw(opts.Interface)
	if err != nil {
		return stats, err
	}
	defer f.Close()

	pw, err := NewPcapWriter(w, opts.Snaplen)
	if err != nil {
		return stats, err
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Duration)
	defer cancel()
	go func() {
		<-ctx.Done()
		f.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, opts.Snaplen)
	start := time.Now()
	for {
		f.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := f.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if os.IsTimeout(err) {
				continue
			}
			stats.Duration = time.Since(start)
			return stats, err
		}
		if !opts.Filter.Match(buf[:n]) {
			continue
		}
		if err := pw.WritePacket(time.Now(), buf[:n], n); err != nil {
			stats.Duration = time.Since(start)
			return stats, err
		}
		packets, bytes := pw.Stats()
		stats.Packets, stats.Bytes = packets, bytes
		if packets >= opts.MaxPacket || bytes >= opts.MaxBytes {
			break
		}
	}
	stats.Duration = time.Since(start)
	return stats, nil
}

// Discover listens for LLDP and CDP advertisements. Switches send them once a
// minute by default, so the window has to be longer than that to be worth
// anything — and a sensor only listens when asked, rather than holding a
// promiscuous socket open for months.
func Discover(ctx context.Context, iface string, window time.Duration) ([]Neighbour, error) {
	if window <= 0 {
		window = 70 * time.Second
	}
	f, err := openRaw(iface)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()
	go func() {
		<-ctx.Done()
		f.SetReadDeadline(time.Now())
	}()

	seen := map[string]Neighbour{}
	buf := make([]byte, 4096)
	for {
		f.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := f.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if os.IsTimeout(err) {
				continue
			}
			return neighbourList(seen), err
		}
		if neighbour, ok := ParseFrame(buf[:n]); ok {
			seen[neighbour.Protocol+"/"+neighbour.Summary()] = neighbour
		}
	}
	return neighbourList(seen), nil
}

func neighbourList(m map[string]Neighbour) []Neighbour {
	out := make([]Neighbour, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	return out
}
