package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joshuapeters/klanky/internal/gh"
)

// DefaultWorktreeRoot returns ~/.klanky/worktrees, the locked-by-design root
// for klanky-managed worktrees.
func DefaultWorktreeRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".klanky", "worktrees"), nil
}

// WorktreePath assembles the per-task worktree path under the given root.
func WorktreePath(root, repoName string, featureID, taskNumber int) string {
	return filepath.Join(root, repoName, fmt.Sprintf("feat-%d", featureID), fmt.Sprintf("task-%d", taskNumber))
}

// EnsureCleanWorktree guarantees a fresh git worktree at wtPath on the given
// branch, branched from base. Wipes any existing path and prunes git's
// worktree registry first so retries always start clean.
//
// repoRoot is the absolute path to the main checkout (where .git lives).
func EnsureCleanWorktree(ctx context.Context, r gh.Runner, repoRoot, wtPath, branch, base string) error {
	if err := os.RemoveAll(wtPath); err != nil {
		return fmt.Errorf("rm worktree path %s: %w", wtPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
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

// RemoveWorktree tears down the per-task worktree at wtPath. Uses --force so
// untracked agent scratch files (TDD experiments, build artifacts) don't block
// cleanup of an otherwise successful task.
func RemoveWorktree(ctx context.Context, r gh.Runner, repoRoot, wtPath string) error {
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "remove", wtPath, "--force"); err != nil {
		return fmt.Errorf("git worktree remove %s: %w", wtPath, err)
	}
	return nil
}
