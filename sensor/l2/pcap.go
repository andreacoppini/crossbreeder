package l2

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// PcapWriter writes the classic libpcap format, which every analyser reads.
// A capture taken by a sensor at the far end of a site is the evidence nobody
// has when the argument starts, and it has to open in Wireshark without
// conversion.
type PcapWriter struct {
	mu      sync.Mutex
	w       io.Writer
	snaplen int
	packets int
	bytes   int64
}

const (
	pcapMagic     = 0xa1b2c3d4
	linkTypeEther = 1
)

// NewPcapWriter writes the file header and returns a writer for the packets.
func NewPcapWriter(w io.Writer, snaplen int) (*PcapWriter, error) {
	if snaplen <= 0 {
		snaplen = 262144
	}
	hdr := make([]byte, 24)
	binary.LittleEndian.PutUint32(hdr[0:], pcapMagic)
	binary.LittleEndian.PutUint16(hdr[4:], 2) // version 2.4
	binary.LittleEndian.PutUint16(hdr[6:], 4)
	binary.LittleEndian.PutUint32(hdr[16:], uint32(snaplen))
	binary.LittleEndian.PutUint32(hdr[20:], linkTypeEther)
	if _, err := w.Write(hdr); err != nil {
		return nil, err
	}
	return &PcapWriter{w: w, snaplen: snaplen}, nil
}

// WritePacket records one frame. origLen is the length on the wire, which can
// be longer than the bytes captured when a snap length is in force.
func (p *PcapWriter) WritePacket(ts time.Time, data []byte, origLen int) error {
	if origLen < len(data) {
		origLen = len(data)
	}
	if len(data) > p.snaplen {
		data = data[:p.snaplen]
	}
	hdr := make([]byte, 16)
	binary.LittleEndian.PutUint32(hdr[0:], uint32(ts.Unix()))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(ts.Nanosecond()/1000))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(origLen))

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.w.Write(hdr); err != nil {
		return err
	}
	if _, err := p.w.Write(data); err != nil {
		return err
	}
	p.packets++
	p.bytes += int64(len(data)) + 16
	return nil
}

// Stats reports what has been written so far, which is what bounds a capture.
func (p *PcapWriter) Stats() (packets int, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.packets, p.bytes
}

// Filter is a small subset of what a capture filter usually does, applied in
// this process rather than compiled into the kernel. It covers the cases a
// remote capture is actually asked for — one client, one server, one port —
// without carrying a BPF compiler onto the sensor.
type Filter struct {
	Host  string // match either direction
	Port  int    // TCP or UDP, either direction
	Proto string // tcp, udp, icmp, arp
}

// Empty reports whether the filter would keep everything.
func (f Filter) Empty() bool { return f.Host == "" && f.Port == 0 && f.Proto == "" }

// Match reports whether a frame should be kept.
func (f Filter) Match(frame []byte) bool {
	if f.Empty() {
		return true
	}
	if len(frame) < 14 {
		return false
	}
	ethertype := binary.BigEndian.Uint16(frame[12:14])
	proto := strings.ToLower(f.Proto)

	if ethertype == 0x0806 { // ARP
		if proto != "" && proto != "arp" {
			return false
		}
		return f.Host == "" || arpMentions(frame[14:], f.Host)
	}
	if ethertype != 0x0800 || len(frame) < 34 {
		return false
	}
	ip := frame[14:]
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl {
		return false
	}
	src, dst := net.IP(ip[12:16]).String(), net.IP(ip[16:20]).String()
	if f.Host != "" && src != f.Host && dst != f.Host {
		return false
	}
	switch ip[9] {
	case 1:
		if proto != "" && proto != "icmp" {
			return false
		}
		return f.Port == 0
	case 6, 17:
		name := "tcp"
		if ip[9] == 17 {
			name = "udp"
		}
		if proto != "" && proto != name {
			return false
		}
		if f.Port == 0 {
			return true
		}
		if len(ip) < ihl+4 {
			return false
		}
		sport := int(binary.BigEndian.Uint16(ip[ihl:]))
		dport := int(binary.BigEndian.Uint16(ip[ihl+2:]))
		return sport == f.Port || dport == f.Port
	}
	return proto == ""
}

func arpMentions(arp []byte, host string) bool {
	if len(arp) < 28 {
		return false
	}
	return net.IP(arp[14:18]).String() == host || net.IP(arp[24:28]).String() == host
}

// CaptureOptions bounds a capture. A sensor sitting in a cupboard must never
// be able to fill its own disk, so a capture always has three ends: packets,
// bytes and time, whichever comes first.
type CaptureOptions struct {
	Interface string
	Snaplen   int
	Filter    Filter
	MaxPacket int
	MaxBytes  int64
	Duration  time.Duration
}

// CaptureStats is what a capture produced.
type CaptureStats struct {
	Packets  int
	Bytes    int64
	Duration time.Duration
	Dropped  int
}

var errCaptureUnsupported = errors.New("packet capture needs a Linux host with a raw socket")

func (o *CaptureOptions) withDefaults() {
	if o.Snaplen <= 0 {
		o.Snaplen = 262144
	}
	if o.MaxPacket <= 0 {
		o.MaxPacket = 20000
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 64 << 20
	}
	if o.Duration <= 0 {
		o.Duration = 60 * time.Second
	}
}
