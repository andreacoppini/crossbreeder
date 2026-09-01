package main

import (
	"math"
	"time"
)

// Scoring turns a pass into numbers an operator can watch move. The rule is
// deliberately blunt — a pass is 100, a slow pass is 65, a failure is 0 —
// because a score whose arithmetic nobody can follow gets ignored the first
// time it disagrees with somebody's experience.
const (
	scoreOK   = 100
	scoreWarn = 65
	scoreFail = 0
)

// serviceWeights say how much each layer matters to the overall figure. The
// lower layers weigh more: a site whose DHCP is failing is in worse trouble
// than one whose Dropbox is slow, even though both are one red measurement.
var serviceWeights = map[Service]float64{
	ServiceWireless:     1.5,
	ServiceAuth:         1.5,
	ServiceDHCP:         1.3,
	ServiceGateway:      1.2,
	ServiceDNS:          1.2,
	ServiceInternet:     1.0,
	ServiceApplications: 1.0,
	ServiceVoice:        0.8,
	ServiceThroughput:   0.8,
	ServiceLAN:          0.5,
}

// Score reduces a pass to a score per service and one overall. Skipped
// measurements are left out entirely: a test that never ran because the layer
// beneath it was down must not be counted as either a pass or a failure.
func Score(measurements []Measurement) (map[Service]int, int) {
	sums := map[Service]float64{}
	counts := map[Service]float64{}
	for _, m := range measurements {
		if m.Status == StatusSkipped {
			continue
		}
		var v float64
		switch m.Status {
		case StatusOK:
			v = scoreOK
		case StatusWarn:
			v = scoreWarn
		case StatusFail:
			v = scoreFail
		}
		sums[m.Service] += v
		counts[m.Service]++
	}

	scores := make(map[Service]int, len(sums))
	var weighted, weight float64
	for service, sum := range sums {
		score := sum / counts[service]
		scores[service] = int(math.Round(score))
		w := serviceWeights[service]
		if w == 0 {
			w = 1
		}
		weighted += score * w
		weight += w
	}
	if weight == 0 {
		return scores, 0
	}
	return scores, int(math.Round(weighted / weight))
}

// Health is the word a score is shown as. The bands are the ones people
// already use for a network: fine, noticeable, and someone is on the phone.
func Health(score int) string {
	switch {
	case score >= 90:
		return "good"
	case score >= 70:
		return "fair"
	case score > 0:
		return "poor"
	}
	return "down"
}

// judgeDuration grades a timing against a warn and a fail line.
func judgeDuration(d, warn, fail time.Duration) Status {
	switch {
	case fail > 0 && d >= fail:
		return StatusFail
	case warn > 0 && d >= warn:
		return StatusWarn
	}
	return StatusOK
}

// judgeAtLeast grades a value that should be high — a signal in dBm, a MOS, a
// rate in Mbps.
func judgeAtLeast(v, warn, fail float64) Status {
	switch {
	case v <= fail:
		return StatusFail
	case v <= warn:
		return StatusWarn
	}
	return StatusOK
}

// judgeAtMost grades a value that should be low — loss, air-time use.
func judgeAtMost(v, warn, fail float64) Status {
	switch {
	case v >= fail:
		return StatusFail
	case v >= warn:
		return StatusWarn
	}
	return StatusOK
}

// worst reduces a set of statuses to the one that matters.
func worst(statuses ...Status) Status {
	out := StatusOK
	for _, s := range statuses {
		if s.Worse(out) {
			out = s
		}
	}
	return out
}
