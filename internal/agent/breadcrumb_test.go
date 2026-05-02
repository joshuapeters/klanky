package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/gh"
)

func TestBuildBreadcrumb_ContainsAllRequiredSections(t *testing.T) {
	got := BuildBreadcrumb(BreadcrumbData{
		Attempt:      3,
		StartedAt:    time.Date(2026, 4, 26, 9, 32, 17, 0, time.UTC),
		Duration:     15*time.Minute + 54*time.Second,
		Outcome:      "agent exited cleanly but no PR was opened on klanky/feat-7/task-42",
		WorktreePath: "/home/u/.klanky/worktrees/proj/feat-7/task-42",
		LogPath:      ".klanky/logs/task-42.log",
		LastLogLines: []string{"line A", "line B", "line C"},
	})

	wantSubstrs := []string{
		"<!-- klanky-attempt -->",
		"Klanky attempt #3 — needs-attention",
		"2026-04-26",
		"15m54s",
		"agent exited cleanly",
		"/home/u/.klanky/worktrees/proj/feat-7/task-42",
		".klanky/logs/task-42.log",
		"line A",
		"line C",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(got, want) {
			t.Errorf("breadcrumb missing %q\n---\n%s", want, got)
		}
	}
}

func TestCountPriorAttempts_ZeroComments(t *testing.T) {
	r := gh.NewFakeRunner()
	r.Stub([]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "comments"},
		[]byte(`{"comments":[]}`), nil)

	n, err := CountPriorAttempts(context.Background(), r, "alice/proj", 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestCountPriorAttempts_OnlyKlankySentinelCommentsCount(t *testing.T) {
	r := gh.NewFakeRunner()
	r.Stub([]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "comments"},
		[]byte(`{"comments":[
			{"body":"<!-- klanky-attempt -->\n**Klanky attempt #1...**"},
			{"body":"some user comment"},
			{"body":"<!-- klanky-attempt -->\n**Klanky attempt #2...**"}
		]}`), nil)

	n, err := CountPriorAttempts(context.Background(), r, "alice/proj", 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

func TestPostBreadcrumb_CallsGhIssueComment(t *testing.T) {
	r := gh.NewFakeRunner()
	r.Stub([]string{"gh", "issue", "comment", "42", "--repo", "alice/proj", "--body", "hello"}, nil, nil)

	if err := PostBreadcrumb(context.Background(), r, "alice/proj", 42, "hello"); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
}
