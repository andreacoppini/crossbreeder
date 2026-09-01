package main

import (
	"testing"
	"time"
)

func TestScoreIgnoresSkippedTests(t *testing.T) {
	ms := []Measurement{
		{Service: ServiceDNS, Status: StatusOK},
		{Service: ServiceDNS, Status: StatusOK},
		// A test that never ran because the layer beneath it was down says
		// nothing about DNS, and must not be counted either way.
		{Service: ServiceApplications, Status: StatusSkipped},
	}
	scores, overall := Score(ms)
	if scores[ServiceDNS] != 100 {
		t.Errorf("DNS = %d", scores[ServiceDNS])
	}
	if _, ok := scores[ServiceApplications]; ok {
		t.Error("a skipped service was scored")
	}
	if overall != 100 {
		t.Errorf("overall = %d", overall)
	}
}

func TestScoreWeighsTheLowerLayersMore(t *testing.T) {
	// One failure at the bottom of the stack against one at the top: the same
	// count of red, but not the same situation.
	deep, _ := Score([]Measurement{
		{Service: ServiceDHCP, Status: StatusFail},
		{Service: ServiceApplications, Status: StatusOK},
	})
	shallow, _ := Score([]Measurement{
		{Service: ServiceDHCP, Status: StatusOK},
		{Service: ServiceApplications, Status: StatusFail},
	})
	_, deepOverall := Score([]Measurement{
		{Service: ServiceDHCP, Status: StatusFail},
		{Service: ServiceApplications, Status: StatusOK},
	})
	_, shallowOverall := Score([]Measurement{
		{Service: ServiceDHCP, Status: StatusOK},
		{Service: ServiceApplications, Status: StatusFail},
	})
	if deepOverall >= shallowOverall {
		t.Errorf("a DHCP failure (%d) scored no worse than an application failure (%d)",
			deepOverall, shallowOverall)
	}
	if deep[ServiceDHCP] != 0 || shallow[ServiceApplications] != 0 {
		t.Error("a failing service did not score zero")
	}
}

func TestScoreAveragesWithinAService(t *testing.T) {
	scores, _ := Score([]Measurement{
		{Service: ServiceApplications, Status: StatusOK},
		{Service: ServiceApplications, Status: StatusFail},
	})
	if scores[ServiceApplications] != 50 {
		t.Errorf("one of two applications failing scored %d, want 50", scores[ServiceApplications])
	}
}

func TestHealthBands(t *testing.T) {
	cases := map[int]string{100: "good", 90: "good", 89: "fair", 70: "fair", 69: "poor", 1: "poor", 0: "down"}
	for score, want := range cases {
		if got := Health(score); got != want {
			t.Errorf("Health(%d) = %q, want %q", score, got, want)
		}
	}
}

func TestJudgements(t *testing.T) {
	warn, fail := 100*time.Millisecond, time.Second
	if judgeDuration(50*time.Millisecond, warn, fail) != StatusOK {
		t.Error("a fast answer was not OK")
	}
	if judgeDuration(200*time.Millisecond, warn, fail) != StatusWarn {
		t.Error("a slow answer was not a warning")
	}
	if judgeDuration(2*time.Second, warn, fail) != StatusFail {
		t.Error("an answer past the fail line was not a failure")
	}

	// Signal is the other way round: bigger is better, and it is negative.
	if judgeAtLeast(-55, -70, -80) != StatusOK {
		t.Error("-55 dBm was not judged healthy")
	}
	if judgeAtLeast(-75, -70, -80) != StatusWarn {
		t.Error("-75 dBm was not a warning")
	}
	if judgeAtLeast(-85, -70, -80) != StatusFail {
		t.Error("-85 dBm was not a failure")
	}
	if judgeAtMost(80, 50, 75) != StatusFail {
		t.Error("80% air time in use was not a failure")
	}
	if worst(StatusOK, StatusWarn, StatusFail, StatusSkipped) != StatusFail {
		t.Error("worst() did not pick the failure")
	}
	if !StatusWarn.Worse(StatusOK) || StatusSkipped.Worse(StatusOK) {
		t.Error("the status ordering is wrong")
	}
}
