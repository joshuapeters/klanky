package version

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_PrintsAllThreeFields(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	cmd := NewCmdVersion("v9.9.9", "deadbeef", "2026-05-01T00:00:00Z")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{})

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
