package root

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_Help_ListsRegisteredSubcommands(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := NewCmdRoot(".klankyrc.json", "dev", "none", "unknown")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help failed: %v", err)
	}
	for _, want := range []string{"init", "issue", "project", "run", "version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected --help to mention %q; got:\n%s", want, out.String())
		}
	}
}

func TestRootCommand_VersionFlag_PrintsVersion(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := NewCmdRoot(".klankyrc.json", "dev", "none", "unknown")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(out.String(), "dev") {
		t.Errorf("expected 'dev' in --version output; got %q", out.String())
	}
}
