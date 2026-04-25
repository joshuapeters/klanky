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
	for _, want := range []string{"init", "project", "feature", "task"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("expected --help to mention %q; got:\n%s", want, helpText)
		}
	}
}
