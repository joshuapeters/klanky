package worktree

import (
	"context"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/gh"
)

func TestPath_Lowercases(t *testing.T) {
	got := Path("/root", "JoshuaPeters", "Klanky", "auth", 42)
	want := "/root/worktrees/joshuapeters/klanky/auth/issue-42"
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestLogPath(t *testing.T) {
	got := LogPath("/root", "owner", "repo", "auth", 42)
	want := "/root/logs/owner/repo/auth/issue-42.log"
	if got != want {
		t.Errorf("LogPath = %q, want %q", got, want)
	}
}

func TestEnsureClean_RunsExpectedCommands(t *testing.T) {
	dir := t.TempDir()
	wt := dir + "/wt/issue-1"

	fake := gh.NewFakeRunner()
	fake.Stub([]string{"git", "-C", "/repo", "worktree", "prune"}, nil, nil)
	fake.Stub([]string{"git", "-C", "/repo", "worktree", "add", wt, "-b", "klanky/auth/issue-1", "main"}, nil, nil)

	if err := EnsureClean(context.Background(), fake, "/repo", wt, "klanky/auth/issue-1", "main"); err != nil {
		t.Fatalf("EnsureClean: %v", err)
	}
	if len(fake.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(fake.Calls), fake.Calls)
	}
	if got := strings.Join(fake.Calls[1], " "); !strings.Contains(got, "worktree add") {
		t.Errorf("second call should be worktree add: %v", got)
	}
}

func TestRemove(t *testing.T) {
	fake := gh.NewFakeRunner()
	fake.Stub([]string{"git", "-C", "/repo", "worktree", "remove", "/wt", "--force"}, nil, nil)
	if err := Remove(context.Background(), fake, "/repo", "/wt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}
