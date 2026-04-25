package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestInit_FullSequence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()

	// 1. Create project.
	fake.Stub(
		[]string{"gh", "project", "create", "--owner", "@me", "--title", "Klanky", "--format", "json"},
		[]byte(`{"id":"PVT_new","number":7,"url":"https://github.com/users/joshuapeters/projects/7","owner":{"login":"joshuapeters","type":"User"}}`),
		nil,
	)
	// 2. Create Phase field.
	fake.Stub(
		[]string{"gh", "project", "field-create", "7",
			"--owner", "@me", "--name", "Phase", "--data-type", "NUMBER", "--format", "json"},
		[]byte(`{"id":"PVTF_phase","name":"Phase","type":"ProjectV2Field"}`),
		nil,
	)
	// 3. Field-list to find the default Status field's ID and existing options.
	fake.Stub(
		[]string{"gh", "project", "field-list", "7", "--owner", "@me", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTSSF_status","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"opt_todo","name":"Todo"},
				{"id":"opt_inp","name":"In Progress"},
				{"id":"opt_done","name":"Done"}
			]},
			{"id":"PVTF_phase","name":"Phase","type":"ProjectV2Field"}
		]}`), nil,
	)
	// 4. Update Status options via GraphQL (preserving existing IDs).
	expectedMutation := buildUpdateStatusOptionsMutation(map[string]string{
		"Todo":        "opt_todo",
		"In Progress": "opt_inp",
		"Done":        "opt_done",
	})
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + expectedMutation,
			"-f", "fieldId=PVTSSF_status"},
		[]byte(`{"data":{"updateProjectV2Field":{"projectV2Field":{"id":"PVTSSF_status","options":[
			{"id":"opt_todo","name":"Todo"},
			{"id":"opt_inp","name":"In Progress"},
			{"id":"new_ir","name":"In Review"},
			{"id":"new_na","name":"Needs Attention"},
			{"id":"opt_done","name":"Done"}
		]}}}}`), nil,
	)
	// 5. Create label on the repo.
	fake.Stub(
		[]string{"gh", "label", "create", "klanky:feature",
			"--repo", "joshuapeters/klanky",
			"--description", "Marks an issue as a Klanky feature (parent of task sub-issues)",
			"--color", "0E8A16"},
		[]byte(""), nil,
	)

	out := &bytes.Buffer{}
	err := RunInit(context.Background(), fake, InitOptions{
		Owner:      "@me",
		Title:      "Klanky",
		RepoSlug:   "joshuapeters/klanky",
		ConfigPath: cfgPath,
	}, out)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.Project.NodeID != "PVT_new" {
		t.Errorf("NodeID = %q, want PVT_new", cfg.Project.NodeID)
	}
	if cfg.Project.Fields.Phase.ID != "PVTF_phase" {
		t.Errorf("Phase ID = %q", cfg.Project.Fields.Phase.ID)
	}
	if cfg.Project.Fields.Status.Options["In Review"] != "new_ir" {
		t.Errorf(`Status options["In Review"] = %q, want new_ir`,
			cfg.Project.Fields.Status.Options["In Review"])
	}
}
