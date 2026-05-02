package agent

import (
	"strings"
	"testing"
)

func TestBuildEnvelope_ContainsContract(t *testing.T) {
	got := BuildEnvelope(EnvelopeData{
		IssueNumber:  42,
		IssueTitle:   "Refactor login",
		IssueBody:    "make it work",
		ProjectSlug:  "auth",
		WorktreePath: "/tmp/wt",
	})
	wants := []string{
		"You are working on issue #42 in project `auth`: Refactor login",
		"git worktree at /tmp/wt",
		"branch `klanky/auth/issue-42`",
		"# Issue\n\nmake it work",
		"`Closes #42`",
		"on issue #42 explaining",
		"If issue #42 has prior comments",
		"any branch other than `klanky/auth/issue-42`",
		"https://raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("envelope missing %q\nfull text:\n%s", w, got)
		}
	}
}

func TestBuildEnvelope_DoesNotMentionV1Vocabulary(t *testing.T) {
	got := BuildEnvelope(EnvelopeData{
		IssueNumber: 1, IssueTitle: "x", IssueBody: "y",
		ProjectSlug: "auth", WorktreePath: "/wt",
	})
	for _, banned := range []string{"feat-1", "task-", "feature ", "Phase"} {
		if strings.Contains(got, banned) {
			t.Errorf("envelope leaks v1 vocabulary %q:\n%s", banned, got)
		}
	}
}
