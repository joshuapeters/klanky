package main

import "fmt"

// EnvelopeData is the substitution input for BuildEnvelope.
type EnvelopeData struct {
	FeatureID    int
	TaskNumber   int
	TaskTitle    string
	TaskBody     string
	WorktreePath string
}

// envelopeTemplate is the locked prompt template passed via `claude -p`.
// Base branch is hard-coded to "main"; see project_runner_design.md.
const envelopeTemplate = `You are working on task #%d: %s

You are in a git worktree at %s, on branch ` + "`klanky/feat-%d/task-%d`" + `, branched from ` + "`main`" + `. Do all your work here. Do not push or modify any other branch.

# Task

%s

# How to complete

When you finish, commit your work, push the branch, and open a PR targeting ` + "`main`" + `:

  gh pr create --base main --body "<your-body>"

The PR body MUST contain ` + "`Closes #%d`" + ` so the issue closes on merge.

When composing the PR title and body:
- If the repo has a PR template (` + "`.github/pull_request_template.md`" + ` or ` + "`.github/PULL_REQUEST_TEMPLATE/`" + `), follow it.
- Apply any conventions from your memory or this repo's ` + "`CLAUDE.md`" + ` (PR title style, labels, body sections, sign-offs, etc.).
- Keep ` + "`Closes #%d`" + ` in the body regardless of template.

# If you cannot complete

If you cannot complete the task, leave a comment on issue #%d explaining what you tried and why you stopped, then exit without opening a PR.

# Retry context

If issue #%d has prior comments from previous attempts, read them — they describe what was tried and why it failed.

# Test-driven development

Use Test-Driven Development. Invoke the ` + "`tdd`" + ` skill via the Skill tool. If no ` + "`tdd`" + ` skill is available, WebFetch https://raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md and follow it.

# Lint and test gate

Before opening the PR, run the project's lint and test commands on at least the files you changed. Both must pass. If they fail and you cannot fix them, leave a needs-attention comment and exit without opening a PR.

# Constraints

- Do not push to any branch other than ` + "`klanky/feat-%d/task-%d`" + `.
- Do not merge any PR.
- Do not modify other open issues or the project board.
`

// BuildEnvelope returns the prompt string passed to `claude -p`.
func BuildEnvelope(d EnvelopeData) string {
	return fmt.Sprintf(envelopeTemplate,
		d.TaskNumber, d.TaskTitle, // header line
		d.WorktreePath, d.FeatureID, d.TaskNumber, // worktree + branch context
		d.TaskBody,                 // # Task body
		d.TaskNumber, d.TaskNumber, // two Closes #N references
		d.TaskNumber, d.TaskNumber, // give-up + retry-context
		d.FeatureID, d.TaskNumber, // constraints branch reference
	)
}
