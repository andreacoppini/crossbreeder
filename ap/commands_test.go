package ap

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Several commands go out in order, in the one session — which is the whole
// point: unlocking a country code is a sequence, not a single command.
func TestCommandsRunInOrderInOneSession(t *testing.T) {
	f := newFakeAP(t, KindZoneFlex, 0, false)
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.Actions = Actions{Commands: []string{
		"set country-code unlock",
		"set country-code GB",
		"get country-code",
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if r := Run(ctx, host, cfg); r.Error != "" {
		t.Fatalf("run failed: %s", r.Error)
	}

	seen := f.seen()
	var got []string
	for _, c := range seen {
		if strings.HasPrefix(c, "set country-code") || c == "get country-code" {
			got = append(got, c)
		}
	}
	want := []string{"set country-code unlock", "set country-code GB", "get country-code"}
	if len(got) != len(want) {
		t.Fatalf("AP saw %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d = %q, want %q (order matters)", i, got[i], want[i])
		}
	}
}

// The original ignored whether each line came back to the prompt and carried on
// regardless. A command that stalls must not cost the operator the rest of the
// sequence — on a locked country code the unlock is the first line.
func TestAStalledCommandDoesNotCostTheRest(t *testing.T) {
	f := newFakeAP(t, KindZoneFlex, 0, false)
	f.swallow = "set country-code unlock" // answers nothing, so the wait times out
	host, port := f.addr()

	cfg := testConfig()
	cfg.Port = port
	cfg.DialogTimeout = 600 * time.Millisecond
	cfg.Actions = Actions{Commands: []string{
		"set country-code unlock",
		"set country-code GB",
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r := Run(ctx, host, cfg)

	if r.Error != "" {
		t.Fatalf("a stalled command failed the whole AP: %s", r.Error)
	}
	var sawSecond bool
	for _, c := range f.seen() {
		if c == "set country-code GB" {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Error("the command after the stalled one was never sent")
	}
	if !strings.Contains(r.Note, "no prompt back") {
		t.Errorf("Note = %q, want it to report the stalled command", r.Note)
	}
}
