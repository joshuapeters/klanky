package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FakeSpawner records the subprocess invocations and returns scripted exit codes.
type FakeSpawner struct {
	Calls    []FakeSpawnCall
	exitCode int
	err      error
	stdout   string
	stderr   string
}

type FakeSpawnCall struct {
	Name string
	Args []string
	Cwd  string
	Env  []string
}

func (f *FakeSpawner) Stub(exitCode int, stdout, stderr string, err error) {
	f.exitCode = exitCode
	f.stdout = stdout
	f.stderr = stderr
	f.err = err
}

func (f *FakeSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	f.Calls = append(f.Calls, FakeSpawnCall{Name: name, Args: args, Cwd: opts.Cwd, Env: opts.Env})
	if opts.Stdout != nil && f.stdout != "" {
		opts.Stdout.Write([]byte(f.stdout))
	}
	if opts.Stderr != nil && f.stderr != "" {
		opts.Stderr.Write([]byte(f.stderr))
	}
	return f.exitCode, f.err
}

func TestRunAgent_HappyPath_ReturnsInReview(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "task-42.log")

	sp := &FakeSpawner{}
	sp.Stub(0, "did stuff\n", "", nil)

	r := NewFakeRunner()
	// Branch verification: at least one commit beyond main.
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("2\n"), nil)
	// PR verification.
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://github.com/alice/proj/pull/77","number":77}]`), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID:    7,
		Task:         TaskInfo{Number: 42, Title: "T", Body: "..."},
		WorktreePath: "/wt",
		LogPath:      logPath,
		RepoSlug:     "alice/proj",
		Timeout:      20 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeInReview {
		t.Errorf("Outcome = %v, want OutcomeInReview", res.Outcome)
	}
	if res.PR == nil || res.PR.Number != 77 {
		t.Errorf("PR = %+v, want number=77", res.PR)
	}

	// Spawn invocation should pass the envelope and the right flags.
	if len(sp.Calls) != 1 {
		t.Fatalf("expected 1 spawn call, got %d", len(sp.Calls))
	}
	call := sp.Calls[0]
	if call.Name != "claude" {
		t.Errorf("Name = %q, want claude", call.Name)
	}
	wantArgs := []string{"-p", "", "--permission-mode", "bypassPermissions"}
	if len(call.Args) != len(wantArgs) {
		t.Fatalf("Args length = %d, want %d", len(call.Args), len(wantArgs))
	}
	if call.Args[0] != "-p" {
		t.Errorf("Args[0] = %q, want -p", call.Args[0])
	}
	if !strings.Contains(call.Args[1], "task #42") {
		t.Errorf("Args[1] should contain envelope; got: %s", call.Args[1])
	}
	if call.Args[2] != "--permission-mode" || call.Args[3] != "bypassPermissions" {
		t.Errorf("permission flags wrong: %v", call.Args)
	}

	// GH_REPO must be in env.
	foundEnv := false
	for _, e := range call.Env {
		if e == "GH_REPO=alice/proj" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("env missing GH_REPO=alice/proj: %v", call.Env)
	}

	// Log file should contain spawn stdout.
	logged, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logged), "did stuff") {
		t.Errorf("log file missing spawn stdout; got: %s", logged)
	}
}

func TestRunAgent_NoCommits_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("0\n"), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no commits") {
		t.Errorf("OutcomeReason = %q, want to mention no commits", res.OutcomeReason)
	}
}

func TestRunAgent_NoPR_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("3\n"), nil)
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[]`), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no PR") {
		t.Errorf("OutcomeReason = %q, want to mention no PR", res.OutcomeReason)
	}
}

func TestRunAgent_TimeoutKilled_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(-1, "", "", context.DeadlineExceeded)

	r := NewFakeRunner()

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunAgent should not propagate timeout as error: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "timeout") {
		t.Errorf("OutcomeReason = %q, want to mention timeout", res.OutcomeReason)
	}
}

func TestRunAgent_SpawnError_PropagatesAsError(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(-1, "", "", errors.New("exec: \"claude\": executable file not found in $PATH"))

	res, err := RunAgent(context.Background(), NewFakeRunner(), sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err == nil {
		t.Fatalf("expected error from spawn failure, got result %+v", res)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention claude: %v", err)
	}
}

// Smoke check: log buffer is wired before spawn.
func TestRunAgent_LogFile_CreatedEvenOnEarlyExit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "task-42.log")
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("0\n"), nil)

	_, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: logPath, RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

// Sanity check on bytes.Buffer assertion shape — ensures stdout writer wiring works.
var _ = bytes.NewBuffer
