package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/gh"
)

// fakeSpawner records the spawn invocation and returns a stubbed exit code/err.
type fakeSpawner struct {
	exitCode int
	err      error

	gotName string
	gotArgs []string
	gotOpts SpawnOpts
}

func (f *fakeSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	f.gotName = name
	f.gotArgs = args
	f.gotOpts = opts
	return f.exitCode, f.err
}

func newJob(t *testing.T) Job {
	dir := t.TempDir()
	return Job{
		ProjectSlug: "auth",
		IssueNumber: 42, IssueTitle: "Login UI", IssueBody: "do it",
		WorktreePath: dir,
		LogPath:      filepath.Join(dir, "issue.log"),
		RepoSlug:     "joshuapeters/klanky",
		Timeout:      time.Second,
	}
}

func TestRunAgent_HappyPath_InReview(t *testing.T) {
	job := newJob(t)
	sp := &fakeSpawner{}
	fake := gh.NewFakeRunner()
	fake.Stub([]string{"git", "-C", job.WorktreePath, "rev-list", "--count", "main..HEAD"},
		[]byte("3\n"), nil)
	fake.Stub([]string{"gh", "pr", "list",
		"--repo", "joshuapeters/klanky",
		"--head", "klanky/auth/issue-42",
		"--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://x","number":99}]`), nil)

	res, err := RunAgent(context.Background(), fake, sp, job)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeInReview || res.PR == nil || res.PR.Number != 99 {
		t.Errorf("got %+v", res)
	}
	if sp.gotName != "claude" {
		t.Errorf("spawned %q, want claude", sp.gotName)
	}
	if len(sp.gotArgs) < 1 || !strings.Contains(sp.gotArgs[1], "issue #42") {
		t.Errorf("envelope arg: %v", sp.gotArgs)
	}
	hasGHRepo := false
	for _, e := range sp.gotOpts.Env {
		if e == "GH_REPO=joshuapeters/klanky" {
			hasGHRepo = true
		}
	}
	if !hasGHRepo {
		t.Errorf("GH_REPO not set in env")
	}
}

func TestRunAgent_NoCommits_NeedsAttention(t *testing.T) {
	job := newJob(t)
	sp := &fakeSpawner{exitCode: 0}
	fake := gh.NewFakeRunner()
	fake.Stub([]string{"git", "-C", job.WorktreePath, "rev-list", "--count", "main..HEAD"},
		[]byte("0\n"), nil)
	res, err := RunAgent(context.Background(), fake, sp, job)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("got %v, want NeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no commits") {
		t.Errorf("reason should mention no commits: %q", res.OutcomeReason)
	}
}

func TestRunAgent_NoPR_NeedsAttention(t *testing.T) {
	job := newJob(t)
	sp := &fakeSpawner{exitCode: 0}
	fake := gh.NewFakeRunner()
	fake.Stub([]string{"git", "-C", job.WorktreePath, "rev-list", "--count", "main..HEAD"},
		[]byte("3\n"), nil)
	fake.Stub([]string{"gh", "pr", "list",
		"--repo", "joshuapeters/klanky",
		"--head", "klanky/auth/issue-42",
		"--state", "open",
		"--json", "url,number"},
		[]byte(`[]`), nil)
	res, err := RunAgent(context.Background(), fake, sp, job)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("got %v", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no open PR") {
		t.Errorf("reason: %q", res.OutcomeReason)
	}
}

func TestRunAgent_Timeout(t *testing.T) {
	job := newJob(t)
	sp := &fakeSpawner{err: context.DeadlineExceeded}
	fake := gh.NewFakeRunner()
	res, err := RunAgent(context.Background(), fake, sp, job)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("got %v", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "timeout") {
		t.Errorf("reason: %q", res.OutcomeReason)
	}
}

func TestRunAgent_SpawnError_Returned(t *testing.T) {
	job := newJob(t)
	sp := &fakeSpawner{err: errors.New("exec: \"claude\": not found")}
	fake := gh.NewFakeRunner()
	_, err := RunAgent(context.Background(), fake, sp, job)
	if err == nil {
		t.Fatal("expected setup error to bubble up")
	}
}
