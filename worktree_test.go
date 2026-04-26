package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCleanWorktree_FreshCreate(t *testing.T) {
	wtRoot := t.TempDir()
	wtPath := filepath.Join(wtRoot, "feat-7", "task-42")
	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/repo", "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", "/repo", "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

	if err := EnsureCleanWorktree(context.Background(), r, "/repo", wtPath, "klanky/feat-7/task-42", "main"); err != nil {
		t.Fatalf("EnsureCleanWorktree: %v", err)
	}
}

func TestEnsureCleanWorktree_WipesExistingPath(t *testing.T) {
	wtRoot := t.TempDir()
	wtPath := filepath.Join(wtRoot, "feat-7", "task-42")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "leftover.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/repo", "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", "/repo", "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

	if err := EnsureCleanWorktree(context.Background(), r, "/repo", wtPath, "klanky/feat-7/task-42", "main"); err != nil {
		t.Fatalf("EnsureCleanWorktree: %v", err)
	}

	// Path should have been removed before git was asked to recreate it.
	if _, err := os.Stat(filepath.Join(wtPath, "leftover.txt")); !os.IsNotExist(err) {
		t.Errorf("leftover.txt should be gone; got err=%v", err)
	}
}

func TestWorktreePath_StableLayout(t *testing.T) {
	got := WorktreePath("/home/u/.klanky/worktrees", "myrepo", 7, 42)
	want := "/home/u/.klanky/worktrees/myrepo/feat-7/task-42"
	if got != want {
		t.Errorf("WorktreePath = %q, want %q", got, want)
	}
}

func TestDefaultWorktreeRoot_UsesHome(t *testing.T) {
	root, err := DefaultWorktreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".klanky", "worktrees")
	if root != want {
		t.Errorf("DefaultWorktreeRoot = %q, want %q", root, want)
	}
}
