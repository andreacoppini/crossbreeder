package ap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ErrExpectTimeout is returned when none of the wanted patterns arrived in time.
var ErrExpectTimeout = errors.New("timed out waiting for prompt")

// expecter is a small expect(1)-style helper over an SSH channel. The SSH
// channel has no SetReadDeadline, so a background goroutine pumps bytes into a
// channel and Expect races them against a timer.
type expecter struct {
	w       io.Writer
	rx      chan []byte
	rxErr   chan error
	buf     bytes.Buffer
	timeout time.Duration

	mu         sync.Mutex
	transcript bytes.Buffer
}

func newExpecter(w io.Writer, r io.Reader, timeout time.Duration) *expecter {
	e := &expecter{
		w:       w,
		rx:      make(chan []byte, 16),
		rxErr:   make(chan error, 1),
		timeout: timeout,
	}
	go func() {
		defer close(e.rx)
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, b[:n])
				e.rx <- chunk
			}
			if err != nil {
				e.rxErr <- err
				return
			}
		}
	}()
	return e
}

// Transcript returns everything received so far, for the debug pane / logs.
func (e *expecter) Transcript() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.transcript.String()
}

func (e *expecter) record(b []byte) {
	e.mu.Lock()
	e.transcript.Write(b)
	e.mu.Unlock()
}

// Send writes a line terminated with a bare LF, the way the AP CLI expects it.
func (e *expecter) Send(line string) error {
	e.record([]byte(line + "\n"))
	_, err := io.WriteString(e.w, line+"\n")
	return err
}

// Expect reads until one of want appears. It returns the index of the pattern
// that matched and everything consumed up to and including it.
func (e *expecter) Expect(want ...string) (int, string, error) {
	deadline := time.NewTimer(e.timeout)
	defer deadline.Stop()

	for {
		if idx, out, ok := e.scan(want); ok {
			return idx, out, nil
		}
		select {
		case chunk, ok := <-e.rx:
			if !ok {
				if idx, out, ok := e.scan(want); ok {
					return idx, out, nil
				}
				return -1, e.drain(), io.EOF
			}
			e.record(chunk)
			e.buf.Write(chunk)
		case err := <-e.rxErr:
			if idx, out, ok := e.scan(want); ok {
				return idx, out, nil
			}
			return -1, e.drain(), err
		case <-deadline.C:
			return -1, e.drain(), fmt.Errorf("%w: wanted one of %q", ErrExpectTimeout, want)
		}
	}
}

// scan looks for the earliest match of any wanted pattern in the pending buffer.
func (e *expecter) scan(want []string) (int, string, bool) {
	s := e.buf.String()
	bestIdx, bestAt := -1, -1
	for i, w := range want {
		if at := strings.Index(s, w); at >= 0 {
			if bestAt < 0 || at < bestAt {
				bestIdx, bestAt = i, at
			}
		}
	}
	if bestIdx < 0 {
		return -1, "", false
	}
	end := bestAt + len(want[bestIdx])
	out := s[:end]
	e.buf.Reset()
	e.buf.WriteString(s[end:])
	return bestIdx, out, true
}

func (e *expecter) drain() string {
	s := e.buf.String()
	e.buf.Reset()
	return s
}
