package summary

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// SummaryData feeds RenderSummary. Use the FeatureComplete branch when the
// whole feature is done, the AwaitingReviewLinks branch when the phase only
// has open PRs (no eligible work was found), or Rows otherwise.
type SummaryData struct {
	// Branch 1: whole feature is closed.
	FeatureComplete bool
	FeatureNumber   int
	TotalTasks      int

	// Branch 2: phase has only awaiting-review tasks (no work was attempted).
	AwaitingReviewLinks []string // pre-formatted "#42 https://..."

	// Branch 3: tasks were attempted.
	Phase    int
	Duration time.Duration
	Rows     []SummaryRow
}

// SummaryRow is one line in the attempted-tasks table.
type SummaryRow struct {
	Task   int
	Status string // "in-review" or "needs-attention"
	Link   string
	Note   string // optional (e.g. "3rd attempt")
}

// RenderSummary writes the end-of-run summary to w. Branch order matters —
// FeatureComplete and AwaitingReviewLinks are checked before Rows.
func RenderSummary(d SummaryData, w io.Writer) {
	if d.FeatureComplete {
		fmt.Fprintf(w, "Feature #%d is complete: all %d tasks closed.\n", d.FeatureNumber, d.TotalTasks)
		return
	}
	if len(d.AwaitingReviewLinks) > 0 {
		fmt.Fprintf(w, "Phase %d has %d PRs awaiting your review:\n", d.Phase, len(d.AwaitingReviewLinks))
		for _, link := range d.AwaitingReviewLinks {
			fmt.Fprintf(w, "  - %s\n", link)
		}
		fmt.Fprintln(w, "Merge or close them, then re-run.")
		return
	}

	fmt.Fprintf(w, "Phase %d run complete in %s.\n\n", d.Phase, d.Duration.Round(time.Second))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TASK\tSTATUS\tLINK\tNOTE")
	inReview := 0
	needsAttention := 0
	for _, row := range d.Rows {
		switch row.Status {
		case "in-review":
			inReview++
		case "needs-attention":
			needsAttention++
		}
		fmt.Fprintf(tw, "  #%d\t%s\t%s\t%s\n", row.Task, row.Status, row.Link, row.Note)
	}
	tw.Flush()

	fmt.Fprintf(w, "\n%d tasks attempted: %d in-review, %d needs-attention.\n",
		len(d.Rows), inReview, needsAttention)

	switch {
	case inReview > 0 && needsAttention > 0:
		fmt.Fprintf(w, "Next: review the %d PRs above. Re-run `klanky run <F>` after merging.\n", inReview)
		fmt.Fprintf(w, "      Also: address %d task(s) in needs-attention before re-running (or they'll auto-retry).\n", needsAttention)
	case inReview > 0:
		fmt.Fprintf(w, "Next: review the %d PRs above. Re-run `klanky run <F>` after merging.\n", inReview)
	case needsAttention > 0:
		fmt.Fprintln(w, "Next: all attempts failed; inspect the breadcrumbs and re-run.")
	}
}
