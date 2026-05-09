package identifiers_test

import (
	"path/filepath"
	"testing"

	"github.com/joshuapeters/klanky/internal/identifiers"
)

func TestIdentifiers(t *testing.T) {
	ids := identifiers.New("/state", "Owner", "Repo", "auth")

	t.Run("Branch", func(t *testing.T) {
		if got := ids.Branch(42); got != "klanky/auth/issue-42" {
			t.Errorf("Branch(42) = %q", got)
		}
	})

	t.Run("BranchPrefix", func(t *testing.T) {
		if got := ids.BranchPrefix(); got != "klanky/auth/" {
			t.Errorf("BranchPrefix() = %q", got)
		}
	})

	t.Run("WorktreePath", func(t *testing.T) {
		want := filepath.Join("/state", "worktrees", "owner", "repo", "auth", "issue-42")
		if got := ids.WorktreePath(42); got != want {
			t.Errorf("WorktreePath(42) = %q, want %q", got, want)
		}
	})

	t.Run("LogPath", func(t *testing.T) {
		want := filepath.Join("/state", "logs", "owner", "repo", "auth", "issue-42.log")
		if got := ids.LogPath(42); got != want {
			t.Errorf("LogPath(42) = %q, want %q", got, want)
		}
	})

	t.Run("LockPath", func(t *testing.T) {
		want := filepath.Join("/state", "locks", "owner", "repo", "auth.lock")
		if got := ids.LockPath(); got != want {
			t.Errorf("LockPath() = %q, want %q", got, want)
		}
	})

	t.Run("OwnerRepoLowercased", func(t *testing.T) {
		upper := identifiers.New("/state", "JoshuaPeters", "Klanky", "auth")
		want := filepath.Join("/state", "worktrees", "joshuapeters", "klanky", "auth", "issue-1")
		if got := upper.WorktreePath(1); got != want {
			t.Errorf("WorktreePath with uppercase owner/repo = %q, want %q", got, want)
		}
	})
}
