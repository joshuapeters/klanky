package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/snapshot"
)

// --- selectWork unit tests ---------------------------------------------

func mkSnap(slug string, issues ...snapshot.Issue) *snapshot.Snapshot {
	return &snapshot.Snapshot{ProjectSlug: slug, Issues: issues, PRsByBranch: map[string]snapshot.PR{}}
}

func openIssue(num int, status string, blockers ...snapshot.Blocker) snapshot.Issue {
	return snapshot.Issue{Number: num, ItemID: "PVTI", State: "OPEN", Status: status, BlockedBy: blockers}
}

func closedBlocker(n int) snapshot.Blocker {
	return snapshot.Blocker{Number: n, State: "CLOSED", Repo: "joshuapeters/klanky"}
}
func openBlocker(n int) snapshot.Blocker {
	return snapshot.Blocker{Number: n, State: "OPEN", Repo: "joshuapeters/klanky"}
}

func TestSelectWork_Eligible(t *testing.T) {
	snap := mkSnap("auth",
		openIssue(1, "Todo"),
		openIssue(2, "Needs Attention"),
		openIssue(3, "Todo", openBlocker(99)), // blocked
		openIssue(4, "In Review"),             // awaiting
		openIssue(5, "Done"),                  // ineligible status
	)
	got, scenario := selectWork(snap)
	if scenario != nil {
		t.Fatalf("unexpected scenario: %v", scenario)
	}
	want := []snapshot.Issue{openIssue(1, "Todo"), openIssue(2, "Needs Attention")}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("eligible (-want +got):\n%s", diff)
	}
}

func TestSelectWork_AllClosed(t *testing.T) {
	snap := mkSnap("auth",
		snapshot.Issue{Number: 1, State: "CLOSED", Status: "Done"},
	)
	_, scenario := selectWork(snap)
	if scenario == nil || !strings.Contains(scenario.msg, "no open tracked issues") {
		t.Errorf("scenario = %v", scenario)
	}
}

func TestSelectWork_AllBlocked(t *testing.T) {
	snap := mkSnap("auth",
		openIssue(1, "Todo", openBlocker(99)),
		openIssue(2, "Todo", openBlocker(99)),
	)
	_, scenario := selectWork(snap)
	if scenario == nil || !strings.Contains(scenario.msg, "all blocked") {
		t.Errorf("scenario = %v", scenario)
	}
}

func TestSelectWork_AwaitingReview(t *testing.T) {
	snap := mkSnap("auth", openIssue(1, "In Review"))
	_, scenario := selectWork(snap)
	if scenario == nil || !strings.Contains(scenario.msg, "awaiting review") {
		t.Errorf("scenario = %v", scenario)
	}
}

func TestSelectWork_ClosedBlockerDoesNotBlock(t *testing.T) {
	snap := mkSnap("auth",
		openIssue(1, "Todo", closedBlocker(99)),
	)
	got, scenario := selectWork(snap)
	if scenario != nil || len(got) != 1 {
		t.Errorf("expected 1 eligible (closed blocker doesn't count); got %v / %v", got, scenario)
	}
}

func TestSelectWork_InProgressIsBugScenario(t *testing.T) {
	snap := mkSnap("auth", openIssue(1, "In Progress"))
	_, scenario := selectWork(snap)
	if scenario == nil || scenario.exit != 1 {
		t.Errorf("expected bug scenario exit=1, got %v", scenario)
	}
}

// --- Run() end-to-end with fakes ---------------------------------------

// fakeSpawner returns a stubbed exit code/err and lets the test inspect args.
type fakeSpawner struct {
	exitCode int
	err      error
}

func (f *fakeSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts2) (int, error) {
	return f.exitCode, f.err
}

// SpawnOpts2 is an alias to avoid importing agent.SpawnOpts twice in a file
// that imports package agent under its own name. Keep it identical.
type SpawnOpts2 = agent.SpawnOpts

// agentSpawnerAdapter wraps fakeSpawner so it satisfies agent.Spawner.
type agentSpawnerAdapter struct{ inner *fakeSpawner }

func (a agentSpawnerAdapter) Spawn(ctx context.Context, name string, args []string, opts agent.SpawnOpts) (int, error) {
	return a.inner.Spawn(ctx, name, args, opts)
}

func TestRun_NothingToDo_AllClosed(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects: map[string]config.Project{
			"auth": {
				NodeID: "PVT_x", Number: 4, OwnerLogin: "joshuapeters",
				Fields: config.ProjectFields{Status: config.StatusField{
					ID: "PVTSSF", Name: "Status",
					Options: map[string]string{"Todo": "1", "In Progress": "2", "In Review": "3", "Needs Attention": "4", "Done": "5"},
				}},
			},
		},
	}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fake := gh.NewFakeRunner()
	// Snapshot fetch: empty items.
	fake.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshot.SnapshotQuery,
		"-f", "pid=PVT_x"},
		[]byte(`{"data":{"node":{"items":{"nodes":[]}}}}`), nil)
	fake.Stub([]string{"gh", "pr", "list",
		"--repo", "joshuapeters/klanky",
		"--state", "all",
		"--search", "head:klanky/auth/",
		"--json", "headRefName,number,url,state",
		"--limit", "200"},
		[]byte(`[]`), nil)

	stateRoot := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := Run(context.Background(), Deps{
		Runner: fake, Spawner: agentSpawnerAdapter{&fakeSpawner{}},
		Config: cfg, ProjectSlug: "auth",
		RepoRoot: dir, StateRoot: stateRoot, Output: "text",
		Stdout: stdout, Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "no open tracked issues") {
		t.Errorf("stdout:\n%s", stdout.String())
	}
}

func TestRun_HappyPath_OneEligibleIssue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	// Make repoRoot a real git repo dir for worktree commands. We'll stub
	// the git commands so it doesn't need real git state.
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects: map[string]config.Project{
			"auth": {
				NodeID: "PVT_x", Number: 4, OwnerLogin: "joshuapeters",
				Fields: config.ProjectFields{Status: config.StatusField{
					ID: "PVTSSF", Name: "Status",
					Options: map[string]string{"Todo": "1", "In Progress": "2", "In Review": "3", "Needs Attention": "4", "Done": "5"},
				}},
			},
		},
	}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stateRoot := t.TempDir()

	fake := gh.NewFakeRunner()
	// Snapshot fetch: 1 tracked open issue.
	fake.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshot.SnapshotQuery,
		"-f", "pid=PVT_x"},
		[]byte(`{"data":{"node":{"items":{"nodes":[
			{"id":"PVTI_42","content":{
				"number":42,"title":"Login","state":"OPEN","body":"do it",
				"labels":{"nodes":[{"name":"klanky:tracked"}]},
				"blockedBy":{"nodes":[]}
			},"fieldValues":{"nodes":[
				{"name":"Todo","field":{"name":"Status"}}
			]}}
		]}}}}`), nil)
	fake.Stub([]string{"gh", "pr", "list",
		"--repo", "joshuapeters/klanky",
		"--state", "all",
		"--search", "head:klanky/auth/",
		"--json", "headRefName,number,url,state",
		"--limit", "200"},
		[]byte(`[]`), nil)

	wtPath := filepath.Join(stateRoot, "worktrees", "joshuapeters", "klanky", "auth", "issue-42")
	// Worktree commands.
	fake.Stub([]string{"git", "-C", dir, "worktree", "prune"}, nil, nil)
	fake.Stub([]string{"git", "-C", dir, "worktree", "add", wtPath, "-b", "klanky/auth/issue-42", "main"}, nil, nil)
	// Status=In Progress.
	stubStatusWrite(fake, "PVT_x", "PVTI_42", "PVTSSF", "2") // Todo→In Progress optID="2"
	// Post-spawn verification: branch has commits + PR exists.
	fake.Stub([]string{"git", "-C", wtPath, "rev-list", "--count", "main..HEAD"}, []byte("3\n"), nil)
	fake.Stub([]string{"gh", "pr", "list",
		"--repo", "joshuapeters/klanky",
		"--head", "klanky/auth/issue-42",
		"--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://github.com/joshuapeters/klanky/pull/99","number":99}]`), nil)
	// Status=In Review optID="3".
	stubStatusWrite(fake, "PVT_x", "PVTI_42", "PVTSSF", "3")
	// Worktree remove.
	fake.Stub([]string{"git", "-C", dir, "worktree", "remove", wtPath, "--force"}, nil, nil)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := Run(context.Background(), Deps{
		Runner: fake, Spawner: agentSpawnerAdapter{&fakeSpawner{exitCode: 0}},
		Config: cfg, ProjectSlug: "auth",
		RepoRoot: dir, StateRoot: stateRoot, Output: "text",
		Timeout: 5 * time.Second,
		Stdout:  stdout, Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("Run: %v\nstderr:\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "in-review") || !strings.Contains(stdout.String(), "#42") {
		t.Errorf("summary missing expected lines:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "1 issue(s) attempted: 1 in-review, 0 needs-attention.") {
		t.Errorf("footer missing/incorrect:\n%s", stdout.String())
	}

	// Lock cleanup.
	if _, err := os.Stat(LockPathFor(stateRoot, "joshuapeters", "klanky", "auth")); !os.IsNotExist(err) {
		t.Errorf("lock should have been released")
	}
}

func stubStatusWrite(fake *gh.FakeRunner, pid, iid, fid, oid string) {
	const q = `mutation($pid: ID!, $iid: ID!, $fid: ID!, $oid: String!) {
  updateProjectV2ItemFieldValue(input: {projectId: $pid, itemId: $iid, fieldId: $fid, value: {singleSelectOptionId: $oid}}) {
    projectV2Item { id }
  }
}`
	fake.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + q,
		"-f", "fid=" + fid,
		"-f", "iid=" + iid,
		"-f", "oid=" + oid,
		"-f", "pid=" + pid},
		[]byte(`{"data":{"updateProjectV2ItemFieldValue":{"projectV2Item":{"id":"`+iid+`"}}}}`),
		nil)
}
