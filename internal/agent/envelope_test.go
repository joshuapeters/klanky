package agent

import (
	"strings"
	"testing"
)

func TestBuildEnvelope_SubstitutesTaskFields(t *testing.T) {
	got := BuildEnvelope(EnvelopeData{
		FeatureID:    7,
		TaskNumber:   42,
		TaskTitle:    "Add login form",
		TaskBody:     "## Context\nWe need login.",
		WorktreePath: "/home/u/.klanky/worktrees/proj/feat-7/task-42",
	})

	wantSubstrs := []string{
		"task #42: Add login form",
		"branch `klanky/feat-7/task-42`",
		"branched from `main`",
		"/home/u/.klanky/worktrees/proj/feat-7/task-42",
		"## Context\nWe need login.",
		"gh pr create --base main",
		"Closes #42",
		"`.github/pull_request_template.md`",
		"`.github/PULL_REQUEST_TEMPLATE/`",
		"CLAUDE.md",
		"comment on issue #42",
		"prior comments from previous attempts",
		"Test-Driven Development",
		"raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md",
		"lint and test",
		"Do not push to any branch other than `klanky/feat-7/task-42`",
		"Do not merge any PR",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing substring %q", want)
		}
	}
}

func TestBuildEnvelope_BodyPlacedVerbatim(t *testing.T) {
	body := "## Context\nFoo\n\n## Acceptance criteria\n- [ ] Bar\n\n## Out of scope\nBaz"
	got := BuildEnvelope(EnvelopeData{
		FeatureID: 1, TaskNumber: 1, TaskTitle: "T",
		TaskBody: body, WorktreePath: "/wt",
	})
	if !strings.Contains(got, body) {
		t.Errorf("body not placed verbatim; envelope:\n%s", got)
	}
}
