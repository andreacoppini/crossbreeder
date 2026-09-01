package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func passAt(network string, at time.Time, score int) SuiteResult {
	return SuiteResult{
		Sensor: "lobby-1", Network: network, Kind: "wifi", Start: at, Duration: time.Second,
		Overall: score,
		Measurements: []Measurement{{
			Test: "DHCP", Service: ServiceDHCP, Status: StatusOK, Value: 42, Unit: "ms", At: at,
		}},
	}
}

func TestStoreAppendAndQuery(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(Storage{Dir: dir, Keep: Duration(30 * 24 * time.Hour), MaxMiB: 64})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := s.Append(passAt("Corp", now.Add(-time.Duration(i)*time.Hour), 100-i)); err != nil {
			t.Fatal(err)
		}
	}
	s.Append(passAt("Guest", now, 80))

	all, err := s.Query(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("query returned %d passes", len(all))
	}
	if !all[0].Start.Before(all[len(all)-1].Start) {
		t.Error("results are not oldest first")
	}

	corp, err := s.Query(time.Time{}, time.Time{}, "Corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(corp) != 5 {
		t.Errorf("filtering by network returned %d", len(corp))
	}
	recent, err := s.Query(now.Add(-90*time.Minute), time.Time{}, "Corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Errorf("the window returned %d passes, want 2", len(recent))
	}
}

func TestStoreSurvivesATruncatedLine(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(Storage{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s.Append(passAt("Corp", time.Now().UTC(), 100))

	// A sensor that lost power mid-write leaves half a line behind. The rest
	// of the history has to remain readable.
	path := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"sensor":"lobby-1","network":"Cor`)
	f.Close()

	out, err := s.Query(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("a truncated line cost us the file: %d passes", len(out))
	}
}

func TestStoreRecentAndLatest(t *testing.T) {
	s, _ := NewStore(Storage{}) // no directory: memory only
	now := time.Now()
	s.Append(passAt("Corp", now.Add(-2*time.Minute), 90))
	s.Append(passAt("Corp", now, 70))
	s.Append(passAt("Guest", now.Add(-time.Minute), 50))

	recent := s.Recent(2)
	if len(recent) != 2 || recent[0].Network != "Guest" {
		t.Fatalf("recent = %+v", recent)
	}
	latest := s.Latest()
	if len(latest) != 2 {
		t.Fatalf("latest = %+v", latest)
	}
	if latest["Corp"].Overall != 70 {
		t.Errorf("the newest Corp pass is not the one kept: %d", latest["Corp"].Overall)
	}
}

func TestStorePrunesByAge(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02")+".jsonl")
	os.WriteFile(old, []byte("{}\n"), 0o644)
	fresh := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	os.WriteFile(fresh, []byte("{}\n"), 0o644)

	if _, err := NewStore(Storage{Dir: dir, Keep: Duration(14 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("a 40-day-old file survived a fortnight's retention")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("today's file was pruned")
	}
}

func TestStorePrunesBySize(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 700<<10)
	for i := range big {
		big[i] = '\n'
	}
	for i := 3; i >= 1; i-- {
		name := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02") + ".jsonl"
		os.WriteFile(filepath.Join(dir, name), big, 0o644)
	}
	s, err := NewStore(Storage{Dir: dir, MaxMiB: 1})
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("%d files kept under a 1 MiB cap", len(files))
	}
	// The one kept must be the newest.
	if day, _ := dayOf(files[0]); day.Before(time.Now().UTC().AddDate(0, 0, -2)) {
		t.Errorf("the file kept is %s — the oldest, not the newest", files[0])
	}
}

func TestStoreSeries(t *testing.T) {
	s, _ := NewStore(Storage{})
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Append(passAt("Corp", now.Add(time.Duration(i)*time.Minute), 90+i))
	}
	points, err := s.Series("Corp", "DHCP", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 3 || points[0].Value != 42 {
		t.Fatalf("series = %+v", points)
	}
	scores, err := s.ScoreSeries("Corp", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 3 || scores[2].Value != 92 {
		t.Fatalf("score series = %+v", scores)
	}
}
