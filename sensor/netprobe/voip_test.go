package netprobe

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// startReflector runs the real reflector on loopback and returns its address.
func startReflector(t *testing.T) string {
	t.Helper()
	conn, err := ListenReflector("127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); conn.Close() })
	go Reflect(ctx, conn)
	return conn.LocalAddr().String()
}

func TestVoIPOverLoopbackScoresWell(t *testing.T) {
	addr := startReflector(t)
	r := RunVoIP(context.Background(), VoIPTest{
		Reflector: addr, Packets: 50, Interval: 2 * time.Millisecond, Timeout: time.Second,
	})
	if r.Err != nil {
		t.Fatalf("stream failed: %v", r.Err)
	}
	if r.Received != 50 || r.LossPct != 0 {
		t.Fatalf("received %d of %d (%.1f%% loss)", r.Received, r.Sent, r.LossPct)
	}
	if !r.OK() || r.MOS < 4.0 {
		t.Errorf("MOS over loopback = %.2f, want a clean call", r.MOS)
	}
	if r.RTT <= 0 || r.RTTMax < r.RTTMin {
		t.Errorf("RTT figures are not coherent: %+v", r)
	}
	if r.OutOfOrder != 0 {
		t.Errorf("loopback reordered %d packets", r.OutOfOrder)
	}
}

// A reflector that swallows half the stream is what a congested uplink looks
// like, and it has to show up as loss and as a MOS nobody would accept.
func TestVoIPReportsLossAndDropsTheScore(t *testing.T) {
	conn, err := ListenReflector("127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback: %v", err)
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if binary.BigEndian.Uint32(buf[4:])%2 == 0 {
				buf[24] = 0xff
				conn.WriteTo(buf[:n], addr)
			}
		}
	}()

	r := RunVoIP(context.Background(), VoIPTest{
		Reflector: conn.LocalAddr().String(), Packets: 40,
		Interval: 2 * time.Millisecond, Timeout: time.Second,
	})
	if r.LossPct < 40 || r.LossPct > 60 {
		t.Fatalf("loss = %.1f%%, want about half", r.LossPct)
	}
	if r.OK() {
		t.Errorf("a call losing half its packets passed with MOS %.2f", r.MOS)
	}
}

func TestVoIPReportsASilentReflector(t *testing.T) {
	r := RunVoIP(context.Background(), VoIPTest{
		Reflector: "192.0.2.1:19999", Packets: 3,
		Interval: time.Millisecond, Timeout: 200 * time.Millisecond,
	})
	if r.Err == nil {
		t.Fatal("a silent reflector produced a result")
	}
	if r.LossPct != 100 {
		t.Errorf("loss = %.1f%%, want 100", r.LossPct)
	}
}

// The E-model is the whole basis of the score, so its shape is pinned: a
// clean line scores near the codec's ceiling, loss and delay each pull it
// down, and G.729 never beats G.711 on the same line.
func TestMOSFollowsTheEModel(t *testing.T) {
	clean := MOS(0, 20*time.Millisecond, time.Millisecond, G711)
	if clean < 4.3 || clean > 4.5 {
		t.Errorf("a clean G.711 line scored %.2f, want ~4.4", clean)
	}
	if lossy := MOS(5, 20*time.Millisecond, time.Millisecond, G711); lossy >= clean {
		t.Errorf("5%% loss scored %.2f, no worse than a clean line", lossy)
	}
	if slow := MOS(0, 600*time.Millisecond, time.Millisecond, G711); slow >= clean-0.5 {
		t.Errorf("a 600ms round trip scored %.2f, barely below a clean line", slow)
	}
	if jittery := MOS(0, 20*time.Millisecond, 80*time.Millisecond, G711); jittery >= clean {
		t.Errorf("80ms of jitter scored %.2f", jittery)
	}
	if g729 := MOS(0, 20*time.Millisecond, time.Millisecond, G729); g729 >= clean {
		t.Errorf("G.729 (%.2f) scored at least as well as G.711 (%.2f)", g729, clean)
	}
	if hopeless := MOS(80, 900*time.Millisecond, 200*time.Millisecond, G711); hopeless > 2 {
		t.Errorf("an unusable line scored %.2f", hopeless)
	}
	if hopeless := MOS(100, time.Second, time.Second, G711); hopeless < 1 {
		t.Errorf("MOS went below the 1.0 floor: %.2f", hopeless)
	}
}

// Marking the stream must not break it, whether or not the far end can report
// what it saw.
func TestVoIPWithDSCPMarking(t *testing.T) {
	addr := startReflector(t)
	r := RunVoIP(context.Background(), VoIPTest{
		Reflector: addr, Packets: 20, Interval: time.Millisecond,
		Timeout: time.Second, DSCP: DSCPEF,
	})
	if r.Err != nil {
		t.Fatalf("marked stream failed: %v", r.Err)
	}
	if r.Received == 0 {
		t.Fatal("nothing came back from a marked stream")
	}
	if r.SentDSCP != DSCPEF {
		t.Errorf("SentDSCP = %d", r.SentDSCP)
	}
	// Loopback keeps the marking where the platform can read it; where it
	// cannot, SeenDSCP stays -1 and the check reports as unknown.
	if r.SeenDSCP >= 0 && !r.DSCPPreserved() {
		t.Errorf("loopback re-marked EF as %d", r.SeenDSCP)
	}
}

func TestVoIPIgnoresStrayTraffic(t *testing.T) {
	addr := startReflector(t)
	// Anything that is not one of ours must not count as a returned packet.
	stray, _ := net.Dial("udp", addr)
	stray.Write([]byte("not a probe"))
	stray.Close()

	r := RunVoIP(context.Background(), VoIPTest{
		Reflector: addr, Packets: 10, Interval: time.Millisecond, Timeout: 500 * time.Millisecond,
	})
	if r.Received > r.Sent {
		t.Fatalf("received %d for %d sent — stray traffic was counted", r.Received, r.Sent)
	}
}
