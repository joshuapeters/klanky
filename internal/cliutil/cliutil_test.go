package cliutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSONLine_WritesSingleLineJSON(t *testing.T) {
	out := &bytes.Buffer{}
	err := PrintJSONLine(out, map[string]any{
		"feature_id": 117,
		"url":        "https://example.com/issues/117",
	})
	if err != nil {
		t.Fatalf("PrintJSONLine: %v", err)
	}
	got := out.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output should end with newline, got %q", got)
	}
	if strings.Count(strings.TrimRight(got, "\n"), "\n") != 0 {
		t.Errorf("output must be single-line JSON: %q", got)
	}
	if !strings.Contains(got, `"feature_id":117`) {
		t.Errorf("missing feature_id: %q", got)
	}
}
