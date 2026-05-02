package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/progress"
	"github.com/joshuapeters/klanky/internal/snapshot"
	"github.com/joshuapeters/klanky/internal/worktree"
)

func mockConfig() *config.Config {
	return &config.Config{
		SchemaVersion: 1,
		Repo:          config.ConfigRepo{Owner: "alice", Name: "proj"},
		Project: config.ConfigProject{
			URL: "https://github.com/users/alice/projects/1", Number: 1,
			NodeID: "PVT_x", OwnerLogin: "alice", OwnerType: "User",
			Fields: config.ConfigFields{
				Phase: config.ConfigField{ID: "PVTF_p", Name: "Phase"},
				Status: config.ConfigStatusField{ID: "PVTSSF_s", Name: "Status",
					Options: map[string]string{
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e",
					}},
			},
		},
		FeatureLabel: config.ConfigLabel{Name: "klanky:feature"},
	}
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// fakeSpawner is a minimal Spawner implementation for run_test.
type fakeSpawner struct {
	exitCode int
	stdout   string
	err      error
}

func (f *fakeSpawner) Spawn(ctx context.Context, name string, args []string, opts agent.SpawnOpts) (int, error) {
	if opts.Stdout != nil && f.stdout != "" {
		opts.Stdout.Write([]byte(f.stdout))
	}
	return f.exitCode, f.err
}

func TestRunFeature_HappyPath_OneEligibleTaskOpensPR(t *testing.T) {
	repoRoot := t.TempDir()
	klankyDir := filepath.Join(repoRoot, ".klanky")
	if err := os.MkdirAll(klankyDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := mockConfig()
	r := gh.NewFakeRunner()

	// Snapshot fetch.
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 7, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 42, "title": "T", "body": "## Context\n...",
				"state": "OPEN", "id": "I_42",
				"projectItems": {"nodes": [{
					"id": "PVTI_42", "project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshot.SnapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state",
		"--limit", "200"},
		[]byte(`[]`), nil)

	// Worktree creation.
	wtPath := worktree.WorktreePath(filepath.Join(repoRoot, "wt-root"), "proj", 7, 42)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "remove", wtPath, "--force"}, nil, nil)

	// Status writes (in-progress, then in-review). Both succeed first try.
	r.Stub([]string{"gh", "project", "item-edit",
		"--id", "PVTI_42",
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", cfg.Project.Fields.Status.Options["In Progress"]}, nil, nil)
	r.Stub([]string{"gh", "project", "item-edit",
		"--id", "PVTI_42",
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", cfg.Project.Fields.Status.Options["In Review"]}, nil, nil)

	// Post-spawn verification.
	r.Stub([]string{"git", "-C", wtPath, "rev-list", "--count", "main..HEAD"}, []byte("3\n"), nil)
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://github.com/alice/proj/pull/77","number":77}]`), nil)

	sp := &fakeSpawner{exitCode: 0, stdout: "ok\n"}

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := Feature(context.Background(), Deps{
		Runner: r, Spawner: sp, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     progress.NewProgress(progBuf, fixedClock(time.Now())),
		SummaryOut:   sumBuf,
		Timeout:      time.Minute,
	})
	if err != nil {
		t.Fatalf("Feature: %v", err)
	}

	if !strings.Contains(sumBuf.String(), "1 in-review") {
		t.Errorf("summary missing in-review count:\n%s", sumBuf.String())
	}
	if !strings.Contains(progBuf.String(), "task #42 → in-progress") {
		t.Errorf("progress missing in-progress event:\n%s", progBuf.String())
	}
	if !strings.Contains(progBuf.String(), "task #42 → in-review (PR #77)") {
		t.Errorf("progress missing in-review event:\n%s", progBuf.String())
	}
}

func TestRunFeature_FeatureComplete_ShortCircuits(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".klanky"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := mockConfig()
	r := gh.NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 7, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 42, "title": "T", "body": "...",
				"state": "CLOSED", "id": "I_42",
				"projectItems": {"nodes": [{
					"id": "PVTI_42", "project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Done", "optionId": "e"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshot.SnapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state",
		"--limit", "200"},
		[]byte(`[]`), nil)

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := Feature(context.Background(), Deps{
		Runner: r, Spawner: &fakeSpawner{}, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     progress.NewProgress(progBuf, fixedClock(time.Now())),
		SummaryOut:   sumBuf,
		Timeout:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sumBuf.String(), "Feature #7 is complete") {
		t.Errorf("summary missing feature-complete message:\n%s", sumBuf.String())
	}
}
