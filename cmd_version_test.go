package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_PrintsAllThreeFields(t *testing.T) {
	// Override the package-level build vars so the test is deterministic
	// regardless of how the binary was built. Restore them after.
	origV, origC, origD := version, commit, date
	version = "v9.9.9"
	commit = "deadbeef"
	date = "2026-05-01T00:00:00Z"
	defer func() { version, commit, date = origV, origC, origD }()

	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected `version` to succeed, got: %v", err)
	}

	got := out.String()
	for _, want := range []string{"v9.9.9", "deadbeef", "2026-05-01T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected `version` output to contain %q; got:\n%s", want, got)
		}
	}
}

func TestRootCommand_Help_ListsVersionSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --help to succeed, got: %v", err)
	}

	if !strings.Contains(out.String(), "version") {
		t.Errorf("expected --help to list the `version` subcommand; got:\n%s", out.String())
	}
}
