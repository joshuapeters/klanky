package workqueue

import "github.com/joshuapeters/klanky/internal/snapshot"

// Result is the post-reconcile decision about what to actually run.
type Result struct {
	AllDone             bool                // every task in the feature has issue=closed
	CurrentPhase        int                 // lowest phase number with any open issue; 0 when AllDone
	Eligible            []snapshot.TaskInfo // current-phase tasks the runner will spawn agents for
	AwaitingReview      []snapshot.TaskInfo // current-phase tasks with Status=In Review (informational, surfaced in messaging)
	SurvivingInProgress []snapshot.TaskInfo // current-phase tasks with Status=In Progress that somehow survived reconcile (a bug — surfaced as scenario C)
}

// SelectWork picks the current phase and partitions its tasks into the
// queues the runner needs. Caller must have already applied reconcile actions
// (the Snapshot's Status fields reflect the pre-reconcile values).
//
// NOTE: This function operates on the snapshot Status values, not on a
// post-reconcile mutation. The caller is responsible for either passing in a
// snapshot whose Status fields have been updated to reflect the reconcile, or
// applying reconcile actions in their own code path before relying on these
// queues. Most callers will call ApplyReconcile first (Task 11) which mutates
// the snapshot in-memory.
func SelectWork(snap *snapshot.Snapshot) Result {
	openByPhase := map[int][]snapshot.TaskInfo{}
	hasOpen := false
	for _, task := range snap.Tasks {
		if task.State != "OPEN" {
			continue
		}
		hasOpen = true
		if task.Phase == nil {
			// Tasks without Phase are flagged by reconcile (Row 11) and set to
			// Needs Attention; they don't participate in phase selection.
			continue
		}
		openByPhase[*task.Phase] = append(openByPhase[*task.Phase], task)
	}

	if !hasOpen {
		return Result{AllDone: true}
	}

	// Lowest phase with any open task that has a Phase value.
	current := -1
	for phase := range openByPhase {
		if current == -1 || phase < current {
			current = phase
		}
	}
	// If every open task lacks a Phase, openByPhase is empty — degenerate case.
	if current == -1 {
		return Result{AllDone: false, CurrentPhase: 0}
	}

	res := Result{CurrentPhase: current}
	for _, task := range openByPhase[current] {
		switch task.Status {
		case "", "Todo", "Needs Attention":
			res.Eligible = append(res.Eligible, task)
		case "In Review":
			res.AwaitingReview = append(res.AwaitingReview, task)
		case "In Progress":
			res.SurvivingInProgress = append(res.SurvivingInProgress, task)
		}
	}
	return res
}
