package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunFeature_HappyPath_OneEligibleTaskOpensPR(t *testing.T) {
	repoRoot := t.TempDir()
	klankyDir := filepath.Join(repoRoot, ".klanky")
	if err := os.MkdirAll(klankyDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := mockConfig()
	r := NewFakeRunner()

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
		"-f", "query=" + snapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state,closed,merged",
		"--limit", "200"},
		[]byte(`[]`), nil)

	// Worktree creation.
	wtPath := WorktreePath(filepath.Join(repoRoot, "wt-root"), "proj", 7, 42)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

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

	sp := &FakeSpawner{}
	sp.Stub(0, "ok\n", "", nil)

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := RunFeature(context.Background(), RunFeatureDeps{
		Runner: r, Spawner: sp, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     NewProgress(progBuf, fixedClock(time.Now())),
		SummaryOut:   sumBuf,
		Timeout:      time.Minute,
	})
	if err != nil {
		t.Fatalf("RunFeature: %v", err)
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
	r := NewFakeRunner()
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
		"-f", "query=" + snapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state,closed,merged",
		"--limit", "200"},
		[]byte(`[]`), nil)

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := RunFeature(context.Background(), RunFeatureDeps{
		Runner: r, Spawner: &FakeSpawner{}, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     NewProgress(progBuf, fixedClock(time.Now())),
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
