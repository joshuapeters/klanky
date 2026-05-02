package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/gh"
)

func TestBuildBreadcrumb_ContainsAttemptSentinelAndBody(t *testing.T) {
	got := BuildBreadcrumb(BreadcrumbData{
		Attempt:      2,
		StartedAt:    time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC),
		Duration:     15*time.Minute + 54*time.Second,
		Outcome:      "agent timeout after 20m",
		WorktreePath: "/wt/issue-7",
		LogPath:      "/log",
		LastLogLines: []string{"line A", "line B"},
	})
	for _, w := range []string{
		AttemptSentinel,
		"attempt #2",
		"agent timeout after 20m",
		"`/wt/issue-7`",
		"line A",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("breadcrumb missing %q:\n%s", w, got)
		}
	}
}

func TestBuildReconcileBreadcrumb_HasSentinel(t *testing.T) {
	got := BuildReconcileBreadcrumb("PR was closed.")
	if !strings.HasPrefix(got, ReconcileSentinel) {
		t.Errorf("reconcile breadcrumb missing sentinel: %q", got)
	}
	if !strings.Contains(got, "PR was closed.") {
		t.Errorf("reconcile breadcrumb missing body")
	}
}

func TestCountPriorAttempts(t *testing.T) {
	fake := gh.NewFakeRunner()
	body := `{"comments":[
		{"body":"<!-- klanky-attempt -->\n**Klanky attempt #1**"},
		{"body":"random comment"},
		{"body":"<!-- klanky-attempt -->\n**Klanky attempt #2**"},
		{"body":"<!-- klanky-reconcile -->\nreconcile note"}
	]}`
	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "o/r", "--json", "comments"},
		[]byte(body), nil,
	)
	got, err := CountPriorAttempts(context.Background(), fake, "o/r", 42)
	if err != nil {
		t.Fatalf("CountPriorAttempts: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestTailLines(t *testing.T) {
	in := "a\nb\nc\nd\ne\n\n"
	got := TailLines(in, 3)
	if len(got) != 3 || got[0] != "c" || got[2] != "e" {
		t.Errorf("TailLines = %v", got)
	}
	got = TailLines("only one", 5)
	if len(got) != 1 || got[0] != "only one" {
		t.Errorf("TailLines short = %v", got)
	}
}
