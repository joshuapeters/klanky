package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ReadsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	contents := `{
		"schema_version": 1,
		"repo": {"owner": "alice", "name": "myproj"},
		"project": {
			"url": "https://github.com/users/alice/projects/1",
			"number": 1,
			"node_id": "PVT_x",
			"owner_login": "alice",
			"owner_type": "User",
			"fields": {
				"phase":  {"id": "PVTF_p", "name": "Phase"},
				"status": {"id": "PVTSSF_s", "name": "Status",
					"options": {
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e"
					}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.Repo.Owner != "alice" {
		t.Errorf("Repo.Owner = %q, want alice", cfg.Repo.Owner)
	}
	if cfg.Project.NodeID != "PVT_x" {
		t.Errorf("Project.NodeID = %q, want PVT_x", cfg.Project.NodeID)
	}
	if got := cfg.Project.Fields.Status.Options["In Review"]; got != "c" {
		t.Errorf(`Status.Options["In Review"] = %q, want "c"`, got)
	}
}

func TestLoadConfig_MissingFile_ReturnsHelpfulError(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/.klankyrc.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), ".klankyrc.json") {
		t.Errorf("error message should mention .klankyrc.json: %v", err)
	}
}

func TestLoadConfig_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")

	original := &Config{
		SchemaVersion: 1,
		Repo:          ConfigRepo{Owner: "alice", Name: "proj"},
		Project: ConfigProject{
			URL: "https://x", Number: 7, NodeID: "PVT_y",
			OwnerLogin: "alice", OwnerType: "User",
			Fields: ConfigFields{
				Phase:  ConfigField{ID: "p", Name: "Phase"},
				Status: ConfigStatusField{ID: "s", Name: "Status", Options: map[string]string{"Todo": "t"}},
			},
		},
		FeatureLabel: ConfigLabel{Name: "klanky:feature"},
	}
	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Project.NodeID != original.Project.NodeID {
		t.Errorf("NodeID round-trip failed: got %q, want %q", loaded.Project.NodeID, original.Project.NodeID)
	}
	if loaded.Project.Fields.Status.Options["Todo"] != "t" {
		t.Errorf("Status options round-trip failed")
	}
}
