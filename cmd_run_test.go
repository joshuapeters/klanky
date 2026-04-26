package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCmd_RequiresFeatureIDArg(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from missing arg, got nil")
	}
}

func TestRunCmd_RejectsNonNumericFeatureID(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"abc"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from non-numeric arg, got nil")
	}
	if !strings.Contains(err.Error(), "feature-id") {
		t.Errorf("error should mention feature-id: %v", err)
	}
}

func TestRunCmd_MissingConfig_ReturnsHelpfulError(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"42"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from missing config, got nil")
	}
	if !strings.Contains(err.Error(), ".klankyrc.json") {
		t.Errorf("error should mention .klankyrc.json: %v", err)
	}
}

func TestRunCmd_ValidArgs_AttemptsToRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
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
					"options": {"Todo": "a", "In Progress": "b", "In Review": "c", "Needs Attention": "d", "Done": "e"}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(cfgPath, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	cmd := newRunCmd(cfgPath)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"42"})

	err := cmd.Execute()
	// We expect this to fail because the real `gh` will reject the call (no auth
	// in test env) — but it must NOT fail at config-load time, and the error
	// message should not be the "TODO: not implemented" stub.
	if err == nil {
		// Could also succeed in environments where gh is set up; that's fine.
		return
	}
	if strings.Contains(err.Error(), "TODO") {
		t.Errorf("stub still wired; err: %v", err)
	}
}
