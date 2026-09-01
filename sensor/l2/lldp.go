// Package l2 covers what the sensor can learn from the wire itself: which
// switch and port it is plugged into, and — when someone asks — a packet
// capture taken where the problem is rather than in the data centre.
package l2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Neighbour is the device on the other end of the sensor's Ethernet port, as
// it describes itself. Knowing the switch and port a sensor is on is what
// turns "the wired network is slow at reception" into a port to look at.
type Neighbour struct {
	Protocol     string // LLDP or CDP
	ChassisID    string
	PortID       string
	PortDesc     string
	SystemName   string
	SystemDesc   string
	MgmtAddr     string
	VLAN         int
	Capabilities []string
	TTL          int
	Received     time.Time
}

// Summary is the one-line form the dashboard shows.
func (n Neighbour) Summary() string {
	name := n.SystemName
	if name == "" {
		name = n.ChassisID
	}
	port := n.PortDesc
	if port == "" {
		port = n.PortID
	}
	if name == "" {
		return ""
	}
	if port == "" {
		return name
	}
	return name + " " + port
}

// Multicast addresses the two protocols use.
var (
	lldpGroup = net.HardwareAddr{0x01, 0x80, 0xc2, 0x00, 0x00, 0x0e}
	cdpGroup  = net.HardwareAddr{0x01, 0x00, 0x0c, 0xcc, 0xcc, 0xcc}
)

// ParseFrame reads a discovery frame off the wire, whichever of the two
// protocols it is. Anything else is rejected without an error worth reporting:
// on ETH_P_ALL most frames are not for us.
func ParseFrame(frame []byte) (Neighbour, bool) {
	if len(frame) < 14 {
		return Neighbour{}, false
	}
	dst := net.HardwareAddr(frame[0:6])
	ethertype := binary.BigEndian.Uint16(frame[12:14])

	switch {
	case ethertype == 0x88cc || dst.String() == lldpGroup.String():
		n, err := ParseLLDP(frame[14:])
		if err != nil {
			return Neighbour{}, false
		}
		return n, true
	case dst.String() == cdpGroup.String():
		// CDP rides on 802.3 with an LLC/SNAP header: AA AA 03, then the OUI
		// and protocol ID, then the CDP frame itself.
		payload := frame[14:]
		if len(payload) < 8 || payload[0] != 0xaa || payload[1] != 0xaa {
			return Neighbour{}, false
		}
		n, err := ParseCDP(payload[8:])
		if err != nil {
			return Neighbour{}, false
		}
		return n, true
	}
	return Neighbour{}, false
}

// LLDP TLV types (IEEE 802.1AB).
const (
	tlvEnd          = 0
	tlvChassisID    = 1
	tlvPortID       = 2
	tlvTTL          = 3
	tlvPortDesc     = 4
	tlvSystemName   = 5
	tlvSystemDesc   = 6
	tlvCapabilities = 7
	tlvMgmtAddr     = 8
	tlvOrgSpecific  = 127
)

// ParseLLDP reads the TLV chain of an LLDP frame, starting after the
// ethertype.
func ParseLLDP(b []byte) (Neighbour, error) {
	n := Neighbour{Protocol: "LLDP", Received: time.Now()}
	seen := false
	for off := 0; off+2 <= len(b); {
		header := binary.BigEndian.Uint16(b[off:])
		typ := int(header >> 9)
		length := int(header & 0x01ff)
		off += 2
		if typ == tlvEnd {
			break
		}
		if off+length > len(b) {
			return n, errors.New("LLDP TLV runs past the end of the frame")
		}
		value := b[off : off+length]
		off += length
		seen = true

		switch typ {
		case tlvChassisID:
			n.ChassisID = idString(value)
		case tlvPortID:
			n.PortID = idString(value)
		case tlvTTL:
			if len(value) >= 2 {
				n.TTL = int(binary.BigEndian.Uint16(value))
			}
		case tlvPortDesc:
			n.PortDesc = text(value)
		case tlvSystemName:
			n.SystemName = text(value)
		case tlvSystemDesc:
			n.SystemDesc = text(value)
		case tlvCapabilities:
			if len(value) >= 4 {
				n.Capabilities = decodeCapabilities(binary.BigEndian.Uint16(value[2:]))
			}
		case tlvMgmtAddr:
			n.MgmtAddr = managementAddress(value)
		case tlvOrgSpecific:
			// 00-80-C2 subtype 1 is the port VLAN, which is the setting most
			// often wrong when a port "does not work".
			if len(value) >= 6 && value[0] == 0x00 && value[1] == 0x80 && value[2] == 0xc2 && value[3] == 1 {
				n.VLAN = int(binary.BigEndian.Uint16(value[4:]))
			}
		}
	}
	if !seen {
		return n, errors.New("no LLDP TLVs in the frame")
	}
	return n, nil
}

// idString renders a chassis or port identifier according to its subtype: a
// MAC where it is one, an address where it is one, otherwise the text.
func idString(v []byte) string {
	if len(v) < 2 {
		return ""
	}
	subtype, value := v[0], v[1:]
	switch subtype {
	case 4: // MAC address
		if len(value) == 6 {
			return net.HardwareAddr(value).String()
		}
	case 5: // network address
		if len(value) == 5 && value[0] == 1 {
			return net.IP(value[1:]).String()
		}
	}
	return text(value)
}

func managementAddress(v []byte) string {
	if len(v) < 2 {
		return ""
	}
	length := int(v[0])
	if length < 1 || 1+length > len(v) {
		return ""
	}
	family, addr := v[1], v[2:1+length]
	switch family {
	case 1:
		if len(addr) == 4 {
			return net.IP(addr).String()
		}
	case 2:
		if len(addr) == 16 {
			return net.IP(addr).String()
		}
	}
	return ""
}

var capabilityNames = []struct {
	bit  uint16
	name string
}{
	{1 << 0, "Other"}, {1 << 1, "Repeater"}, {1 << 2, "Bridge"},
	{1 << 3, "WLAN AP"}, {1 << 4, "Router"}, {1 << 5, "Telephone"},
	{1 << 6, "DOCSIS"}, {1 << 7, "Station"},
}

func decodeCapabilities(bits uint16) []string {
	var out []string
	for _, c := range capabilityNames {
		if bits&c.bit != 0 {
			out = append(out, c.name)
		}
	}
	return out
}

// CDP TLV types.
const (
	cdpDeviceID   = 0x0001
	cdpAddresses  = 0x0002
	cdpPortID     = 0x0003
	cdpCapability = 0x0004
	cdpVersion    = 0x0005
	cdpPlatform   = 0x0006
	cdpNativeVLAN = 0x000a
)

// ParseCDP reads a Cisco Discovery Protocol frame, starting at its own header.
func ParseCDP(b []byte) (Neighbour, error) {
	if len(b) < 4 {
		return Neighbour{}, errors.New("short CDP frame")
	}
	n := Neighbour{Protocol: "CDP", Received: time.Now(), TTL: int(b[1])}
	for off := 4; off+4 <= len(b); {
		typ := binary.BigEndian.Uint16(b[off:])
		length := int(binary.BigEndian.Uint16(b[off+2:]))
		if length < 4 || off+length > len(b) {
			break
		}
		value := b[off+4 : off+length]
		off += length

		switch typ {
		case cdpDeviceID:
			n.SystemName = text(value)
			n.ChassisID = n.SystemName
		case cdpPortID:
			n.PortID = text(value)
			n.PortDesc = n.PortID
		case cdpVersion:
			n.SystemDesc = text(value)
		case cdpPlatform:
			if n.SystemDesc == "" {
				n.SystemDesc = text(value)
			}
		case cdpNativeVLAN:
			if len(value) >= 2 {
				n.VLAN = int(binary.BigEndian.Uint16(value))
			}
		case cdpCapability:
			if len(value) >= 4 {
				n.Capabilities = decodeCDPCapabilities(binary.BigEndian.Uint32(value))
			}
		case cdpAddresses:
			n.MgmtAddr = firstCDPAddress(value)
		}
	}
	if n.SystemName == "" && n.PortID == "" {
		return n, errors.New("no usable CDP TLVs")
	}
	return n, nil
}

func decodeCDPCapabilities(bits uint32) []string {
	var out []string
	for _, c := range []struct {
		bit  uint32
		name string
	}{
		{0x01, "Router"}, {0x02, "Bridge"}, {0x04, "Source route bridge"},
		{0x08, "Switch"}, {0x10, "Host"}, {0x20, "IGMP"}, {0x40, "Repeater"},
	} {
		if bits&c.bit != 0 {
			out = append(out, c.name)
		}
	}
	return out
}

// firstCDPAddress reads the first address out of the address list, which is
// the one to manage the switch on.
func firstCDPAddress(v []byte) string {
	if len(v) < 4 {
		return ""
	}
	count := binary.BigEndian.Uint32(v)
	off := 4
	for i := uint32(0); i < count && off+5 <= len(v); i++ {
		protoLen := int(v[off+1])
		off += 2 + protoLen
		if off+2 > len(v) {
			return ""
		}
		addrLen := int(binary.BigEndian.Uint16(v[off:]))
		off += 2
		if off+addrLen > len(v) {
			return ""
		}
		if addrLen == 4 {
			return net.IP(v[off : off+4]).String()
		}
		off += addrLen
	}
	return ""
}

// text trims the padding and control characters network gear pads its strings
// with, so a switch name does not arrive with a trailing NUL.
func text(v []byte) string {
	return strings.TrimRight(strings.TrimSpace(string(v)), "\x00")
}

// String renders a neighbour the way it would be read out over the phone.
func (n Neighbour) String() string {
	parts := []string{n.Protocol + ": " + n.Summary()}
	if n.VLAN != 0 {
		parts = append(parts, fmt.Sprintf("VLAN %d", n.VLAN))
	}
	if n.MgmtAddr != "" {
		parts = append(parts, "managed at "+n.MgmtAddr)
	}
	return strings.Join(parts, ", ")
}
