//go:build !linux

package netprobe

import "net"

// tosConn exists so the reflector compiles everywhere. Off Linux the marking
// that arrived cannot be read, and the sensor reports the DSCP check as
// unknown rather than guessing.
type tosConn struct{}

func newTOSReceiver(net.PacketConn) (*tosConn, bool) { return nil, false }

func (*tosConn) ReadFromTOS([]byte) (int, int, net.Addr, error) { panic("unreachable") }
