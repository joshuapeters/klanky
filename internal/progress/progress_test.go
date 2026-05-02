package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProgress_ReconcileLine(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 15, 0, time.Local)))

	p.Reconciled(4, "set #41 → Done (issue closed)")

	got := buf.String()
	if !strings.Contains(got, "[09:32:15]") {
		t.Errorf("missing timestamp; got: %s", got)
	}
	if !strings.Contains(got, "reconcile: 4 tasks scanned") {
		t.Errorf("missing reconcile body; got: %s", got)
	}
	if !strings.Contains(got, "set #41 → Done") {
		t.Errorf("missing per-task transition; got: %s", got)
	}
}

func TestProgress_PhaseSummary(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 16, 0, time.Local)))

	p.PhaseSelected(2, 3, 2, 0)

	got := buf.String()
	if !strings.Contains(got, "phase 2") {
		t.Errorf("missing phase number; got: %s", got)
	}
	if !strings.Contains(got, "5 tasks ready (3 todo, 2 needs-attention)") {
		t.Errorf("missing eligibility breakdown; got: %s", got)
	}
}

func TestProgress_TaskTransitions(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 17, 0, time.Local)))

	p.TaskInProgress(42)
	p.TaskInReview(44, 88)
	p.TaskNeedsAttention(42, 3)

	got := buf.String()
	for _, want := range []string{
		"task #42 → in-progress",
		"task #44 → in-review (PR #88)",
		"task #42 → needs-attention (3rd attempt)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestProgress_OrdinalSuffixes(t *testing.T) {
	cases := map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd"}
	for n, want := range cases {
		got := ordinal(n)
		if got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

// fixedClock returns a clock function that always returns the given time.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }
