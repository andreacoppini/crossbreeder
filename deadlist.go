package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
)

// maxDeadLines caps the reported ranges unless -v is given. A list this long is
// already telling you something structural, and the full set is in the results
// file either way.
const maxDeadLines = 40

// reportDead prints the addresses that did not answer the sweep.
//
// Consecutive addresses are collapsed into ranges, which is what makes the
// output useful rather than just long: a dead /26 shows up as one line instead
// of sixty, and a scattered handful stays a scattered handful.
func reportDead(w *os.File, dead []string, mode string, verbose bool) {
	if len(dead) == 0 {
		return
	}
	label := "did not answer"
	if mode == "icmp" {
		label = "did not answer ping"
	}
	fmt.Fprintf(w, "%d %s:\n", len(dead), label)

	lines := collapseRanges(dead)
	shown := lines
	if !verbose && len(lines) > maxDeadLines {
		shown = lines[:maxDeadLines]
	}
	for _, l := range shown {
		fmt.Fprintf(w, "  %s\n", l)
	}
	if len(shown) < len(lines) {
		fmt.Fprintf(w, "  ... and %d more (-v for the full list, or -dead <file>)\n", len(lines)-len(shown))
	}
	fmt.Fprintln(w)
}

// collapseRanges sorts addresses numerically and folds runs of consecutive ones
// into "first - last (count)".
func collapseRanges(hosts []string) []string {
	nums := make([]uint32, 0, len(hosts))
	other := make([]string, 0)
	for _, h := range hosts {
		if v4 := net.ParseIP(h).To4(); v4 != nil {
			nums = append(nums, uint32(v4[0])<<24|uint32(v4[1])<<16|uint32(v4[2])<<8|uint32(v4[3]))
			continue
		}
		other = append(other, h) // hostnames and IPv6 cannot be ranged
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })

	var out []string
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		if i == j {
			out = append(out, ipString(nums[i]))
		} else {
			out = append(out, fmt.Sprintf("%-15s - %-15s (%d)", ipString(nums[i]), ipString(nums[j]), j-i+1))
		}
		i = j + 1
	}
	sort.Strings(other)
	return append(out, other...)
}

func ipString(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
}

// writeDeadList writes the silent addresses one per line, so the file can be
// fed straight back in as -csv once the cabling or power is sorted out.
func writeDeadList(path string, dead []string) error {
	if path == "" || len(dead) == 0 {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sorted := make([]string, len(dead))
	copy(sorted, dead)
	sort.Slice(sorted, func(i, j int) bool { return ipLess(sorted[i], sorted[j]) })
	return writeLines(f, sorted)
}

func writeLines(f *os.File, lines []string) error {
	_, err := f.WriteString(strings.Join(lines, "\n") + "\n")
	return err
}

func ipLess(a, b string) bool {
	x, y := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if x == nil || y == nil {
		return a < b
	}
	return string(x) < string(y)
}
