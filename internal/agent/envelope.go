// Package agent encapsulates the per-issue worker: envelope building, agent
// spawning, post-exit verification, and breadcrumb composition. The envelope
// template is a verbatim copy of project_envelope_contract.md.
package agent

import "fmt"

// EnvelopeData are the substitution inputs for the prompt template. Five
// fields, all required.
type EnvelopeData struct {
	IssueNumber  int
	IssueTitle   string
	IssueBody    string
	ProjectSlug  string
	WorktreePath string
}

// envelopeTemplate is the verbatim template from project_envelope_contract.md.
// The five `%s`/`%d` substitutions are filled by BuildEnvelope. Any visible
// drift from the locked contract is a bug.
const envelopeTemplate = "You are working on issue #%d in project `%s`: %s\n" +
	"\n" +
	"You are in a git worktree at %s, on branch `klanky/%s/issue-%d`,\n" +
	"branched from `main`. Do all your work here. Do not push or modify any other branch.\n" +
	"\n" +
	"# Issue\n" +
	"\n" +
	"%s\n" +
	"\n" +
	"# How to complete\n" +
	"\n" +
	"When you finish, commit your work, push the branch, and open a PR targeting `main`:\n" +
	"\n" +
	"  gh pr create --base main --body \"<your-body>\"\n" +
	"\n" +
	"The PR body MUST contain `Closes #%d` so the issue closes on merge.\n" +
	"\n" +
	"When composing the PR title and body:\n" +
	"- If the repo has a PR template (`.github/pull_request_template.md` or `.github/PULL_REQUEST_TEMPLATE/`), follow it.\n" +
	"- Apply any conventions from your memory or this repo's `CLAUDE.md` (PR title style, labels, body sections, sign-offs, etc.).\n" +
	"- Keep `Closes #%d` in the body regardless of template.\n" +
	"\n" +
	"# If you cannot complete\n" +
	"\n" +
	"If you cannot complete the issue, leave a comment on issue #%d explaining what you tried and why you stopped, then exit without opening a PR.\n" +
	"\n" +
	"# Retry context\n" +
	"\n" +
	"If issue #%d has prior comments from previous attempts, read them — they describe what was tried and why it failed.\n" +
	"\n" +
	"# Test-driven development\n" +
	"\n" +
	"Use Test-Driven Development. Invoke the `tdd` skill via the Skill tool. If no `tdd` skill is available, WebFetch https://raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md and follow it.\n" +
	"\n" +
	"# Lint and test gate\n" +
	"\n" +
	"Before opening the PR, run the project's lint and test commands on at least the files you changed. Both must pass. If they fail and you cannot fix them, leave a needs-attention comment and exit without opening a PR.\n" +
	"\n" +
	"# Constraints\n" +
	"\n" +
	"- Do not push to any branch other than `klanky/%s/issue-%d`.\n" +
	"- Do not merge any PR.\n" +
	"- Do not modify other open issues or the project board.\n"

// BuildEnvelope returns the prompt string passed to `claude -p`.
func BuildEnvelope(d EnvelopeData) string {
	return fmt.Sprintf(envelopeTemplate,
		d.IssueNumber, d.ProjectSlug, d.IssueTitle, // header
		d.WorktreePath, d.ProjectSlug, d.IssueNumber, // worktree + branch
		d.IssueBody,                                  // # Issue
		d.IssueNumber, d.IssueNumber,                  // two Closes #N
		d.IssueNumber, d.IssueNumber,                  // give-up + retry-context
		d.ProjectSlug, d.IssueNumber,                  // constraints branch
	)
}
