package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProjectURL(t *testing.T) {
	cases := []struct {
		url           string
		wantOwner     string
		wantNumber    int
		wantOwnerType string
	}{
		{"https://github.com/users/alice/projects/4", "alice", 4, "User"},
		{"https://github.com/orgs/wistia/projects/12", "wistia", 12, "Organization"},
		{"https://github.com/orgs/wistia/projects/12/views/1", "wistia", 12, "Organization"},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			owner, num, ot, err := ParseProjectURL(c.url)
			if err != nil {
				t.Fatalf("ParseProjectURL: %v", err)
			}
			if owner != c.wantOwner || num != c.wantNumber || ot != c.wantOwnerType {
				t.Errorf("got (%q, %d, %q), want (%q, %d, %q)",
					owner, num, ot, c.wantOwner, c.wantNumber, c.wantOwnerType)
			}
		})
	}
}

func TestParseProjectURL_Invalid(t *testing.T) {
	bad := []string{
		"",
		"not a url",
		"https://github.com/alice/foo/issues/1",
		"https://github.com/users/alice/projects/notanumber",
	}
	for _, u := range bad {
		if _, _, _, err := ParseProjectURL(u); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
}

func TestProjectLink_ValidatesAndWritesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"id":"PVT_x","number":4,"url":"https://github.com/users/alice/projects/4","title":"T"}`), nil,
	)
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTF_p","name":"Phase","type":"ProjectV2Field"},
			{"id":"PVTSSF_s","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"t","name":"Todo"},{"id":"ip","name":"In Progress"},
				{"id":"ir","name":"In Review"},{"id":"na","name":"Needs Attention"},
				{"id":"d","name":"Done"}
			]}
		]}`), nil,
	)
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", "alice/proj", "--search", "klanky:feature", "--json", "name"},
		[]byte(`[{"name":"klanky:feature"}]`), nil,
	)

	out := &bytes.Buffer{}
	err := RunProjectLink(context.Background(), fake, ProjectLinkOptions{
		ProjectURL: "https://github.com/users/alice/projects/4",
		RepoSlug:   "alice/proj",
		ConfigPath: cfgPath,
	}, out)
	if err != nil {
		t.Fatalf("RunProjectLink: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.Project.NodeID != "PVT_x" {
		t.Errorf("NodeID not stored: got %q", cfg.Project.NodeID)
	}
	if cfg.Project.Fields.Status.Options["Done"] != "d" {
		t.Errorf("Status options not stored")
	}
	if cfg.Repo.Owner != "alice" || cfg.Repo.Name != "proj" {
		t.Errorf("Repo not stored: %+v", cfg.Repo)
	}
}

func TestProjectLink_NonConforming_PrintsDiff(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"id":"PVT_x","number":4,"url":"https://github.com/users/alice/projects/4"}`), nil,
	)
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTF_p","name":"Phase","type":"ProjectV2Field"},
			{"id":"PVTSSF_s","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"t","name":"Todo"},{"id":"ip","name":"In Progress"},{"id":"d","name":"Done"}
			]}
		]}`), nil,
	)
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", "alice/proj", "--search", "klanky:feature", "--json", "name"},
		[]byte(`[{"name":"klanky:feature"}]`), nil,
	)

	out := &bytes.Buffer{}
	err := RunProjectLink(context.Background(), fake, ProjectLinkOptions{
		ProjectURL: "https://github.com/users/alice/projects/4",
		RepoSlug:   "alice/proj",
		ConfigPath: cfgPath,
	}, out)
	if err == nil {
		t.Fatal("expected error for non-conforming project")
	}
	if !strings.Contains(err.Error(), "In Review") || !strings.Contains(err.Error(), "Needs Attention") {
		t.Errorf("error should list missing options: %v", err)
	}
}
