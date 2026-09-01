package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store keeps the history. It is a file of JSON lines per day, which is the
// right shape for this: a sensor writes a handful of records every five
// minutes, an SD card hates random writes, and an operator who wants the raw
// data can read it with grep.
type Store struct {
	dir    string
	keep   time.Duration
	maxMiB int

	mu     sync.Mutex
	recent []SuiteResult // newest last
	max    int
}

// NewStore opens (and creates) the directory a sensor keeps its history in. A
// sensor with no directory configured still works: it keeps the recent passes
// in memory and forgets them on restart, which is what a sensor being
// commissioned on a bench wants anyway.
func NewStore(cfg Storage) (*Store, error) {
	s := &Store{
		dir: cfg.Dir, keep: cfg.Keep.D(), maxMiB: cfg.MaxMiB, max: 2000,
	}
	if s.dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("history directory: %w", err)
	}
	return s, s.Prune()
}

// Append records one pass.
func (s *Store) Append(r SuiteResult) error {
	s.mu.Lock()
	s.recent = append(s.recent, r)
	if len(s.recent) > s.max {
		s.recent = append([]SuiteResult(nil), s.recent[len(s.recent)-s.max:]...)
	}
	s.mu.Unlock()

	if s.dir == "" {
		return nil
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.fileFor(r.Start), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *Store) fileFor(t time.Time) string {
	return filepath.Join(s.dir, t.UTC().Format("2006-01-02")+".jsonl")
}

// Recent returns the last n passes, newest first, from memory.
func (s *Store) Recent(n int) []SuiteResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n > len(s.recent) {
		n = len(s.recent)
	}
	out := make([]SuiteResult, 0, n)
	for i := len(s.recent) - 1; i >= len(s.recent)-n; i-- {
		out = append(out, s.recent[i])
	}
	return out
}

// Latest returns the most recent pass for each network, which is what the
// dashboard opens on.
func (s *Store) Latest() map[string]SuiteResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]SuiteResult{}
	for _, r := range s.recent {
		if existing, ok := out[r.Network]; !ok || r.Start.After(existing.Start) {
			out[r.Network] = r
		}
	}
	return out
}

// Query reads the history back over a window, oldest first. It walks the
// day files rather than holding an index, because the largest a sensor's
// history gets is a few hundred thousand lines.
func (s *Store) Query(from, to time.Time, network string) ([]SuiteResult, error) {
	if s.dir == "" {
		var out []SuiteResult
		for _, r := range s.Recent(0) {
			if inWindow(r, from, to, network) {
				out = append(out, r)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
		return out, nil
	}

	files, err := s.files()
	if err != nil {
		return nil, err
	}
	var out []SuiteResult
	for _, path := range files {
		day, err := dayOf(path)
		if err != nil {
			continue
		}
		// Skip a file that cannot hold anything in the window. The day is in
		// UTC and the window may not be, so a day either side is kept.
		if !to.IsZero() && day.After(to.Add(24*time.Hour)) {
			continue
		}
		if !from.IsZero() && day.Before(from.Add(-24*time.Hour)) {
			continue
		}
		results, err := readResults(path)
		if err != nil {
			return out, err
		}
		for _, r := range results {
			if inWindow(r, from, to, network) {
				out = append(out, r)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

func inWindow(r SuiteResult, from, to time.Time, network string) bool {
	if network != "" && r.Network != network {
		return false
	}
	if !from.IsZero() && r.Start.Before(from) {
		return false
	}
	if !to.IsZero() && r.Start.After(to) {
		return false
	}
	return true
}

func readResults(path string) ([]SuiteResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []SuiteResult
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r SuiteResult
		// A truncated last line — a sensor that lost power mid-write — is
		// skipped rather than failing the whole query.
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, scanner.Err()
}

func (s *Store) files() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, filepath.Join(s.dir, e.Name()))
	}
	sort.Strings(out) // the names are dates, so this is chronological
	return out, nil
}

func dayOf(path string) (time.Time, error) {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return time.ParseInLocation("2006-01-02", base, time.UTC)
}

// Prune enforces both limits: the age of the history and its size. A sensor
// in a cupboard has to be able to run for years without anyone logging in, so
// it can never be allowed to fill its own card.
func (s *Store) Prune() error {
	if s.dir == "" {
		return nil
	}
	files, err := s.files()
	if err != nil {
		return err
	}
	var kept []string
	if s.keep > 0 {
		cutoff := time.Now().Add(-s.keep).UTC().Truncate(24 * time.Hour)
		for _, path := range files {
			day, err := dayOf(path)
			if err != nil {
				continue
			}
			if day.Before(cutoff) {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				continue
			}
			kept = append(kept, path)
		}
	} else {
		kept = files
	}
	if s.maxMiB <= 0 {
		return nil
	}

	limit := int64(s.maxMiB) << 20
	var total int64
	sizes := make(map[string]int64, len(kept))
	for _, path := range kept {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		sizes[path] = info.Size()
		total += info.Size()
	}
	// Oldest first, until it fits.
	for _, path := range kept {
		if total <= limit {
			break
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= sizes[path]
	}
	return nil
}

// Point is one value on a chart.
type Point struct {
	At     time.Time `json:"at"`
	Value  float64   `json:"value"`
	Status Status    `json:"status"`
}

// Series pulls one measurement's history out for a chart: the first
// measurement whose test name matches, per pass.
func (s *Store) Series(network, test string, from, to time.Time) ([]Point, error) {
	results, err := s.Query(from, to, network)
	if err != nil {
		return nil, err
	}
	var out []Point
	for _, r := range results {
		for _, m := range r.Measurements {
			if m.Test != test {
				continue
			}
			out = append(out, Point{At: m.At, Value: m.Value, Status: m.Status})
			break
		}
	}
	return out, nil
}

// ScoreSeries pulls the overall score per pass, which is the chart the
// dashboard opens on.
func (s *Store) ScoreSeries(network string, from, to time.Time) ([]Point, error) {
	results, err := s.Query(from, to, network)
	if err != nil {
		return nil, err
	}
	out := make([]Point, 0, len(results))
	for _, r := range results {
		out = append(out, Point{At: r.Start, Value: float64(r.Overall), Status: r.Status()})
	}
	return out, nil
}
