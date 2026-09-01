package l2

import (
	"encoding/binary"
	"net"
	"testing"
)

// lldpTLV assembles one TLV the way a switch would put it on the wire.
func lldpTLV(typ int, value []byte) []byte {
	out := make([]byte, 2+len(value))
	binary.BigEndian.PutUint16(out, uint16(typ)<<9|uint16(len(value)))
	copy(out[2:], value)
	return out
}

func lldpFrame(tlvs ...[]byte) []byte {
	frame := make([]byte, 14)
	copy(frame[0:6], lldpGroup)
	copy(frame[6:12], net.HardwareAddr{0x00, 0x1a, 0x1e, 0x11, 0x22, 0x33})
	binary.BigEndian.PutUint16(frame[12:], 0x88cc)
	for _, t := range tlvs {
		frame = append(frame, t...)
	}
	return append(frame, 0, 0) // end TLV
}

func TestParseLLDPFrame(t *testing.T) {
	chassis := append([]byte{4}, 0x00, 0x1a, 0x1e, 0xaa, 0xbb, 0xcc) // subtype 4: MAC
	port := append([]byte{5}, "GigabitEthernet1/0/24"...)            // subtype 5: interface name
	ttl := []byte{0, 120}
	caps := []byte{0, 0x1c, 0, 0x14} // supported, then enabled: bridge + router
	mgmt := append([]byte{5, 1}, append(net.ParseIP("10.20.0.9").To4(), 0, 0, 0, 0, 0)...)
	vlan := []byte{0x00, 0x80, 0xc2, 1, 0x00, 0x64} // port VLAN 100

	frame := lldpFrame(
		lldpTLV(tlvChassisID, chassis),
		lldpTLV(tlvPortID, port),
		lldpTLV(tlvTTL, ttl),
		lldpTLV(tlvPortDesc, []byte("Reception uplink")),
		lldpTLV(tlvSystemName, []byte("sw-reception-1")),
		lldpTLV(tlvSystemDesc, []byte("Aruba 2930F, firmware WC.16.10")),
		lldpTLV(tlvCapabilities, caps),
		lldpTLV(tlvMgmtAddr, mgmt),
		lldpTLV(tlvOrgSpecific, vlan),
	)

	n, ok := ParseFrame(frame)
	if !ok {
		t.Fatal("a well-formed LLDP frame was rejected")
	}
	if n.Protocol != "LLDP" || n.SystemName != "sw-reception-1" {
		t.Fatalf("neighbour = %+v", n)
	}
	if n.ChassisID != "00:1a:1e:aa:bb:cc" {
		t.Errorf("chassis = %q, want the MAC form", n.ChassisID)
	}
	if n.PortID != "GigabitEthernet1/0/24" || n.PortDesc != "Reception uplink" {
		t.Errorf("port = %q / %q", n.PortID, n.PortDesc)
	}
	if n.TTL != 120 || n.VLAN != 100 {
		t.Errorf("ttl = %d, vlan = %d", n.TTL, n.VLAN)
	}
	if n.MgmtAddr != "10.20.0.9" {
		t.Errorf("management address = %q", n.MgmtAddr)
	}
	if len(n.Capabilities) != 2 || n.Capabilities[0] != "Bridge" {
		t.Errorf("capabilities = %v", n.Capabilities)
	}
	if got := n.Summary(); got != "sw-reception-1 Reception uplink" {
		t.Errorf("summary = %q", got)
	}
}

func TestParseLLDPRejectsRubbish(t *testing.T) {
	// A TLV claiming more bytes than the frame holds is the shape a malformed
	// or truncated capture takes, and it must not panic.
	frame := lldpFrame([]byte{0x02, 0xff, 0x01})
	if _, ok := ParseFrame(frame); ok {
		t.Error("a TLV running past the frame was accepted")
	}
	if _, ok := ParseFrame([]byte{1, 2, 3}); ok {
		t.Error("a three-byte frame parsed as LLDP")
	}
	if _, ok := ParseFrame(nil); ok {
		t.Error("an empty frame parsed as LLDP")
	}
}

func cdpTLV(typ int, value []byte) []byte {
	out := make([]byte, 4+len(value))
	binary.BigEndian.PutUint16(out, uint16(typ))
	binary.BigEndian.PutUint16(out[2:], uint16(len(value)+4))
	copy(out[4:], value)
	return out
}

func TestParseCDPFrame(t *testing.T) {
	addresses := []byte{0, 0, 0, 1, 1, 1, 0xcc, 0, 4}
	addresses = append(addresses, net.ParseIP("172.30.0.2").To4()...)

	body := []byte{2, 180, 0, 0} // version 2, TTL 180, checksum
	body = append(body, cdpTLV(cdpDeviceID, []byte("sw-plantroom.example.com"))...)
	body = append(body, cdpTLV(cdpPortID, []byte("FastEthernet0/12"))...)
	body = append(body, cdpTLV(cdpVersion, []byte("Cisco IOS 15.2"))...)
	body = append(body, cdpTLV(cdpNativeVLAN, []byte{0x00, 0x0a})...)
	body = append(body, cdpTLV(cdpCapability, []byte{0, 0, 0, 0x09})...)
	body = append(body, cdpTLV(cdpAddresses, addresses)...)

	frame := make([]byte, 14)
	copy(frame[0:6], cdpGroup)
	copy(frame[6:12], net.HardwareAddr{0x00, 0x1a, 0x1e, 0x44, 0x55, 0x66})
	binary.BigEndian.PutUint16(frame[12:], uint16(len(body)+8)) // 802.3 length, not an ethertype
	frame = append(frame, 0xaa, 0xaa, 0x03, 0x00, 0x00, 0x0c, 0x20, 0x00)
	frame = append(frame, body...)

	n, ok := ParseFrame(frame)
	if !ok {
		t.Fatal("a well-formed CDP frame was rejected")
	}
	if n.Protocol != "CDP" || n.SystemName != "sw-plantroom.example.com" {
		t.Fatalf("neighbour = %+v", n)
	}
	if n.PortID != "FastEthernet0/12" || n.VLAN != 10 || n.TTL != 180 {
		t.Errorf("port = %q, vlan = %d, ttl = %d", n.PortID, n.VLAN, n.TTL)
	}
	if n.MgmtAddr != "172.30.0.2" {
		t.Errorf("management address = %q", n.MgmtAddr)
	}
	if len(n.Capabilities) != 2 {
		t.Errorf("capabilities = %v, want router and switch", n.Capabilities)
	}
	if n.String() == "" {
		t.Error("the neighbour rendered as nothing")
	}
}

// Most of what arrives on a promiscuous socket is ordinary traffic.
func TestParseFrameIgnoresOrdinaryTraffic(t *testing.T) {
	frame := make([]byte, 60)
	copy(frame[0:6], net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	binary.BigEndian.PutUint16(frame[12:], 0x0800)
	if _, ok := ParseFrame(frame); ok {
		t.Error("an IP frame was read as a discovery advertisement")
	}
}
