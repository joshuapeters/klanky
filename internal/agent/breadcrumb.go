package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/joshuapeters/klanky/internal/gh"
)

// AttemptSentinel marks a comment posted by the runner as the breadcrumb from
// one *agent attempt* (success-or-fail). CountPriorAttempts uses it to derive
// the attempt counter shown in the summary.
const AttemptSentinel = "<!-- klanky-attempt -->"

// ReconcileSentinel marks a comment posted by the reconcile pass — e.g. "PR
// was closed without merging" or "Status was Done but issue is open." Distinct
// from the attempt sentinel so reconcile breadcrumbs don't inflate the
// agent-attempt count.
const ReconcileSentinel = "<!-- klanky-reconcile -->"

// BuildReconcileBreadcrumb formats a reconcile-pass comment body around the
// given freeform text. Used by the run orchestrator when reconcile mutations
// need to leave a trail on the issue.
func BuildReconcileBreadcrumb(text string) string {
	return fmt.Sprintf("%s\n**Klanky reconcile**\n\n%s\n", ReconcileSentinel, text)
}

// BreadcrumbData is the substitution input for BuildBreadcrumb.
type BreadcrumbData struct {
	Attempt      int
	StartedAt    time.Time
	Duration     time.Duration
	Outcome      string
	WorktreePath string
	LogPath      string
	LastLogLines []string
}

// BuildBreadcrumb returns the markdown body of a needs-attention comment.
// Format is locked in project_runner_design.md and uses the
// `<!-- klanky-attempt -->` sentinel as the count anchor.
func BuildBreadcrumb(d BreadcrumbData) string {
	var b strings.Builder
	fmt.Fprintln(&b, AttemptSentinel)
	fmt.Fprintf(&b, "**Klanky attempt #%d — needs-attention**\n\n", d.Attempt)
	fmt.Fprintf(&b, "- Started: %s\n", d.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Duration: %s\n", d.Duration.Round(time.Second).String())
	fmt.Fprintf(&b, "- Outcome: %s\n", d.Outcome)
	fmt.Fprintf(&b, "- Worktree: `%s` (preserved)\n", d.WorktreePath)
	fmt.Fprintf(&b, "- Log: `%s`\n\n", d.LogPath)
	fmt.Fprintln(&b, "**Last 20 log lines:**")
	fmt.Fprintln(&b, "```")
	for _, line := range d.LastLogLines {
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b, "```")
	return b.String()
}

// CountPriorAttempts returns the number of comments on the issue whose body
// starts with the klanky-attempt sentinel. The next attempt's number is
// returned-value + 1.
func CountPriorAttempts(ctx context.Context, r gh.Runner, repoSlug string, taskNumber int) (int, error) {
	out, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(taskNumber),
		"--repo", repoSlug, "--json", "comments")
	if err != nil {
		return 0, fmt.Errorf("gh issue view: %w", err)
	}
	var resp struct {
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, fmt.Errorf("parse comments: %w", err)
	}
	count := 0
	for _, c := range resp.Comments {
		if strings.HasPrefix(strings.TrimSpace(c.Body), AttemptSentinel) {
			count++
		}
	}
	return count, nil
}

// PostBreadcrumb posts a comment with the given body on the task issue.
func PostBreadcrumb(ctx context.Context, r gh.Runner, repoSlug string, taskNumber int, body string) error {
	if _, err := r.Run(ctx, "gh", "issue", "comment", strconv.Itoa(taskNumber),
		"--repo", repoSlug, "--body", body); err != nil {
		return fmt.Errorf("gh issue comment: %w", err)
	}
	return nil
}

// TailLines returns the last n lines of a string. If the string has fewer
// than n lines, returns all of them. Trailing empty lines are dropped.
func TailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
