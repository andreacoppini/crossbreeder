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

// pat is one thing Expect can wait for.
type pat struct {
	text string
	// atEnd restricts the match to the tail of what has arrived, which is what
	// distinguishes a prompt the device is waiting at from the same characters
	// appearing in a banner or in echoed output.
	atEnd bool
	// fold matches without regard to case, for prompts whose capitalisation
	// differs between firmware builds ("New Password:" against "new password:").
	fold bool
}

func anywhere(s string) pat     { return pat{text: s} }
func atEnd(s string) pat        { return pat{text: s, atEnd: true} }
func anywhereFold(s string) pat { return pat{text: s, fold: true} }
func atEndFold(s string) pat    { return pat{text: s, atEnd: true, fold: true} }

// Collect keeps reading for up to d and returns whatever arrived. It is for
// watching a long-running operation that prints progress without ever coming
// back to a prompt.
func (e *expecter) Collect(d time.Duration) string {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		select {
		case chunk, ok := <-e.rx:
			if !ok {
				return e.drain()
			}
			e.record(chunk)
			e.buf.Write(chunk)
		case <-e.rxErr:
			return e.drain()
		case <-deadline.C:
			return e.drain()
		}
	}
}

// Expect reads until one of want appears anywhere in the stream. It returns the
// index of the pattern that matched and everything consumed up to and including
// it.
func (e *expecter) Expect(want ...string) (int, string, error) {
	pats := make([]pat, len(want))
	for i, w := range want {
		pats[i] = anywhere(w)
	}
	return e.ExpectPats(pats...)
}

// Pending returns what has arrived but not yet matched, for error context.
func (e *expecter) Pending() string { return e.buf.String() }

// ExpectPats reads until one of pats matches.
func (e *expecter) ExpectPats(want ...pat) (int, string, error) {
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
			// Leave the buffer alone. Draining it here used to throw away a
			// prompt that had in fact arrived, so a caller retrying with a
			// different pattern could never match it.
			return -1, e.buf.String(), fmt.Errorf("%w: wanted one of %s", ErrExpectTimeout, describe(want))
		}
	}
}

// scan resolves the pending buffer against the wanted patterns.
//
// Free patterns are matched first, earliest position wins: "Login incorrect"
// has to beat a prompt that arrives after it. Only if none matches do the
// end-anchored patterns get a look, longest first, so "(ap-mode)# " wins over
// "# ".
func (e *expecter) scan(want []pat) (int, string, bool) {
	s := e.buf.String()

	bestIdx, bestAt := -1, -1
	for i, w := range want {
		if w.atEnd {
			continue
		}
		at := strings.Index(s, w.text)
		if w.fold {
			at = indexFold(s, w.text)
		}
		if at >= 0 && (bestAt < 0 || at < bestAt) {
			bestIdx, bestAt = i, at
		}
	}
	if bestIdx >= 0 {
		return bestIdx, e.take(bestAt + len(want[bestIdx].text)), true
	}

	tail := strings.TrimRight(s, " \t\r\n\x00")
	bestIdx = -1
	for i, w := range want {
		if !w.atEnd {
			continue
		}
		text := strings.TrimRight(w.text, " ")
		hit := strings.HasSuffix(tail, text)
		if w.fold {
			hit = hasSuffixFold(tail, text)
		}
		if hit {
			if bestIdx < 0 || len(w.text) > len(want[bestIdx].text) {
				bestIdx = i
			}
		}
	}
	if bestIdx < 0 {
		return -1, "", false
	}
	return bestIdx, e.take(len(s)), true
}

// take consumes the first n bytes of the buffer and returns them.
func (e *expecter) take(n int) string {
	s := e.buf.String()
	out := s[:n]
	e.buf.Reset()
	e.buf.WriteString(s[n:])
	return out
}

func describe(want []pat) string {
	parts := make([]string, len(want))
	for i, w := range want {
		parts[i] = fmt.Sprintf("%q", w.text)
	}
	return strings.Join(parts, ", ")
}

func (e *expecter) drain() string {
	s := e.buf.String()
	e.buf.Reset()
	return s
}

// indexFold is strings.Index with ASCII case folding. It compares byte by byte
// rather than lowercasing a copy, so the index it returns is an offset into s
// itself — which scan relies on to consume exactly the matched text.
func indexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && equalFold(s[len(s)-len(suffix):], suffix)
}

// equalFold compares two equal-length strings with ASCII case folding. The AP
// CLI is ASCII, so this deliberately does not do Unicode folding, which could
// change byte length and so break the offsets indexFold hands back.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
