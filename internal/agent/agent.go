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

// Outcome enumerates the final state of an agent run for one task.
type Outcome int

const (
	OutcomeUnknown Outcome = iota
	OutcomeInReview
	OutcomeNeedsAttention
)

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

// AgentJob is the per-task input to RunAgent.
type AgentJob struct {
	FeatureID    int
	Task         snapshot.TaskInfo
	WorktreePath string
	LogPath      string
	RepoSlug     string // owner/name
	Timeout      time.Duration
}

// TaskResult is the per-task output of RunAgent, consumed by the runner for
// status writes, summary rendering, and breadcrumb composition.
type TaskResult struct {
	TaskNumber    int
	Outcome       Outcome
	OutcomeReason string // freeform sentence describing why (used in breadcrumb)
	PR            *snapshot.PRInfo
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

// Spawner abstracts subprocess execution for long-running agents.
// Implementations are expected to honor ctx cancellation and return
// (-1, ctx.Err()) on timeout, with the process group SIGKILLed.
type Spawner interface {
	Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (exitCode int, err error)
}

// RealSpawner uses exec.CommandContext and sets the process group so the
// runner can SIGKILL the whole tree on timeout.
type RealSpawner struct{}

func (RealSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the whole process group rather than just the leader.
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

// RunAgent spawns one agent for one task and verifies its result.
// Returns a non-error TaskResult for both success (in-review) and failure
// (needs-attention) outcomes — only setup errors (e.g. claude binary missing,
// log file un-creatable) are returned as err.
func RunAgent(ctx context.Context, r gh.Runner, sp Spawner, job AgentJob) (*TaskResult, error) {
	if err := os.MkdirAll(filepath.Dir(job.LogPath), 0755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile, err := os.Create(job.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create log %s: %w", job.LogPath, err)
	}
	defer logFile.Close()

	envelope := BuildEnvelope(EnvelopeData{
		FeatureID:    job.FeatureID,
		TaskNumber:   job.Task.Number,
		TaskTitle:    job.Task.Title,
		TaskBody:     job.Task.Body,
		WorktreePath: job.WorktreePath,
	})

	args := []string{"-p", envelope, "--permission-mode", "bypassPermissions"}
	env := append(os.Environ(), "GH_REPO="+job.RepoSlug)

	res := &TaskResult{TaskNumber: job.Task.Number, StartedAt: time.Now()}

	spawnCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	exitCode, spawnErr := sp.Spawn(spawnCtx, "claude", args, SpawnOpts{
		Cwd: job.WorktreePath, Env: env,
		Stdout: logFile, Stderr: logFile,
	})
	res.Duration = time.Since(res.StartedAt)

	// A spawn-system error (binary missing, exec failure) is fatal for this task
	// and should be returned to the runner so it's surfaced rather than silently
	// becoming needs-attention.
	if spawnErr != nil && !errors.Is(spawnErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("spawn claude: %w", spawnErr)
	}

	timedOut := errors.Is(spawnErr, context.DeadlineExceeded) || spawnCtx.Err() == context.DeadlineExceeded
	if timedOut {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent timeout after %s", job.Timeout)
		return res, nil
	}

	// Verify branch has commits beyond main.
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

	// Verify open PR exists for the branch.
	branch := snapshot.BranchForTask(job.FeatureID, job.Task.Number)
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
		res.OutcomeReason = fmt.Sprintf("agent exited with code %d but no PR (open) was found on %s", exitCode, branch)
		return res, nil
	}

	res.Outcome = OutcomeInReview
	res.PR = &snapshot.PRInfo{Number: prs[0].Number, URL: prs[0].URL, State: "OPEN", HeadRefName: branch}
	return res, nil
}
