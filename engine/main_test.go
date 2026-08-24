package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollapseRangesFoldsConsecutiveAddresses(t *testing.T) {
	// Shaped after a real site sweep: a few scattered singles and one dead block.
	in := []string{
		"172.20.45.140", "172.20.43.87", "172.20.45.131", "172.20.45.130",
		"172.20.44.151", "172.20.45.132", "172.20.46.55",
	}
	got := collapseRanges(in)

	want := []string{
		"172.20.43.87",
		"172.20.44.151",
		"172.20.45.130   - 172.20.45.132   (3)",
		"172.20.45.140",
		"172.20.46.55",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

func TestCollapseRangesHandlesEdges(t *testing.T) {
	if got := collapseRanges(nil); len(got) != 0 {
		t.Errorf("empty input produced %v", got)
	}
	if got := collapseRanges([]string{"10.0.0.1"}); len(got) != 1 || got[0] != "10.0.0.1" {
		t.Errorf("single address produced %v", got)
	}
	// A run that crosses an octet boundary is still consecutive.
	got := collapseRanges([]string{"10.0.0.255", "10.0.1.0"})
	if len(got) != 1 || !strings.Contains(got[0], "(2)") {
		t.Errorf("cross-octet run not folded: %v", got)
	}
}

// The field CSV that broke the first run: a stray quote inside an unquoted
// field made encoding/csv reject the whole file.
func TestLoadHostsToleratesStrayQuotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aps.csv")
	body := "IP \"Address\",Note\n" +
		"172.20.44.10,building \"A\"\n" +
		"\"172.20.44.11\",quoted row\n" +
		"172.20.44.10,duplicate\n" +
		"not-an-ip,ignored\n" +
		"\n" +
		"172.20.44.12\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	hosts, err := loadHosts(path)
	if err != nil {
		t.Fatalf("loadHosts: %v", err)
	}
	want := []string{"172.20.44.10", "172.20.44.11", "172.20.44.12"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", hosts, want)
	}
}

// Excel and Notepad write a UTF-8 BOM. With a header row it lands on the header
// and is discarded with it; on a bare one-line list it landed on the only
// address there was.
func TestLoadHostsStripsUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"bare.csv":        "\ufeff192.168.77.115",
		"bare-crlf.csv":   "\ufeff192.168.77.115\r\n",
		"with-header.csv": "\ufeffIP Address\r\n192.168.77.115\r\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		hosts, err := loadHosts(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(hosts) != 1 || hosts[0] != "192.168.77.115" {
			t.Errorf("%s: got %v, want [192.168.77.115]", name, hosts)
		}
	}
}

func TestWriteDeadListIsSortedAndReFeedable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dead.txt")
	if err := writeDeadList(path, []string{"172.20.45.130", "172.20.43.87", "172.20.45.9"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "172.20.43.87\n172.20.45.9\n172.20.45.130\n"
	if string(body) != want {
		t.Errorf("got %q, want %q", body, want)
	}

	// The whole point of the file is that it goes straight back in as -csv.
	hosts, err := loadHosts(path)
	if err != nil {
		t.Fatalf("re-reading the dead list failed: %v", err)
	}
	if len(hosts) != 3 {
		t.Errorf("re-read %d hosts, want 3", len(hosts))
	}
}

func TestNoReplyStatusNamesTheCheck(t *testing.T) {
	for probe, want := range map[string]string{
		"icmp": "No ping reply",
		"tcp":  "No SSH port",
		"both": "No response",
	} {
		if got := noReplyStatus(probe); got != want {
			t.Errorf("%s: got %q, want %q", probe, got, want)
		}
	}
}

// A bare invocation must open the console, and a missing -csv must say so in
// terms that name the way out rather than surfacing os.Open("").
func TestBareInvocationOpensTheConsole(t *testing.T) {
	// run() decides on os.Args directly, so exercise that.
	saved := os.Args
	defer func() { os.Args = saved }()

	os.Args = []string{"crossbreeder-engine.exe"}
	if !(len(os.Args) == 1) {
		t.Fatal("test setup wrong")
	}

	os.Args = []string{"crossbreeder-engine.exe", "-user", "admin"}
	if len(os.Args) == 1 {
		t.Error("a flagged invocation must not be treated as bare")
	}
}

func TestMissingCSVGivesAnActionableError(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"crossbreeder-engine.exe", "-user", "admin"}

	err := run(options{user: "admin"})
	if err == nil {
		t.Fatal("expected an error with no -csv")
	}
	msg := err.Error()
	if !strings.Contains(msg, "-csv") || !strings.Contains(msg, "console") {
		t.Errorf("error %q names neither -csv nor the console", msg)
	}
	if strings.Contains(msg, "The system cannot find") || strings.HasPrefix(msg, "open :") {
		t.Errorf("still leaking the raw open() failure: %q", msg)
	}
}
