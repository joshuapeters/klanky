package config

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")

	want := &Config{
		SchemaVersion: SchemaVersion,
		Repo:          Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects: map[string]Project{
			"auth": {
				URL: "https://github.com/users/joshuapeters/projects/12", Number: 12,
				NodeID: "PVT_x", Title: "Auth", OwnerLogin: "joshuapeters", OwnerType: "User",
				Fields: ProjectFields{
					Status: StatusField{
						ID: "PVTSSF_x", Name: "Status",
						Options: map[string]string{
							"Todo": "1", "In Progress": "2", "In Review": "3",
							"Needs Attention": "4", "Done": "5",
						},
					},
				},
			},
		},
	}
	if err := SaveConfig(path, want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	if Exists(path) {
		t.Errorf("Exists on missing path = true")
	}
	cfg := &Config{SchemaVersion: SchemaVersion, Repo: Repo{Owner: "o", Name: "r"}}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if !Exists(path) {
		t.Errorf("Exists on existing path = false")
	}
}
