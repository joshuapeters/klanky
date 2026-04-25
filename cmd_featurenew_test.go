package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, ".klankyrc.json")
	contents := `{
		"schema_version": 1,
		"repo": {"owner": "alice", "name": "proj"},
		"project": {
			"url": "https://github.com/users/alice/projects/1",
			"number": 1, "node_id": "PVT_x",
			"owner_login": "alice", "owner_type": "User",
			"fields": {
				"phase":  {"id": "PVTF_p", "name": "Phase"},
				"status": {"id": "PVTSSF_s", "name": "Status",
					"options": {"Todo":"t","In Progress":"ip","In Review":"ir","Needs Attention":"na","Done":"d"}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFeatureNew_CallsGhIssueCreateAndProjectAdd_AndPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "Login overhaul",
			"--label", "klanky:feature",
			"--body", ""},
		[]byte("https://github.com/alice/proj/issues/42\n"), nil,
	)
	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":42,"id":"I_xyz","url":"https://github.com/alice/proj/issues/42"}`), nil,
	)
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/42",
			"--format", "json"},
		[]byte(`{"id":"PVTI_abc"}`), nil,
	)

	out := &bytes.Buffer{}
	err = RunFeatureNew(context.Background(), fake, cfg, FeatureNewOptions{
		Title: "Login overhaul",
	}, out)
	if err != nil {
		t.Fatalf("RunFeatureNew: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"feature_id":42`) {
		t.Errorf("output missing feature_id: %s", got)
	}
	if !strings.Contains(got, "https://github.com/alice/proj/issues/42") {
		t.Errorf("output missing url: %s", got)
	}
}

func TestFeatureNew_WithBodyFile_ReadsAndPasses(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, _ := LoadConfig(cfgPath)
	bodyPath := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyPath, []byte("Long context goes here."), 0644); err != nil {
		t.Fatal(err)
	}

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "X",
			"--label", "klanky:feature",
			"--body", "Long context goes here."},
		[]byte("https://github.com/alice/proj/issues/43\n"), nil,
	)
	fake.Stub(
		[]string{"gh", "issue", "view", "43", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":43,"id":"I_x","url":"https://github.com/alice/proj/issues/43"}`), nil,
	)
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/43",
			"--format", "json"},
		[]byte(`{"id":"PVTI_y"}`), nil,
	)

	out := &bytes.Buffer{}
	err := RunFeatureNew(context.Background(), fake, cfg, FeatureNewOptions{
		Title:    "X",
		BodyFile: bodyPath,
	}, out)
	if err != nil {
		t.Fatalf("RunFeatureNew: %v", err)
	}
}

func TestFeatureNew_TitleRequired(t *testing.T) {
	cfg := &Config{}
	out := &bytes.Buffer{}
	err := RunFeatureNew(context.Background(), NewFakeRunner(), cfg, FeatureNewOptions{}, out)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should mention title: %v", err)
	}
}
