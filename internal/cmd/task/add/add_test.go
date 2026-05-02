package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskAdd_FullSequence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, _ := LoadConfig(cfgPath)

	specPath := filepath.Join(dir, "spec.md")
	specBody := "## Context\nWhy.\n## Acceptance criteria\n- [ ] X\n## Out of scope\nY\n"
	if err := os.WriteFile(specPath, []byte(specBody), 0644); err != nil {
		t.Fatal(err)
	}

	fake := NewFakeRunner()

	// 1. Look up parent (feature) node ID.
	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "id"},
		[]byte(`{"id":"I_parent"}`), nil,
	)

	// 2. Create the task issue.
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "Add login form",
			"--body", specBody},
		[]byte("https://github.com/alice/proj/issues/119\n"), nil,
	)

	// 3. View the new issue to get its number, id, url.
	fake.Stub(
		[]string{"gh", "issue", "view", "119", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":119,"id":"I_child","url":"https://github.com/alice/proj/issues/119"}`), nil,
	)

	// 4. Link as sub-issue via GraphQL (variables sorted alphabetically: issueId, subIssueId).
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + addSubIssueMutation,
			"-f", "issueId=I_parent",
			"-f", "subIssueId=I_child"},
		[]byte(`{"data":{"addSubIssue":{"issue":{"number":42}}}}`), nil,
	)

	// 5. Add to project, capturing item ID.
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/119",
			"--format", "json"},
		[]byte(`{"id":"PVTI_item"}`), nil,
	)

	// 6. Set Phase = 1.
	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTF_p",
			"--project-id", "PVT_x",
			"--number", "1"},
		[]byte(`{}`), nil,
	)

	// 7. Set Status = Todo.
	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTSSF_s",
			"--project-id", "PVT_x",
			"--single-select-option-id", "t"},
		[]byte(`{}`), nil,
	)

	out := &bytes.Buffer{}
	err := RunTaskAdd(context.Background(), fake, cfg, TaskAddOptions{
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
		opts TaskAddOptions
	}{
		{"no feature", TaskAddOptions{Phase: 1, Title: "x", SpecFile: "x"}},
		{"no phase", TaskAddOptions{FeatureID: 1, Title: "x", SpecFile: "x"}},
		{"no title", TaskAddOptions{FeatureID: 1, Phase: 1, SpecFile: "x"}},
		{"no spec-file", TaskAddOptions{FeatureID: 1, Phase: 1, Title: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RunTaskAdd(context.Background(), NewFakeRunner(), &Config{}, c.opts, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
