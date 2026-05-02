package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderSummary_AllInReview(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase:    2,
		Duration: 18*time.Minute + 43*time.Second,
		Rows: []SummaryRow{
			{Task: 43, Status: "in-review", Link: "https://github.com/o/r/pull/89"},
			{Task: 44, Status: "in-review", Link: "https://github.com/o/r/pull/88"},
		},
	}, buf)

	got := buf.String()
	for _, want := range []string{
		"Phase 2 run complete in 18m43s",
		"#43",
		"https://github.com/o/r/pull/89",
		"2 tasks attempted: 2 in-review, 0 needs-attention",
		"review the 2 PRs above",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSummary_MixedOutcomes(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase:    2,
		Duration: 5 * time.Minute,
		Rows: []SummaryRow{
			{Task: 42, Status: "needs-attention", Link: "https://github.com/o/r/issues/42", Note: "3rd attempt"},
			{Task: 43, Status: "in-review", Link: "https://github.com/o/r/pull/89"},
		},
	}, buf)

	got := buf.String()
	if !strings.Contains(got, "1 in-review, 1 needs-attention") {
		t.Errorf("counts wrong:\n%s", got)
	}
	if !strings.Contains(got, "address") {
		t.Errorf("missing address-needs-attention note:\n%s", got)
	}
}

func TestRenderSummary_FeatureComplete(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{FeatureComplete: true, FeatureNumber: 7, TotalTasks: 12}, buf)

	got := buf.String()
	for _, want := range []string{
		"Feature #7 is complete",
		"all 12 tasks closed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSummary_OnlyAwaitingReview(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase: 2,
		AwaitingReviewLinks: []string{
			"#42 https://github.com/o/r/pull/77",
			"#43 https://github.com/o/r/pull/78",
		},
	}, buf)

	got := buf.String()
	for _, want := range []string{
		"Phase 2 has 2 PRs awaiting your review",
		"https://github.com/o/r/pull/77",
		"Merge or close them",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
