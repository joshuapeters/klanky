// Package reconcile computes the Status updates needed to bring the
// runner-maintained Status mirror in sync with underlying GitHub truth (issue
// state + PR state). Decisions follow the matrix in project_runner_design.md.
//
// Note: rows 3 and 5 of the spec matrix are collapsed here. Both describe an
// "in-progress without an open PR" condition; row 3 sets Status=Needs Attention
// and row 5 sets Status=Todo based on whether a worktree exists. Worktree
// inspection would couple this pure function to the filesystem, and either
// outcome leaves the issue eligible on the next pass — so we always pick
// Needs Attention. If the distinction matters later, add a worktree check.
package reconcile

import (
	"fmt"
	"strings"

	"github.com/joshuapeters/klanky/internal/snapshot"
)

// Action is a single Status mutation (with optional breadcrumb) the runner
// applies during reconcile. NewStatus is the Status option name (e.g. "Done").
type Action struct {
	IssueNumber int
	ItemID      string
	NewStatus   string
	Breadcrumb  string // freeform; "" means no comment
}

// Reconcile returns the list of state updates needed to align Status with
// truth. Implements the rows from project_runner_design.md.
func Reconcile(snap *snapshot.Snapshot) []Action {
	var actions []Action
	for _, issue := range snap.Issues {
		branch := snapshot.BranchForIssue(snap.ProjectSlug, issue.Number)
		pr, hasPR := snap.PRsByBranch[branch]

		// Row 1: closed issue → Done.
		if issue.State == "CLOSED" {
			if issue.Status != "Done" {
				actions = append(actions, Action{
					IssueNumber: issue.Number, ItemID: issue.ItemID,
					NewStatus: "Done",
				})
			}
			continue
		}

		switch issue.Status {
		case "", "Todo":
			// Row 2: nothing to do.

		case "In Progress":
			if hasPR && pr.State == "OPEN" {
				// Row 4: agent landed PR but Status flip didn't take.
				actions = append(actions, Action{
					IssueNumber: issue.Number, ItemID: issue.ItemID,
					NewStatus: "In Review",
				})
			} else {
				// Row 3 (collapsed with 5): in-progress with no live runner and
				// no open PR. Lock takeover already cleared any process; this
				// is by definition stale.
				actions = append(actions, Action{
					IssueNumber: issue.Number, ItemID: issue.ItemID,
					NewStatus:   "Needs Attention",
					Breadcrumb:  "previous run crashed mid-issue before opening a PR; worktree preserved if it existed.",
				})
			}

		case "In Review":
			switch {
			case !hasPR:
				// Row 8: status set but PR absent.
				actions = append(actions, Action{
					IssueNumber: issue.Number, ItemID: issue.ItemID,
					NewStatus:   "Needs Attention",
					Breadcrumb:  "Status was In Review but no PR exists on the expected branch; was the PR deleted or the branch force-pushed?",
				})
			case pr.State != "OPEN":
				// Row 7: PR closed without merge (merged would have closed the issue).
				actions = append(actions, Action{
					IssueNumber: issue.Number, ItemID: issue.ItemID,
					NewStatus:   "Needs Attention",
					Breadcrumb:  fmt.Sprintf("PR #%d was %s without merging; review feedback may be in the PR thread.", pr.Number, strings.ToLower(pr.State)),
				})
			}
			// Row 6: open PR → no-op.

		case "Needs Attention":
			// Row 9: eligible for retry; no Status change here.

		case "Done":
			// Row 10: invariant violation — issue is open but Status says Done.
			actions = append(actions, Action{
				IssueNumber: issue.Number, ItemID: issue.ItemID,
				NewStatus:   "Needs Attention",
				Breadcrumb:  "invariant violation: Status was Done but issue is open; was the issue manually re-opened?",
			})
		}
	}
	return actions
}
