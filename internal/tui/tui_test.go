package tui

import (
	"testing"
)

func TestLatencyStats(t *testing.T) {
	s := &latencyStats{}
	if s.count() != 0 {
		t.Error("expected 0 samples initially")
	}
	if s.avg() != 0 {
		t.Error("expected 0 avg initially")
	}

	s.add(100)
	s.add(200)
	s.add(300)

	if s.count() != 3 {
		t.Errorf("expected 3 samples, got %d", s.count())
	}
	if s.avg() != 200 {
		t.Errorf("expected avg 200, got %f", s.avg())
	}
	if s.min() != 100 {
		t.Errorf("expected min 100, got %d", s.min())
	}
	if s.max() != 300 {
		t.Errorf("expected max 300, got %d", s.max())
	}
}

func TestLatencyStatsWindow(t *testing.T) {
	s := &latencyStats{}
	for i := 0; i < 100; i++ {
		s.add(int64(i))
	}
	if s.count() != maxLatencySamples {
		t.Errorf("expected %d samples, got %d", maxLatencySamples, s.count())
	}
	// Oldest values should have been dropped.
	if s.min() != 50 {
		t.Errorf("expected min 50 after window trim, got %d", s.min())
	}
}

func TestVerdictColor(t *testing.T) {
	if verdictColor("approve").String() != "green" {
		t.Error("approve should be green")
	}
	if verdictColor("deny").String() != "red" {
		t.Error("deny should be red")
	}
	if verdictColor("escalate").String() != "yellow" {
		t.Error("escalate should be yellow")
	}
}

func TestVerdictLabel(t *testing.T) {
	if verdictLabel("approve") != "APPROVED" {
		t.Error("expected APPROVED")
	}
	if verdictLabel("deny") != "DENIED" {
		t.Error("expected DENIED")
	}
	if verdictLabel("escalate") != "ESCALATED" {
		t.Error("expected ESCALATED")
	}
	if verdictLabel("error") != "ERROR" {
		t.Error("expected ERROR")
	}
	if verdictLabel("unknown") != "UNKNOWN" {
		t.Error("expected UNKNOWN for unrecognized verdicts")
	}
}

func TestMinMax(t *testing.T) {
	if minOf([]int64{3, 1, 4, 1, 5}) != 1 {
		t.Error("minOf failed")
	}
	if maxOf([]int64{3, 1, 4, 1, 5}) != 5 {
		t.Error("maxOf failed")
	}
	if minOf(nil) != 0 {
		t.Error("min of nil should be 0")
	}
	if maxOf(nil) != 0 {
		t.Error("max of nil should be 0")
	}
}
