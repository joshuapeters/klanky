package worktree

import (
	"context"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/gh"
)

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
