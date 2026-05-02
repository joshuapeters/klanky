package progress

import (
	"fmt"
	"io"
	"time"
)

// Progress emits human-readable timestamped event lines. Output goes to a
// configurable writer (stderr in production) so that the end-of-run summary
// table on stdout stays clean for shell composition.
type Progress struct {
	w     io.Writer
	clock func() time.Time
}

// NewProgress returns a Progress writing to w using clock for timestamps.
// Pass time.Now in production; tests inject a fixed clock.
func NewProgress(w io.Writer, clock func() time.Time) *Progress {
	if clock == nil {
		clock = time.Now
	}
	return &Progress{w: w, clock: clock}
}

func (p *Progress) line(format string, args ...any) {
	fmt.Fprintf(p.w, "[%s] %s\n", p.clock().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// Reconciled summarizes the reconcile pass.
func (p *Progress) Reconciled(scanned int, summary string) {
	if summary == "" {
		p.line("reconcile: %d tasks scanned, no changes", scanned)
	} else {
		p.line("reconcile: %d tasks scanned, %s", scanned, summary)
	}
}

// PhaseSelected reports the chosen phase and the work-queue breakdown.
func (p *Progress) PhaseSelected(phase, todo, needsAttention, awaitingReview int) {
	p.line("phase %d: %d tasks ready (%d todo, %d needs-attention), %d awaiting review",
		phase, todo+needsAttention, todo, needsAttention, awaitingReview)
}

// TaskInProgress logs that a task started executing.
func (p *Progress) TaskInProgress(taskNumber int) {
	p.line("task #%d → in-progress", taskNumber)
}

// TaskInReview logs that a task succeeded with an open PR.
func (p *Progress) TaskInReview(taskNumber, prNumber int) {
	p.line("task #%d → in-review (PR #%d)", taskNumber, prNumber)
}

// TaskNeedsAttention logs that a task ended in needs-attention with the
// running attempt count.
func (p *Progress) TaskNeedsAttention(taskNumber, attempt int) {
	p.line("task #%d → needs-attention (%s attempt)", taskNumber, ordinal(attempt))
}

// Note logs an arbitrary line (used for setup messages, summaries, etc.).
func (p *Progress) Note(format string, args ...any) {
	p.line(format, args...)
}

// ordinal formats a positive integer with its English ordinal suffix.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
