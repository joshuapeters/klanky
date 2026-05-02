// Package worktree manages per-issue git worktrees under
// `~/.klanky/worktrees/<owner>/<repo>/<slug>/issue-<N>/`. The runner wipes
// and recreates on retry so each agent attempt starts clean against `main`.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joshuapeters/klanky/internal/gh"
)

// Path returns the per-issue worktree path. owner and repo are lowercased.
func Path(stateRoot, owner, repo, slug string, issueNumber int) string {
	return filepath.Join(stateRoot, "worktrees", lc(owner), lc(repo), slug,
		fmt.Sprintf("issue-%d", issueNumber))
}

// LogPath returns the per-issue agent log path.
func LogPath(stateRoot, owner, repo, slug string, issueNumber int) string {
	return filepath.Join(stateRoot, "logs", lc(owner), lc(repo), slug,
		fmt.Sprintf("issue-%d.log", issueNumber))
}

func lc(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// EnsureClean guarantees a fresh git worktree at wtPath on `branch`,
// branched from `base`. Wipes any existing path and prunes git's worktree
// registry first.
//
// repoRoot is the absolute path to the main checkout (where .git lives).
func EnsureClean(ctx context.Context, r gh.Runner, repoRoot, wtPath, branch, base string) error {
	if err := os.RemoveAll(wtPath); err != nil {
		return fmt.Errorf("rm worktree path %s: %w", wtPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", wtPath, err)
	}
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "prune"); err != nil {
		return fmt.Errorf("git worktree prune: %w", err)
	}
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "add", wtPath, "-b", branch, base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	return nil
}

// Remove tears down the per-issue worktree. Uses --force so untracked agent
// scratch files don't block cleanup of an otherwise successful issue.
func Remove(ctx context.Context, r gh.Runner, repoRoot, wtPath string) error {
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "remove", wtPath, "--force"); err != nil {
		return fmt.Errorf("git worktree remove %s: %w", wtPath, err)
	}
	return nil
}
