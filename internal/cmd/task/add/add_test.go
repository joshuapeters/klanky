package add

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
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

func TestTaskAdd_FullSequence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, _ := config.LoadConfig(cfgPath)

	specPath := filepath.Join(dir, "spec.md")
	specBody := "## Context\nWhy.\n## Acceptance criteria\n- [ ] X\n## Out of scope\nY\n"
	if err := os.WriteFile(specPath, []byte(specBody), 0644); err != nil {
		t.Fatal(err)
	}

	fake := gh.NewFakeRunner()

	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "id"},
		[]byte(`{"id":"I_parent"}`), nil,
	)

	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "Add login form",
			"--body", specBody},
		[]byte("https://github.com/alice/proj/issues/119\n"), nil,
	)

	fake.Stub(
		[]string{"gh", "issue", "view", "119", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":119,"id":"I_child","url":"https://github.com/alice/proj/issues/119"}`), nil,
	)

	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + AddSubIssueMutation,
			"-f", "issueId=I_parent",
			"-f", "subIssueId=I_child"},
		[]byte(`{"data":{"addSubIssue":{"issue":{"number":42}}}}`), nil,
	)

	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/119",
			"--format", "json"},
		[]byte(`{"id":"PVTI_item"}`), nil,
	)

	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTF_p",
			"--project-id", "PVT_x",
			"--number", "1"},
		[]byte(`{}`), nil,
	)

	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTSSF_s",
			"--project-id", "PVT_x",
			"--single-select-option-id", "t"},
		[]byte(`{}`), nil,
	)

	out := &bytes.Buffer{}
	err := RunTaskAdd(context.Background(), fake, cfg, Options{
		FeatureID: 42,
		Phase:     1,
		Title:     "Add login form",
		SpecFile:  specPath,
	}, out)
	if err != nil {
		t.Fatalf("RunTaskAdd: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"task_id":119`) {
		t.Errorf("output missing task_id: %s", got)
	}
}

func TestTaskAdd_RequiresAllFlags(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"no feature", Options{Phase: 1, Title: "x", SpecFile: "x"}},
		{"no phase", Options{FeatureID: 1, Title: "x", SpecFile: "x"}},
		{"no title", Options{FeatureID: 1, Phase: 1, SpecFile: "x"}},
		{"no spec-file", Options{FeatureID: 1, Phase: 1, Title: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RunTaskAdd(context.Background(), gh.NewFakeRunner(), &config.Config{}, c.opts, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
