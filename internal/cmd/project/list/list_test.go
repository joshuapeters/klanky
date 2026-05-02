package list

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
)

func write(t *testing.T, cfg *config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

func TestRunProjectList_TextSorted(t *testing.T) {
	cfgPath := write(t, &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
		Projects: map[string]config.Project{
			"zzz": {Title: "Z", URL: "uz"},
			"aaa": {Title: "A", URL: "ua"},
			"mmm": {Title: "M", URL: "um"},
		},
	})
	var out bytes.Buffer
	if err := RunProjectList(Options{ConfigPath: cfgPath}, &out); err != nil {
		t.Fatalf("RunProjectList: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected header + 3 rows, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[1], "aaa") || !strings.HasPrefix(lines[2], "mmm") || !strings.HasPrefix(lines[3], "zzz") {
		t.Errorf("rows not sorted: %v", lines)
	}
}

func TestRunProjectList_JSON(t *testing.T) {
	cfgPath := write(t, &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
		Projects: map[string]config.Project{
			"auth": {Title: "Auth", URL: "https://x"},
		},
	})
	var out bytes.Buffer
	if err := RunProjectList(Options{ConfigPath: cfgPath, Output: "json"}, &out); err != nil {
		t.Fatalf("RunProjectList: %v", err)
	}
	var got []map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0]["slug"] != "auth" || got[0]["title"] != "Auth" {
		t.Errorf("got %v", got)
	}
}

func TestRunProjectList_HonorsConfigDefaultOutput(t *testing.T) {
	cfgPath := write(t, &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
		DefaultOutput: "json",
		Projects: map[string]config.Project{
			"auth": {Title: "Auth", URL: "u"},
		},
	})
	var out bytes.Buffer
	if err := RunProjectList(Options{ConfigPath: cfgPath}, &out); err != nil {
		t.Fatalf("RunProjectList: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "[") {
		t.Errorf("default_output: json should produce JSON; got %q", out.String())
	}
}

func TestRunProjectList_RejectsUnknownOutput(t *testing.T) {
	cfgPath := write(t, &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
	})
	err := RunProjectList(Options{ConfigPath: cfgPath, Output: "yaml"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "yaml") {
		t.Errorf("expected unknown-output error, got %v", err)
	}
}

func TestRunProjectList_EmptyProjects(t *testing.T) {
	cfgPath := write(t, &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
		Projects:      map[string]config.Project{},
	})
	var out bytes.Buffer
	if err := RunProjectList(Options{ConfigPath: cfgPath}, &out); err != nil {
		t.Fatalf("RunProjectList: %v", err)
	}
	// Header still printed; empty rows.
	if !strings.Contains(out.String(), "SLUG") {
		t.Errorf("expected header, got %q", out.String())
	}
}
