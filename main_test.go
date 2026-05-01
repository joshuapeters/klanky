package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_Help_ListsAllSubcommands(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --help to succeed, got: %v", err)
	}

	helpText := out.String()
	for _, want := range []string{"init", "project", "feature", "task", "run"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("expected --help to mention %q; got:\n%s", want, helpText)
		}
	}
}

func TestRootCommand_Help_IncludesVersion(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --help to succeed, got: %v", err)
	}

	helpText := out.String()
	if !strings.Contains(helpText, "Version:") {
		t.Errorf("expected --help to contain a 'Version:' line; got:\n%s", helpText)
	}
	if !strings.Contains(helpText, "dev") {
		t.Errorf("expected --help to print version 'dev' (the default); got:\n%s", helpText)
	}
}

func TestRootCommand_VersionFlag_PrintsVersion(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --version to succeed, got: %v", err)
	}

	flagOut := out.String()
	if !strings.Contains(flagOut, "dev") {
		t.Errorf("expected --version to print 'dev' (the default); got:\n%s", flagOut)
	}
}
