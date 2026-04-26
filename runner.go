package main

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
)

const concurrencyLimit = 5

// RunFeatureDeps bundles the dependencies of RunFeature so the call signature
// stays manageable as features are added.
type RunFeatureDeps struct {
	Runner       Runner
	Spawner      Spawner
	Config       *Config
	RepoRoot     string
	FeatureID    int
	WorktreeRoot string // typically ~/.klanky/worktrees
	Progress     *Progress
	SummaryOut   io.Writer
	Timeout      time.Duration // per-task agent timeout
}

// RunFeature is the top-level orchestrator for `klanky run <feature-id>`.
// Sequence: lock → fetch snapshot → reconcile (apply) → select work →
// spawn agents in parallel → render summary → release lock.
func RunFeature(ctx context.Context, d RunFeatureDeps) error {
	// 1. Lock.
	lockPath := filepath.Join(d.RepoRoot, ".klanky", fmt.Sprintf("runner-%d.lock", d.FeatureID))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("mkdir .klanky: %w", err)
	}
	lock, err := AcquireLock(lockPath)
	if err != nil {
		return err
	}
	defer lock.Release()

	// 2. Fetch snapshot.
	snap, err := FetchSnapshot(ctx, d.Runner, d.Config, d.FeatureID)
	if err != nil {
		return err
	}

	// 3. Reconcile: compute and apply actions, mutate snapshot in-memory so
	// SelectWork sees the post-reconcile state.
	actions := Reconcile(snap, d.FeatureID)
	reconcileSummary := applyReconcile(ctx, d.Runner, d.Config, snap, actions, d.FeatureID)
	d.Progress.Reconciled(len(snap.Tasks), reconcileSummary)

	// 4. Pick work.
	wq := SelectWork(snap)

	// 5. Handle nothing-to-do scenarios.
	if wq.AllDone {
		RenderSummary(SummaryData{
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
			pr, ok := snap.PRsByBranch[BranchForTask(d.FeatureID, t.Number)]
			if ok {
				links = append(links, fmt.Sprintf("#%d %s", t.Number, pr.URL))
			} else {
				links = append(links, fmt.Sprintf("#%d (no PR found)", t.Number))
			}
		}
		RenderSummary(SummaryData{
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
	results := make([]TaskResult, len(wq.Eligible))
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
	rows := make([]SummaryRow, 0, len(results))
	for _, r := range results {
		row := SummaryRow{Task: r.TaskNumber, Status: r.Outcome.String()}
		switch r.Outcome {
		case OutcomeInReview:
			if r.PR != nil {
				row.Link = r.PR.URL
			}
		case OutcomeNeedsAttention:
			row.Link = fmt.Sprintf("https://github.com/%s/issues/%d", repoSlug, r.TaskNumber)
		}
		rows = append(rows, row)
	}
	RenderSummary(SummaryData{
		Phase: wq.CurrentPhase, Duration: time.Since(startedAt), Rows: rows,
	}, d.SummaryOut)

	return nil
}

func runOneTask(ctx context.Context, d RunFeatureDeps, snap *Snapshot, task TaskInfo, repoSlug string) TaskResult {
	wtPath := WorktreePath(d.WorktreeRoot, d.Config.Repo.Name, d.FeatureID, task.Number)
	branch := BranchForTask(d.FeatureID, task.Number)
	logPath := filepath.Join(d.RepoRoot, ".klanky", "logs", fmt.Sprintf("task-%d.log", task.Number))

	if err := EnsureCleanWorktree(ctx, d.Runner, d.RepoRoot, wtPath, branch, "main"); err != nil {
		return d.markEarlyFailure(ctx, task, repoSlug, fmt.Sprintf("worktree setup failed: %v", err))
	}

	if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Progress", time.Second); err != nil {
		// Best-effort; log via progress and continue.
		d.Progress.Note("warn: could not set Status=In Progress for #%d: %v", task.Number, err)
	}
	d.Progress.TaskInProgress(task.Number)

	res, err := RunAgent(ctx, d.Runner, d.Spawner, AgentJob{
		FeatureID: d.FeatureID, Task: task,
		WorktreePath: wtPath, LogPath: logPath, RepoSlug: repoSlug,
		Timeout: d.Timeout,
	})
	if err != nil {
		return d.markEarlyFailure(ctx, task, repoSlug, fmt.Sprintf("agent error: %v", err))
	}

	switch res.Outcome {
	case OutcomeInReview:
		if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Review", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=In Review for #%d: %v", task.Number, err)
		}
		prNum := 0
		if res.PR != nil {
			prNum = res.PR.Number
		}
		d.Progress.TaskInReview(task.Number, prNum)

	case OutcomeNeedsAttention:
		if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "Needs Attention", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=Needs Attention for #%d: %v", task.Number, err)
		}
		// Compose and post breadcrumb (best-effort).
		prior, _ := CountPriorAttempts(ctx, d.Runner, repoSlug, task.Number)
		attempt := prior + 1
		body := BuildBreadcrumb(BreadcrumbData{
			Attempt: attempt, StartedAt: res.StartedAt, Duration: res.Duration,
			Outcome: res.OutcomeReason, WorktreePath: wtPath, LogPath: logPath,
			LastLogLines: tailLog(logPath, 20),
		})
		if err := PostBreadcrumb(ctx, d.Runner, repoSlug, task.Number, body); err != nil {
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
func (d RunFeatureDeps) markEarlyFailure(ctx context.Context, task TaskInfo, repoSlug, reason string) TaskResult {
	if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "Needs Attention", time.Second); err != nil {
		d.Progress.Note("warn: could not set Status=Needs Attention for #%d: %v", task.Number, err)
	}
	prior, _ := CountPriorAttempts(ctx, d.Runner, repoSlug, task.Number)
	attempt := prior + 1
	body := BuildBreadcrumb(BreadcrumbData{
		Attempt: attempt, StartedAt: time.Now(), Duration: 0,
		Outcome: reason, WorktreePath: WorktreePath(d.WorktreeRoot, d.Config.Repo.Name, d.FeatureID, task.Number),
		LogPath: filepath.Join(d.RepoRoot, ".klanky", "logs", fmt.Sprintf("task-%d.log", task.Number)),
	})
	if err := PostBreadcrumb(ctx, d.Runner, repoSlug, task.Number, body); err != nil {
		d.Progress.Note("warn: could not post breadcrumb for #%d: %v", task.Number, err)
	}
	d.Progress.TaskNeedsAttention(task.Number, attempt)
	return TaskResult{
		TaskNumber:    task.Number,
		Outcome:       OutcomeNeedsAttention,
		OutcomeReason: reason,
		StartedAt:     time.Now(),
	}
}

func tailLog(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return TailLines(string(data), n)
}

// applyReconcile mutates snap in-place to reflect the reconcile actions, then
// applies them to GitHub. Returns a short human-readable summary string used
// in the progress event line, or "" when no actions were applied.
func applyReconcile(ctx context.Context, r Runner, cfg *Config, snap *Snapshot, actions []ReconcileAction, featureID int) string {
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
		if err := WriteStatus(ctx, r, cfg, a.ItemID, a.NewStatus, time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "klanky: reconcile WriteStatus #%d → %s failed: %v\n", a.TaskNumber, a.NewStatus, err)
		}
		if a.Breadcrumb != "" {
			body := fmt.Sprintf("%s\n**Klanky reconcile**\n\n%s\n", klankyReconcileSentinel, a.Breadcrumb)
			if err := PostBreadcrumb(ctx, r, cfg.Repo.Owner+"/"+cfg.Repo.Name, a.TaskNumber, body); err != nil {
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

func countByStatus(tasks []TaskInfo, status string) int {
	n := 0
	for _, t := range tasks {
		if t.Status == status {
			n++
		}
	}
	return n
}
