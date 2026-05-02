package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/snapshot"
)

// Outcome enumerates the final state of an agent run for one issue.
type Outcome int

const (
	OutcomeUnknown Outcome = iota
	OutcomeInReview
	OutcomeNeedsAttention
)

// String is the human-readable form used in the summary table.
func (o Outcome) String() string {
	switch o {
	case OutcomeInReview:
		return "in-review"
	case OutcomeNeedsAttention:
		return "needs-attention"
	default:
		return "unknown"
	}
}

// Job is the per-issue input to RunAgent.
type Job struct {
	ProjectSlug  string
	IssueNumber  int
	IssueTitle   string
	IssueBody    string
	WorktreePath string
	LogPath      string
	RepoSlug     string // owner/name
	Timeout      time.Duration
}

// Result is the per-issue output of RunAgent. Outcome is one of OutcomeInReview
// or OutcomeNeedsAttention; the runner uses this to decide which Status to
// write next.
type Result struct {
	IssueNumber   int
	Outcome       Outcome
	OutcomeReason string // sentence-form explanation; goes into the breadcrumb
	PR            *snapshot.PR
	StartedAt     time.Time
	Duration      time.Duration
}

// SpawnOpts is the per-spawn configuration for a Spawner.
type SpawnOpts struct {
	Cwd    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Spawner abstracts subprocess execution so RunAgent can be tested against a
// FakeSpawner without invoking real `claude`.
type Spawner interface {
	Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (exitCode int, err error)
}

// RealSpawner uses exec.CommandContext with Setpgid so the runner can SIGKILL
// the whole process group on timeout or Ctrl-C.
type RealSpawner struct{}

// Spawn runs the agent. ctx cancellation triggers a SIGKILL to the entire
// process group (so children of children die too).
func (RealSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return os.ErrProcessDone
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err := cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// RunAgent spawns one agent for one issue and verifies its result. Returns a
// non-error Result for both in-review and needs-attention outcomes; only
// setup errors (log dir un-creatable, spawn binary missing) are returned as
// err. The caller writes Status and posts breadcrumbs based on Result.
func RunAgent(ctx context.Context, r gh.Runner, sp Spawner, job Job) (*Result, error) {
	if err := os.MkdirAll(filepath.Dir(job.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile, err := os.Create(job.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create log %s: %w", job.LogPath, err)
	}
	defer logFile.Close()

	envelope := BuildEnvelope(EnvelopeData{
		IssueNumber:  job.IssueNumber,
		IssueTitle:   job.IssueTitle,
		IssueBody:    job.IssueBody,
		ProjectSlug:  job.ProjectSlug,
		WorktreePath: job.WorktreePath,
	})

	args := []string{"-p", envelope, "--permission-mode", "bypassPermissions"}
	env := append(os.Environ(), "GH_REPO="+job.RepoSlug)

	res := &Result{IssueNumber: job.IssueNumber, StartedAt: time.Now()}

	spawnCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	exitCode, spawnErr := sp.Spawn(spawnCtx, "claude", args, SpawnOpts{
		Cwd: job.WorktreePath, Env: env,
		Stdout: logFile, Stderr: logFile,
	})
	res.Duration = time.Since(res.StartedAt)

	if spawnErr != nil && !errors.Is(spawnErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("spawn claude: %w", spawnErr)
	}
	timedOut := errors.Is(spawnErr, context.DeadlineExceeded) || spawnCtx.Err() == context.DeadlineExceeded
	if timedOut {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent timeout after %s", job.Timeout)
		return res, nil
	}

	commitOut, gitErr := r.Run(ctx, "git", "-C", job.WorktreePath, "rev-list", "--count", "main..HEAD")
	if gitErr != nil {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("could not verify branch state: %v", gitErr)
		return res, nil
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(commitOut)))
	if count < 1 {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent exited with code %d but the branch has no commits beyond main", exitCode)
		return res, nil
	}

	branch := snapshot.BranchForIssue(job.ProjectSlug, job.IssueNumber)
	prOut, prErr := r.Run(ctx, "gh", "pr", "list",
		"--repo", job.RepoSlug,
		"--head", branch, "--state", "open",
		"--json", "url,number")
	if prErr != nil {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("could not check for PR: %v", prErr)
		return res, nil
	}
	var prs []struct {
		URL    string `json:"url"`
		Number int    `json:"number"`
	}
	if err := json.Unmarshal(prOut, &prs); err != nil || len(prs) == 0 {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent exited with code %d but no open PR was found on %s", exitCode, branch)
		return res, nil
	}

	res.Outcome = OutcomeInReview
	res.PR = &snapshot.PR{Number: prs[0].Number, URL: prs[0].URL, State: "OPEN", HeadRefName: branch}
	return res, nil
}
