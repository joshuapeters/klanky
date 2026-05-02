package reconcile

import (
	"strconv"

	"github.com/joshuapeters/klanky/internal/snapshot"
)

// Action describes a single status mutation (and optional breadcrumb) the
// runner should apply during the reconcile phase. NewStatus is the literal
// Status option name (e.g. "Done", "Needs Attention").
type Action struct {
	TaskNumber int
	ItemID     string
	NewStatus  string
	Breadcrumb string // freeform; empty means no breadcrumb to post
}

// Reconcile inspects the snapshot and returns the list of state updates needed
// to bring the runner-maintained Status mirror in sync with underlying truth
// (issue state + PR state). Implements the 11-row matrix from
// project_runner_design.md.
func Reconcile(snap *snapshot.Snapshot, featureID int) []Action {
	var actions []Action
	for _, task := range snap.Tasks {
		// Row 11: missing Phase value — flag and skip further reconcile for this task.
		if task.Phase == nil {
			if task.Status != "Needs Attention" {
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "Task has no Phase value; set one in the project to re-arm.",
				})
			}
			continue
		}

		// Row 1: closed issue → Done.
		if task.State == "CLOSED" {
			if task.Status != "Done" {
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus: "Done",
				})
			}
			continue
		}

		branch := snapshot.BranchForTask(featureID, task.Number)
		pr, hasPR := snap.PRsByBranch[branch]

		switch task.Status {
		case "", "Todo":
			// Row 2: nothing to reconcile.
		case "In Progress":
			if hasPR && pr.State == "OPEN" {
				// Row 4: agent landed PR but status flip didn't take.
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus: "In Review",
				})
			} else {
				// Row 3: crashed mid-task (lock takeover already cleared the process,
				// so any surviving In Progress is by definition stale).
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "previous run crashed mid-task before opening a PR; worktree preserved if it existed.",
				})
			}
		case "In Review":
			if !hasPR {
				// Row 8: status was set but PR is gone.
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "Status was In Review but no PR exists on the expected branch; was the PR deleted or the branch force-pushed?",
				})
			} else if pr.State != "OPEN" {
				// Row 7: PR closed without merge.
				actions = append(actions, Action{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "PR #" + strconv.Itoa(pr.Number) + " was closed without merging; review feedback may be in the PR thread.",
				})
			}
			// Row 6: open PR, leave alone.
		case "Needs Attention":
			// Row 9: eligible for work via the work queue; reconcile no-op.
		case "Done":
			// Row 10: invariant violation (Done implies closed).
			actions = append(actions, Action{
				TaskNumber: task.Number, ItemID: task.ItemID,
				NewStatus:  "Needs Attention",
				Breadcrumb: "invariant violation: Status was Done but issue is open; was the issue manually re-opened?",
			})
		}
	}
	return actions
}
