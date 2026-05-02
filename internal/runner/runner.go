// Package runner orchestrates `klanky run --project <slug>`. It composes the
// snapshot → reconcile → eligibility → spawn → summarize flow described in
// project_runner_design.md.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/lock"
	"github.com/joshuapeters/klanky/internal/reconcile"
	"github.com/joshuapeters/klanky/internal/snapshot"
	"github.com/joshuapeters/klanky/internal/statuswrite"
	"github.com/joshuapeters/klanky/internal/worktree"
)

// DefaultConcurrency is the maximum number of agents running concurrently.
const DefaultConcurrency = 5

// DefaultTimeout is the per-issue agent timeout.
const DefaultTimeout = 20 * time.Minute

// Deps bundles everything Run needs. Stdout receives the summary; Stderr
// receives progress lines.
type Deps struct {
	Runner      gh.Runner
	Spawner     agent.Spawner
	Config      *config.Config
	ProjectSlug string
	RepoRoot    string // absolute path to the main checkout
	StateRoot   string // typically ~/.klanky
	Output      string // "text" or "json"
	Concurrency int    // 0 → DefaultConcurrency
	Timeout     time.Duration
	Stdout      io.Writer
	Stderr      io.Writer
}

// summaryRow is one entry in the end-of-run report.
type summaryRow struct {
	Issue  int    `json:"issue"`
	Status string `json:"status"`
	Link   string `json:"link,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Run executes one full pass against the named project.
func Run(ctx context.Context, d Deps) error {
	if d.Concurrency == 0 {
		d.Concurrency = DefaultConcurrency
	}
	if d.Timeout == 0 {
		d.Timeout = DefaultTimeout
	}
	mode, err := config.ResolveOutput(d.Config, d.Output)
	if err != nil {
		return err
	}
	d.Output = mode

	project, ok := d.Config.Projects[d.ProjectSlug]
	if !ok {
		return fmt.Errorf("no project with slug %q (try `klanky project list`)", d.ProjectSlug)
	}

	// 1. Acquire lock.
	lockPath := lock.Path(d.StateRoot, d.Config.Repo.Owner, d.Config.Repo.Name, d.ProjectSlug)
	lk, err := lock.Acquire(lockPath)
	if err != nil {
		return err
	}
	defer lk.Release()

	// 2. Fetch snapshot.
	stderrWarn := func(format string, a ...any) { fmt.Fprintf(d.Stderr, format, a...) }
	snap, err := snapshot.Fetch(ctx, d.Runner, d.Config, d.ProjectSlug, stderrWarn)
	if err != nil {
		return err
	}

	// 3. Reconcile (apply mutations to GH and snapshot).
	actions := reconcile.Reconcile(snap)
	d.applyReconcile(ctx, project, snap, actions)

	// 4. Eligibility + nothing-to-do scenarios.
	eligible, scenario := selectWork(snap)
	if scenario != nil {
		d.printScenario(*scenario)
		if scenario.exit != 0 {
			return fmt.Errorf("%s", scenario.msg)
		}
		return nil
	}

	d.logf("eligible: %d issue(s) to run (concurrency %d, timeout %s)", len(eligible), d.Concurrency, d.Timeout)

	// 5. Spawn agents.
	repoSlug := d.Config.Repo.Slug()
	results := make([]agent.Result, len(eligible))

	eg, _ := errgroup.WithContext(ctx) // workers never return errors — best-effort
	sem := semaphore.NewWeighted(int64(d.Concurrency))

	for i, issue := range eligible {
		i, issue := i, issue
		eg.Go(func() error {
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil
			}
			defer sem.Release(1)
			results[i] = d.runOne(ctx, project, issue, repoSlug)
			return nil
		})
	}
	_ = eg.Wait()

	// 6. Summary.
	d.renderSummary(results, repoSlug)
	return nil
}

// applyReconcile mutates snap in-memory and applies each action to GH (best-effort).
func (d Deps) applyReconcile(ctx context.Context, project config.Project, snap *snapshot.Snapshot, actions []reconcile.Action) {
	if len(actions) == 0 {
		d.logf("reconcile: no changes")
		return
	}
	d.logf("reconcile: applying %d action(s)", len(actions))
	for _, a := range actions {
		// Update in-memory so eligibility sees post-reconcile state.
		for i := range snap.Issues {
			if snap.Issues[i].Number == a.IssueNumber {
				snap.Issues[i].Status = a.NewStatus
				break
			}
		}
		if err := statuswrite.Write(ctx, d.Runner, project, a.ItemID, a.NewStatus, time.Second); err != nil {
			d.logf("warn: reconcile #%d → %s failed: %v", a.IssueNumber, a.NewStatus, err)
		}
		if a.Breadcrumb != "" {
			body := agent.BuildReconcileBreadcrumb(a.Breadcrumb)
			if err := agent.PostBreadcrumb(ctx, d.Runner, d.Config.Repo.Slug(), a.IssueNumber, body); err != nil {
				d.logf("warn: reconcile breadcrumb #%d failed: %v", a.IssueNumber, err)
			}
		}
		d.logf("reconcile: #%d → %s", a.IssueNumber, a.NewStatus)
	}
}

// scenarioMsg models a "nothing to do" exit case. exit=0 prints the message;
// exit=1 also returns it as an error.
type scenarioMsg struct {
	msg  string
	exit int
}

// selectWork returns the eligible-and-runnable issues, or a scenario message
// describing why nothing can run. Eligibility:
//   open ∧ all blockedBy closed ∧ Status ∈ {Todo, Needs Attention, ""}.
func selectWork(snap *snapshot.Snapshot) ([]snapshot.Issue, *scenarioMsg) {
	openIssues := 0
	var allBlocked = true
	var allEligibleEmpty = true
	var awaitingReview []snapshot.Issue
	var eligible []snapshot.Issue
	var inProgress []snapshot.Issue

	for _, i := range snap.Issues {
		if i.State != "OPEN" {
			continue
		}
		openIssues++
		if i.Status == "In Progress" {
			inProgress = append(inProgress, i)
			continue
		}
		blocked := false
		for _, b := range i.BlockedBy {
			if b.State == "OPEN" {
				blocked = true
				break
			}
		}
		if !blocked {
			allBlocked = false
		}
		if !blocked && i.Status == "In Review" {
			awaitingReview = append(awaitingReview, i)
		}
		if !blocked && (i.Status == "" || i.Status == "Todo" || i.Status == "Needs Attention") {
			eligible = append(eligible, i)
			allEligibleEmpty = false
		}
	}

	if len(inProgress) > 0 {
		return nil, &scenarioMsg{
			msg:  fmt.Sprintf("project %q has %d issue(s) in unexpected in-progress state — this is a bug; see logs", snap.ProjectSlug, len(inProgress)),
			exit: 1,
		}
	}
	if openIssues == 0 {
		return nil, &scenarioMsg{
			msg: fmt.Sprintf("Project %q has no open tracked issues.", snap.ProjectSlug),
		}
	}
	if allBlocked {
		return nil, &scenarioMsg{
			msg: fmt.Sprintf("Project %q: %d open issue(s), all blocked. Close blockers or remove deps to make work eligible.", snap.ProjectSlug, openIssues),
		}
	}
	if allEligibleEmpty && len(awaitingReview) > 0 {
		var b []string = nil
		for _, i := range awaitingReview {
			b = append(b, fmt.Sprintf("  - #%d %s", i.Number, i.Title))
		}
		return nil, &scenarioMsg{
			msg: fmt.Sprintf("Project %q: %d PR(s) awaiting review:\n%s\nMerge or close them, then re-run.",
				snap.ProjectSlug, len(awaitingReview), join(b, "\n")),
		}
	}
	if allEligibleEmpty {
		return nil, &scenarioMsg{
			msg: fmt.Sprintf("Project %q: nothing eligible to run.", snap.ProjectSlug),
		}
	}
	// Stable order for predictable scheduling.
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].Number < eligible[j].Number })
	return eligible, nil
}

// join is a slim local strings.Join to avoid an extra import in this file.
func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}

// runOne executes the per-issue lifecycle. Always returns a populated Result.
func (d Deps) runOne(ctx context.Context, project config.Project, issue snapshot.Issue, repoSlug string) agent.Result {
	wtPath := worktree.Path(d.StateRoot, d.Config.Repo.Owner, d.Config.Repo.Name, d.ProjectSlug, issue.Number)
	logPath := worktree.LogPath(d.StateRoot, d.Config.Repo.Owner, d.Config.Repo.Name, d.ProjectSlug, issue.Number)
	branch := snapshot.BranchForIssue(d.ProjectSlug, issue.Number)

	if err := worktree.EnsureClean(ctx, d.Runner, d.RepoRoot, wtPath, branch, "main"); err != nil {
		return d.markEarlyFailure(ctx, project, repoSlug, issue, fmt.Sprintf("worktree setup failed: %v", err))
	}
	if err := statuswrite.Write(ctx, d.Runner, project, issue.ItemID, "In Progress", time.Second); err != nil {
		d.logf("warn: could not set Status=In Progress for #%d: %v", issue.Number, err)
	}
	d.logf("#%d: in-progress (worktree %s)", issue.Number, wtPath)

	res, err := agent.RunAgent(ctx, d.Runner, d.Spawner, agent.Job{
		ProjectSlug: d.ProjectSlug,
		IssueNumber: issue.Number, IssueTitle: issue.Title, IssueBody: issue.Body,
		WorktreePath: wtPath, LogPath: logPath, RepoSlug: repoSlug,
		Timeout: d.Timeout,
	})
	if err != nil {
		return d.markEarlyFailure(ctx, project, repoSlug, issue, fmt.Sprintf("agent error: %v", err))
	}

	switch res.Outcome {
	case agent.OutcomeInReview:
		if werr := statuswrite.Write(ctx, d.Runner, project, issue.ItemID, "In Review", time.Second); werr != nil {
			d.logf("warn: could not set Status=In Review for #%d: %v", issue.Number, werr)
		}
		prNum := 0
		if res.PR != nil {
			prNum = res.PR.Number
		}
		d.logf("#%d: in-review (PR #%d)", issue.Number, prNum)
		if rmErr := worktree.Remove(ctx, d.Runner, d.RepoRoot, wtPath); rmErr != nil {
			d.logf("warn: could not remove worktree for #%d: %v", issue.Number, rmErr)
		}
	case agent.OutcomeNeedsAttention:
		d.markNeedsAttention(ctx, project, repoSlug, issue, res, wtPath, logPath)
	}
	return *res
}

// markEarlyFailure handles cases where the issue can't even start (worktree
// setup failed, claude binary missing). Writes Status=Needs Attention and
// posts a breadcrumb (best-effort).
func (d Deps) markEarlyFailure(ctx context.Context, project config.Project, repoSlug string, issue snapshot.Issue, reason string) agent.Result {
	if err := statuswrite.Write(ctx, d.Runner, project, issue.ItemID, "Needs Attention", time.Second); err != nil {
		d.logf("warn: could not set Status=Needs Attention for #%d: %v", issue.Number, err)
	}
	wtPath := worktree.Path(d.StateRoot, d.Config.Repo.Owner, d.Config.Repo.Name, d.ProjectSlug, issue.Number)
	logPath := worktree.LogPath(d.StateRoot, d.Config.Repo.Owner, d.Config.Repo.Name, d.ProjectSlug, issue.Number)
	prior, _ := agent.CountPriorAttempts(ctx, d.Runner, repoSlug, issue.Number)
	body := agent.BuildBreadcrumb(agent.BreadcrumbData{
		Attempt: prior + 1, StartedAt: time.Now(), Duration: 0,
		Outcome: reason, WorktreePath: wtPath, LogPath: logPath,
	})
	if err := agent.PostBreadcrumb(ctx, d.Runner, repoSlug, issue.Number, body); err != nil {
		d.logf("warn: could not post breadcrumb for #%d: %v", issue.Number, err)
	}
	d.logf("#%d: needs-attention (attempt %d) — %s", issue.Number, prior+1, reason)
	return agent.Result{
		IssueNumber: issue.Number, Outcome: agent.OutcomeNeedsAttention,
		OutcomeReason: reason, StartedAt: time.Now(),
	}
}

// markNeedsAttention handles a normal needs-attention result: write Status, post breadcrumb.
func (d Deps) markNeedsAttention(ctx context.Context, project config.Project, repoSlug string, issue snapshot.Issue, res *agent.Result, wtPath, logPath string) {
	if err := statuswrite.Write(ctx, d.Runner, project, issue.ItemID, "Needs Attention", time.Second); err != nil {
		d.logf("warn: could not set Status=Needs Attention for #%d: %v", issue.Number, err)
	}
	prior, _ := agent.CountPriorAttempts(ctx, d.Runner, repoSlug, issue.Number)
	attempt := prior + 1
	body := agent.BuildBreadcrumb(agent.BreadcrumbData{
		Attempt: attempt, StartedAt: res.StartedAt, Duration: res.Duration,
		Outcome: res.OutcomeReason, WorktreePath: wtPath, LogPath: logPath,
		LastLogLines: tailLog(logPath, 20),
	})
	if err := agent.PostBreadcrumb(ctx, d.Runner, repoSlug, issue.Number, body); err != nil {
		d.logf("warn: could not post breadcrumb for #%d: %v", issue.Number, err)
	}
	d.logf("#%d: needs-attention (attempt %d) — %s", issue.Number, attempt, res.OutcomeReason)
}

func tailLog(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return agent.TailLines(string(data), n)
}

func (d Deps) printScenario(s scenarioMsg) {
	if d.Output == config.OutputJSON {
		_ = json.NewEncoder(d.Stdout).Encode(map[string]any{
			"project": d.ProjectSlug, "scenario": s.msg,
		})
		return
	}
	fmt.Fprintln(d.Stdout, s.msg)
}

func (d Deps) renderSummary(results []agent.Result, repoSlug string) {
	rows := make([]summaryRow, 0, len(results))
	for _, r := range results {
		row := summaryRow{Issue: r.IssueNumber, Status: r.Outcome.String()}
		switch r.Outcome {
		case agent.OutcomeInReview:
			if r.PR != nil {
				row.Link = r.PR.URL
			}
		case agent.OutcomeNeedsAttention:
			row.Link = fmt.Sprintf("https://github.com/%s/issues/%d", repoSlug, r.IssueNumber)
			row.Note = r.OutcomeReason
		}
		rows = append(rows, row)
	}

	if d.Output == config.OutputJSON {
		_ = json.NewEncoder(d.Stdout).Encode(rows)
		return
	}
	tw := tabwriter.NewWriter(d.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ISSUE\tSTATUS\tLINK\tNOTE")
	for _, r := range rows {
		fmt.Fprintf(tw, "#%d\t%s\t%s\t%s\n", r.Issue, r.Status, r.Link, r.Note)
	}
	_ = tw.Flush()

	in, na := 0, 0
	for _, r := range rows {
		if r.Status == "in-review" {
			in++
		} else if r.Status == "needs-attention" {
			na++
		}
	}
	fmt.Fprintf(d.Stdout, "\n%d issue(s) attempted: %d in-review, %d needs-attention.\n", len(rows), in, na)
}

// logf writes a timestamped progress line to d.Stderr. Mutex-protected because
// concurrent agents write through it.
var stderrMu sync.Mutex

func (d Deps) logf(format string, a ...any) {
	stderrMu.Lock()
	defer stderrMu.Unlock()
	if d.Stderr == nil {
		return
	}
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(d.Stderr, "[%s] %s\n", ts, fmt.Sprintf(format, a...))
}

// LogPathFor returns the per-issue log path. Exposed for the cmd layer to
// surface in user-facing messages.
func LogPathFor(stateRoot, owner, repo, slug string, issueNumber int) string {
	return worktree.LogPath(stateRoot, owner, repo, slug, issueNumber)
}

// WorktreePathFor returns the per-issue worktree path.
func WorktreePathFor(stateRoot, owner, repo, slug string, issueNumber int) string {
	return worktree.Path(stateRoot, owner, repo, slug, issueNumber)
}

// LockPathFor returns the per-project lock path.
func LockPathFor(stateRoot, owner, repo, slug string) string {
	return lock.Path(stateRoot, owner, repo, slug)
}

// DefaultStateRoot is ~/.klanky.
func DefaultStateRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".klanky"), nil
}
