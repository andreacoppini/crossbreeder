package netprobe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"time"

	"golang.org/x/net/ipv4"
)

// A VoIP test is a stream of small UDP packets at a codec's pace, reflected
// back by a peer. It measures what a call would experience — loss, jitter,
// round-trip delay — and whether the network kept the packets' priority
// marking, which is the usual reason a call sounds bad on a network where
// every other test passes.
const (
	voipMagic   = 0x43425558 // "CBUX"
	voipHdrSize = 28
)

// DSCP values worth marking with. EF is what a phone marks voice with.
const (
	DSCPBestEffort = 0
	DSCPAF41       = 34 // interactive video
	DSCPEF         = 46 // expedited forwarding: voice
)

// VoIPTest describes one call-shaped stream.
type VoIPTest struct {
	Reflector string // host:port of a sensor or collector running Reflect
	Packets   int    // default 250 — five seconds of G.711 at 20ms
	Interval  time.Duration
	Payload   int // bytes per packet, default 172 (G.711 at 20ms with RTP)
	DSCP      int // 0 leaves the packets unmarked
	Timeout   time.Duration
	Codec     Codec
}

// VoIPResult is what the stream experienced.
type VoIPResult struct {
	Sent       int
	Received   int
	LossPct    float64
	RTT        time.Duration // mean
	RTTMin     time.Duration
	RTTMax     time.Duration
	Jitter     time.Duration // RFC 3550 interarrival estimate
	MOS        float64       // 1.0–4.5, E-model
	SentDSCP   int
	SeenDSCP   int // what the far end says arrived; -1 when it could not tell
	OutOfOrder int
	Err        error
}

// OK reports whether the stream is good enough to carry a call. The bar is the
// one the E-model calls "some users dissatisfied": below 3.6, people complain.
func (r VoIPResult) OK() bool { return r.Err == nil && r.Received > 0 && r.MOS >= 3.6 }

// DSCPPreserved reports whether the marking survived the path. A false here
// with everything else healthy is a QoS policy finding, not a network fault.
func (r VoIPResult) DSCPPreserved() bool { return r.SeenDSCP < 0 || r.SentDSCP == r.SeenDSCP }

// Codec supplies the E-model's impairment factor for the encoding a call would
// use, so the MOS is the one that codec would actually achieve.
type Codec struct {
	Name string
	Ie   float64 // equipment impairment at zero loss
	Bpl  float64 // packet-loss robustness
}

// Codecs the sensor can emulate.
var (
	G711 = Codec{Name: "G.711", Ie: 0, Bpl: 25.1}
	G729 = Codec{Name: "G.729", Ie: 11, Bpl: 19}
	G722 = Codec{Name: "G.722", Ie: 13, Bpl: 24}
)

// RunVoIP sends the stream and reports what came back.
func RunVoIP(ctx context.Context, t VoIPTest) VoIPResult {
	if t.Packets <= 0 {
		t.Packets = 250
	}
	if t.Interval <= 0 {
		t.Interval = 20 * time.Millisecond
	}
	if t.Payload < voipHdrSize {
		t.Payload = 172
	}
	if t.Timeout <= 0 {
		t.Timeout = 2 * time.Second
	}
	if t.Codec.Name == "" {
		t.Codec = G711
	}
	res := VoIPResult{Sent: t.Packets, SentDSCP: t.DSCP, SeenDSCP: -1}

	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		res.Err = err
		return res
	}
	defer conn.Close()
	if t.DSCP != 0 {
		// TOS carries DSCP in its top six bits.
		if err := ipv4.NewConn(conn.(*net.UDPConn)).SetTOS(t.DSCP << 2); err != nil {
			res.Err = fmt.Errorf("cannot mark packets DSCP %d: %w", t.DSCP, err)
			return res
		}
	}
	peer, err := net.ResolveUDPAddr("udp4", t.Reflector)
	if err != nil {
		res.Err = err
		return res
	}

	type arrival struct {
		seq  uint32
		rtt  time.Duration
		when time.Duration // arrival, relative to the start of the stream
	}
	arrivals := make(chan arrival, t.Packets)
	done := make(chan struct{})
	origin := time.Now()

	go func() {
		defer close(done)
		buf := make([]byte, 2048)
		for {
			conn.SetReadDeadline(time.Now().Add(t.Timeout))
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < voipHdrSize || binary.BigEndian.Uint32(buf) != voipMagic {
				continue
			}
			seq := binary.BigEndian.Uint32(buf[4:])
			sent := int64(binary.BigEndian.Uint64(buf[8:]))
			if seen := int(buf[24]); seen != 0xff {
				res.SeenDSCP = seen >> 2
			}
			arrivals <- arrival{seq: seq, rtt: time.Duration(time.Now().UnixNano() - sent), when: time.Since(origin)}
		}
	}()

	packet := make([]byte, t.Payload)
	binary.BigEndian.PutUint32(packet, voipMagic)
	deadlineSend := time.Now()
	for i := 0; i < t.Packets; i++ {
		if err := ctx.Err(); err != nil {
			res.Err = err
			break
		}
		binary.BigEndian.PutUint32(packet[4:], uint32(i))
		binary.BigEndian.PutUint64(packet[8:], uint64(time.Now().UnixNano()))
		packet[24] = 0xff // filled in by the reflector with what it saw
		if _, err := conn.WriteTo(packet, peer); err != nil {
			res.Err = err
			break
		}
		deadlineSend = deadlineSend.Add(t.Interval)
		if d := time.Until(deadlineSend); d > 0 {
			timer := time.NewTimer(d)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
			}
		}
	}

	// Give the tail of the stream time to come back before giving up.
	time.Sleep(minDuration(t.Timeout, 500*time.Millisecond))
	conn.SetReadDeadline(time.Now())
	<-done
	close(arrivals)

	var rtts []time.Duration
	var jitter float64
	var prevTransit float64
	first := true
	highest := int64(-1)
	for a := range arrivals {
		res.Received++
		rtts = append(rtts, a.rtt)
		// Transit time relative to the ideal paced send, which is what the
		// interarrival jitter estimate needs.
		transit := a.when.Seconds() - float64(a.seq)*t.Interval.Seconds()
		if !first {
			d := math.Abs(transit - prevTransit)
			jitter += (d - jitter) / 16 // RFC 3550's smoothing
		}
		prevTransit, first = transit, false
		if int64(a.seq) < highest {
			res.OutOfOrder++
		} else {
			highest = int64(a.seq)
		}
	}
	if res.Sent > 0 {
		res.LossPct = 100 * float64(res.Sent-res.Received) / float64(res.Sent)
	}
	if res.LossPct < 0 {
		res.LossPct = 0
	}
	res.Jitter = time.Duration(jitter * float64(time.Second))
	if len(rtts) > 0 {
		sort.Slice(rtts, func(i, j int) bool { return rtts[i] < rtts[j] })
		res.RTTMin, res.RTTMax = rtts[0], rtts[len(rtts)-1]
		var total time.Duration
		for _, d := range rtts {
			total += d
		}
		res.RTT = total / time.Duration(len(rtts))
	}
	res.MOS = MOS(res.LossPct, res.RTT, res.Jitter, t.Codec)
	if res.Received == 0 && res.Err == nil {
		res.Err = errors.New("the reflector never answered")
	}
	return res
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// MOS scores a stream with the ITU-T G.107 E-model, reduced to the terms a
// sensor can measure: delay, jitter (which the jitter buffer turns into more
// delay and more loss) and packet loss.
func MOS(lossPct float64, rtt, jitter time.Duration, codec Codec) float64 {
	if codec.Bpl == 0 {
		codec = G711
	}
	// A jitter buffer holds roughly two jitter periods, and everything late
	// past that is discarded, so jitter costs both delay and loss.
	oneWayMs := rtt.Seconds()*1000/2 + jitter.Seconds()*1000*2

	// Id: delay impairment. Below 177.3ms the penalty is mild and roughly
	// linear; past it, it steepens sharply.
	var id float64
	if oneWayMs < 177.3 {
		id = 0.024 * oneWayMs
	} else {
		id = 0.024*oneWayMs + 0.11*(oneWayMs-177.3)
	}

	ppl := lossPct
	ieEff := codec.Ie + (95-codec.Ie)*ppl/(ppl+codec.Bpl)

	r := 93.2 - id - ieEff
	switch {
	case r < 0:
		return 1
	case r > 100:
		r = 100
	}
	mos := 1 + 0.035*r + r*(r-60)*(100-r)*7e-6
	return math.Round(mos*100) / 100
}

// Reflect answers VoIP probes on conn until ctx is done. Running it on the
// collector — or on a second sensor — is what makes a site-to-site call test
// possible without any other software.
//
// Where the platform can tell us, it reports back the DSCP value that actually
// arrived, which is how the sensor finds out that something along the path
// stripped the marking off the voice packets.
func Reflect(ctx context.Context, conn net.PacketConn) error {
	tos, tosReadable := newTOSReceiver(conn)

	go func() {
		<-ctx.Done()
		conn.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, 2048)
	for {
		var (
			n       int
			seenTOS int
			addr    net.Addr
			err     error
		)
		if tosReadable {
			n, seenTOS, addr, err = tos.ReadFromTOS(buf)
		} else {
			seenTOS = -1
			n, addr, err = conn.ReadFrom(buf)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		if n < voipHdrSize || binary.BigEndian.Uint32(buf) != voipMagic {
			continue
		}
		if seenTOS < 0 {
			buf[24] = 0xff // this reflector cannot see the marking
		} else {
			buf[24] = byte(seenTOS)
		}
		binary.BigEndian.PutUint64(buf[16:], uint64(time.Now().UnixNano()))
		if _, err := conn.WriteTo(buf[:n], addr); err != nil && ctx.Err() != nil {
			return nil
		}
	}
}

// ListenReflector opens the UDP port a reflector answers on.
func ListenReflector(addr string) (net.PacketConn, error) {
	return net.ListenPacket("udp4", addr)
}
