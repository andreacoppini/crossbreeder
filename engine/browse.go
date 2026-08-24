package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// dirEntry is one row in the folder picker.
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// dirListing is what the folder picker shows for one directory.
type dirListing struct {
	Path   string     `json:"path"`
	Parent string     `json:"parent"`
	Dirs   []dirEntry `json:"dirs"`
	Roots  []dirEntry `json:"roots"`
	// Firmware files found here, so the folder that holds the images is
	// recognisable while browsing rather than only after choosing it.
	Firmware []string `json:"firmware"`
}

// browseDir lists a directory for the console's folder picker.
//
// A browser cannot hand back a real filesystem path, and the server is on the
// same machine as the operator, so the picker is served from here instead.
func browseDir(path string) (dirListing, error) {
	if strings.TrimSpace(path) == "" {
		path = workingDir()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return dirListing{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return dirListing{}, fmt.Errorf("cannot open %s: %w", abs, err)
	}

	out := dirListing{Path: abs, Roots: driveRoots()}
	if parent := filepath.Dir(abs); parent != abs {
		out.Parent = parent
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // dotfiles are noise in a firmware folder
		}
		if e.IsDir() {
			out.Dirs = append(out.Dirs, dirEntry{Name: name, Path: filepath.Join(abs, name)})
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".rcks", ".bl7":
			out.Firmware = append(out.Firmware, name)
		}
	}
	sort.Slice(out.Dirs, func(i, j int) bool {
		return strings.ToLower(out.Dirs[i].Name) < strings.ToLower(out.Dirs[j].Name)
	})
	sort.Strings(out.Firmware)
	return out, nil
}

// driveRoots lists the drive letters on Windows, so the picker can leave the
// tree it started in. Elsewhere the filesystem has one root and Parent reaches it.
func driveRoots() []dirEntry {
	if runtime.GOOS != "windows" {
		return nil
	}
	var out []dirEntry
	for c := 'A'; c <= 'Z'; c++ {
		p := string(c) + `:\`
		if _, err := os.Stat(p); err == nil {
			out = append(out, dirEntry{Name: string(c) + ":", Path: p})
		}
	}
	return out
}

// firmwareChoice reports what a firmware push from dir would actually send, so
// "pick automatically" can be shown as a filename instead of a promise.
type firmwareChoice struct {
	Picked     string   `json:"picked"`
	Candidates []string `json:"candidates"`
	Reason     string   `json:"reason"`
	Err        string   `json:"error,omitempty"`
}

func firmwareIn(dir string) firmwareChoice {
	if strings.TrimSpace(dir) == "" {
		dir = workingDir()
	}
	out := firmwareChoice{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		out.Err = fmt.Sprintf("cannot read %s", dir)
		return out
	}
	var rcks, images []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".rcks":
			rcks = append(rcks, e.Name())
		case ".bl7":
			images = append(images, e.Name())
		}
	}
	sort.Strings(rcks)
	sort.Strings(images)
	out.Candidates = append(append([]string{}, rcks...), images...)

	picked, err := pickFirmwareFile(dir)
	if err != nil {
		switch {
		case len(out.Candidates) == 0:
			out.Err = "no .rcks or .bl7 file in this folder"
		default:
			out.Err = "more than one control file here — choose which to push"
		}
		return out
	}
	out.Picked = picked
	if strings.EqualFold(filepath.Ext(picked), ".rcks") {
		out.Reason = "the only control file in this folder"
	} else {
		out.Reason = "the only image in this folder"
	}
	return out
}

// ipChoice is one address the image server could advertise to the APs.
type ipChoice struct {
	IP    string `json:"ip"`
	Label string `json:"label"`
}

// serveIPChoices lists the machine's addresses for the console's dropdown,
// best first, using the same ranking the automatic choice uses.
func serveIPChoices(hosts []string) []ipChoice {
	cands, err := localCandidates()
	if err != nil {
		return nil
	}
	var routeIP string
	if len(hosts) > 0 {
		routeIP = localIPFor(hosts[0])
	}
	for i := range cands {
		for _, t := range hosts {
			if ip := parseIP4(t); ip != nil && cands[i].network.Contains(ip) {
				cands[i].covered++
			}
		}
		cands[i].private = cands[i].ip.IsPrivate()
		cands[i].routePick = cands[i].ip.String() == routeIP
	}
	sortCandidates(cands)

	out := make([]ipChoice, 0, len(cands))
	for _, c := range cands {
		ones, _ := c.network.Mask.Size()
		label := fmt.Sprintf("%s/%d — %s", c.ip, ones, c.iface)
		switch {
		case c.covered > 0:
			label += fmt.Sprintf(" (same subnet as %d AP%s)", c.covered, plural(c.covered))
		case c.private:
			label += " (private)"
		}
		out = append(out, ipChoice{IP: c.ip.String(), Label: label})
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
