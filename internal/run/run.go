package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/lock"
	"github.com/joshuapeters/klanky/internal/progress"
	"github.com/joshuapeters/klanky/internal/reconcile"
	"github.com/joshuapeters/klanky/internal/snapshot"
	"github.com/joshuapeters/klanky/internal/statuswrite"
	"github.com/joshuapeters/klanky/internal/summary"
	"github.com/joshuapeters/klanky/internal/workqueue"
	"github.com/joshuapeters/klanky/internal/worktree"
)

const concurrencyLimit = 5

// Deps bundles the dependencies of Feature so the call signature stays
// manageable as features are added.
type Deps struct {
	Runner       gh.Runner
	Spawner      agent.Spawner
	Config       *config.Config
	RepoRoot     string
	FeatureID    int
	WorktreeRoot string // typically ~/.klanky/worktrees
	Progress     *progress.Progress
	SummaryOut   io.Writer
	Timeout      time.Duration // per-task agent timeout
}

// Feature is the top-level orchestrator for `klanky run <feature-id>`.
// Sequence: lock → fetch snapshot → reconcile (apply) → select work →
// spawn agents in parallel → render summary → release lock.
func Feature(ctx context.Context, d Deps) error {
	// 1. Lock.
	lockPath := filepath.Join(d.RepoRoot, ".klanky", fmt.Sprintf("runner-%d.lock", d.FeatureID))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("mkdir .klanky: %w", err)
	}
	lk, err := lock.AcquireLock(lockPath)
	if err != nil {
		return err
	}
	defer lk.Release()

	// 2. Fetch snapshot.
	snap, err := snapshot.FetchSnapshot(ctx, d.Runner, d.Config, d.FeatureID)
	if err != nil {
		return err
	}

	// 3. Reconcile: compute and apply actions, mutate snapshot in-memory so
	// SelectWork sees the post-reconcile state.
	actions := reconcile.Reconcile(snap, d.FeatureID)
	reconcileSummary := applyReconcile(ctx, d.Runner, d.Config, snap, actions, d.FeatureID)
	d.Progress.Reconciled(len(snap.Tasks), reconcileSummary)

	// 4. Pick work.
	wq := workqueue.SelectWork(snap)

	// 5. Handle nothing-to-do scenarios.
	if wq.AllDone {
		summary.RenderSummary(summary.SummaryData{
			FeatureComplete: true,
			FeatureNumber:   d.FeatureID,
			TotalTasks:      len(snap.Tasks),
		}, d.SummaryOut)
		return nil
	}
	if len(wq.SurvivingInProgress) > 0 {
		return fmt.Errorf("phase %d has %d tasks in unexpected in-progress state — this is a bug; see logs",
			wq.CurrentPhase, len(wq.SurvivingInProgress))
	}
	if len(wq.Eligible) == 0 {
		// Only awaiting-review tasks in this phase.
		links := make([]string, 0, len(wq.AwaitingReview))
		for _, t := range wq.AwaitingReview {
			pr, ok := snap.PRsByBranch[snapshot.BranchForTask(d.FeatureID, t.Number)]
			if ok {
				links = append(links, fmt.Sprintf("#%d %s", t.Number, pr.URL))
			} else {
				links = append(links, fmt.Sprintf("#%d (no PR found)", t.Number))
			}
		}
		summary.RenderSummary(summary.SummaryData{
			Phase:               wq.CurrentPhase,
			AwaitingReviewLinks: links,
		}, d.SummaryOut)
		return nil
	}

	d.Progress.PhaseSelected(
		wq.CurrentPhase,
		countByStatus(wq.Eligible, "Todo")+countByStatus(wq.Eligible, ""),
		countByStatus(wq.Eligible, "Needs Attention"),
		len(wq.AwaitingReview),
	)

	// 6. Spawn agents in parallel, capped at 5.
	eg, gctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(concurrencyLimit)
	results := make([]agent.TaskResult, len(wq.Eligible))
	var resultsMu sync.Mutex
	startedAt := time.Now()

	repoSlug := d.Config.Repo.Owner + "/" + d.Config.Repo.Name

	for i, task := range wq.Eligible {
		i, task := i, task
		eg.Go(func() error {
			if err := sem.Acquire(gctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			res := runOneTask(gctx, d, snap, task, repoSlug)
			resultsMu.Lock()
			results[i] = res
			resultsMu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// 7. Render summary.
	rows := make([]summary.SummaryRow, 0, len(results))
	for _, r := range results {
		row := summary.SummaryRow{Task: r.TaskNumber, Status: r.Outcome.String()}
		switch r.Outcome {
		case agent.OutcomeInReview:
			if r.PR != nil {
				row.Link = r.PR.URL
			}
		case agent.OutcomeNeedsAttention:
			row.Link = fmt.Sprintf("https://github.com/%s/issues/%d", repoSlug, r.TaskNumber)
		}
		rows = append(rows, row)
	}
	summary.RenderSummary(summary.SummaryData{
		Phase: wq.CurrentPhase, Duration: time.Since(startedAt), Rows: rows,
	}, d.SummaryOut)

	return nil
}

func runOneTask(ctx context.Context, d Deps, snap *snapshot.Snapshot, task snapshot.TaskInfo, repoSlug string) agent.TaskResult {
	wtPath := worktree.WorktreePath(d.WorktreeRoot, d.Config.Repo.Name, d.FeatureID, task.Number)
	branch := snapshot.BranchForTask(d.FeatureID, task.Number)
	logPath := filepath.Join(d.RepoRoot, ".klanky", "logs", fmt.Sprintf("task-%d.log", task.Number))

	if err := worktree.EnsureCleanWorktree(ctx, d.Runner, d.RepoRoot, wtPath, branch, "main"); err != nil {
		return d.markEarlyFailure(ctx, task, repoSlug, fmt.Sprintf("worktree setup failed: %v", err))
	}

	if err := statuswrite.WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Progress", time.Second); err != nil {
		d.Progress.Note("warn: could not set Status=In Progress for #%d: %v", task.Number, err)
	}
	d.Progress.TaskInProgress(task.Number)

	res, err := agent.RunAgent(ctx, d.Runner, d.Spawner, agent.AgentJob{
		FeatureID: d.FeatureID, Task: task,
		WorktreePath: wtPath, LogPath: logPath, RepoSlug: repoSlug,
		Timeout: d.Timeout,
	})
	if err != nil {
		return d.markEarlyFailure(ctx, task, repoSlug, fmt.Sprintf("agent error: %v", err))
	}

	switch res.Outcome {
	case agent.OutcomeInReview:
		if err := statuswrite.WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Review", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=In Review for #%d: %v", task.Number, err)
		}
		prNum := 0
		if res.PR != nil {
			prNum = res.PR.Number
		}
		d.Progress.TaskInReview(task.Number, prNum)
		if err := worktree.RemoveWorktree(ctx, d.Runner, d.RepoRoot, wtPath); err != nil {
			d.Progress.Note("warn: could not remove worktree for #%d: %v", task.Number, err)
		}

	case agent.OutcomeNeedsAttention:
		if err := statuswrite.WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "Needs Attention", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=Needs Attention for #%d: %v", task.Number, err)
		}
		// Compose and post breadcrumb (best-effort).
		prior, _ := agent.CountPriorAttempts(ctx, d.Runner, repoSlug, task.Number)
		attempt := prior + 1
		body := agent.BuildBreadcrumb(agent.BreadcrumbData{
			Attempt: attempt, StartedAt: res.StartedAt, Duration: res.Duration,
			Outcome: res.OutcomeReason, WorktreePath: wtPath, LogPath: logPath,
			LastLogLines: tailLog(logPath, 20),
		})
		if err := agent.PostBreadcrumb(ctx, d.Runner, repoSlug, task.Number, body); err != nil {
			d.Progress.Note("warn: could not post breadcrumb for #%d: %v", task.Number, err)
		}
		d.Progress.TaskNeedsAttention(task.Number, attempt)
	}
	return *res
}

// markEarlyFailure handles the case where a task can't even start running
// (worktree setup failed, claude binary missing, log file un-creatable). The
// kanban board must reflect that the task is stuck, so we write Status =
// Needs Attention and post a breadcrumb — both best-effort.
func (d Deps) markEarlyFailure(ctx context.Context, task snapshot.TaskInfo, repoSlug, reason string) agent.TaskResult {
	if err := statuswrite.WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "Needs Attention", time.Second); err != nil {
		d.Progress.Note("warn: could not set Status=Needs Attention for #%d: %v", task.Number, err)
	}
	prior, _ := agent.CountPriorAttempts(ctx, d.Runner, repoSlug, task.Number)
	attempt := prior + 1
	body := agent.BuildBreadcrumb(agent.BreadcrumbData{
		Attempt: attempt, StartedAt: time.Now(), Duration: 0,
		Outcome: reason, WorktreePath: worktree.WorktreePath(d.WorktreeRoot, d.Config.Repo.Name, d.FeatureID, task.Number),
		LogPath: filepath.Join(d.RepoRoot, ".klanky", "logs", fmt.Sprintf("task-%d.log", task.Number)),
	})
	if err := agent.PostBreadcrumb(ctx, d.Runner, repoSlug, task.Number, body); err != nil {
		d.Progress.Note("warn: could not post breadcrumb for #%d: %v", task.Number, err)
	}
	d.Progress.TaskNeedsAttention(task.Number, attempt)
	return agent.TaskResult{
		TaskNumber:    task.Number,
		Outcome:       agent.OutcomeNeedsAttention,
		OutcomeReason: reason,
		StartedAt:     time.Now(),
	}
}

func tailLog(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return agent.TailLines(string(data), n)
}

// applyReconcile mutates snap in-place to reflect the reconcile actions, then
// applies them to GitHub. Returns a short human-readable summary string used
// in the progress event line, or "" when no actions were applied.
func applyReconcile(ctx context.Context, r gh.Runner, cfg *config.Config, snap *snapshot.Snapshot, actions []reconcile.Action, featureID int) string {
	if len(actions) == 0 {
		return ""
	}
	for _, a := range actions {
		// Update in-memory snapshot so SelectWork sees post-reconcile state.
		for i := range snap.Tasks {
			if snap.Tasks[i].Number == a.TaskNumber {
				snap.Tasks[i].Status = a.NewStatus
				break
			}
		}
		// Best-effort writes; failures are logged via stderr by WriteStatus internals.
		if err := statuswrite.WriteStatus(ctx, r, cfg, a.ItemID, a.NewStatus, time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "klanky: reconcile WriteStatus #%d → %s failed: %v\n", a.TaskNumber, a.NewStatus, err)
		}
		if a.Breadcrumb != "" {
			body := agent.BuildReconcileBreadcrumb(a.Breadcrumb)
			if err := agent.PostBreadcrumb(ctx, r, cfg.Repo.Owner+"/"+cfg.Repo.Name, a.TaskNumber, body); err != nil {
				fmt.Fprintf(os.Stderr, "klanky: reconcile PostBreadcrumb #%d failed: %v\n", a.TaskNumber, err)
			}
		}
	}
	first := actions[0]
	if len(actions) == 1 {
		return fmt.Sprintf("set #%d → %s", first.TaskNumber, first.NewStatus)
	}
	return fmt.Sprintf("applied %d status updates (first: #%d → %s)", len(actions), first.TaskNumber, first.NewStatus)
}

func countByStatus(tasks []snapshot.TaskInfo, status string) int {
	n := 0
	for _, t := range tasks {
		if t.Status == status {
			n++
		}
	}
	return n
}
