---
name: klanky-init
description: Bootstrap klanky in a fresh repo. Verify prerequisites (gh CLI with project scope, claude CLI on PATH), run `klanky init --repo owner/name`, and point the user at next steps. Use when a user asks to set up klanky in this repo, when starting a klanky workflow, or when `.klankyrc.json` does not exist.
---

# klanky-init

Bootstrap klanky in this repo. One-time per repo. After this, register
projects with the `klanky-project` skill, then populate them with
`klanky-plan`.

## Prerequisites

Klanky shells out to `gh` and `claude`. Verify both before running init.

### 1. `gh` CLI with `project` scope

Klanky reads and writes GitHub Projects v2 boards, which requires the
`project` OAuth scope. The default `gh auth login` does NOT include it.

Check current scopes:

```
gh auth status
```

If the output does not include `project` in the list of token scopes,
refresh:

```
gh auth refresh -s project
```

Then re-run `gh auth status` to confirm `project` is now listed.

If `gh` itself is not installed, see https://cli.github.com.

### 2. `claude` CLI on `PATH`

Klanky spawns `claude -p` per issue.

```
command -v claude
```

If this prints a path, you're set. If it prints nothing, install Claude
Code from https://docs.claude.com/en/docs/claude-code.

### 3. `klanky` itself

```
command -v klanky
```

If missing, see klanky's README for install instructions
(https://github.com/joshuapeters/klanky).

## Init

Once all three CLIs are present, run from the repo root:

```
klanky init --repo <owner>/<name>
```

Replace `<owner>/<name>` with the GitHub `owner/repo` slug. This writes
`.klankyrc.json` at the repo root with no projects linked yet.

## What happened

Klanky now knows two things about this repo:

- **Repo binding** — `.klankyrc.json` exists with the `owner/repo` you
  passed. This file is committed and travels with the repo, so any machine
  that runs `git clone && klanky run` works without re-bootstrapping.
- **Per-machine state location** — Logs, locks, and worktrees will live
  under `~/.klanky/<class>/<owner>/<repo>/<slug>/...` once you start
  running. Nothing is created there yet.

The `.klankyrc.json` file is intentionally minimal at this point — no
projects are linked. That's the next step.

## Next step

Register a project. Two paths:

- A board does not yet exist for the workstream you want to manage (most
  common): use the `klanky-project` skill to run `klanky project new`.
- A board already exists on GitHub for this workstream: use the
  `klanky-project` skill to run `klanky project link <url>`.

Either way, hand off to the `klanky-project` skill from here.

## Inline example

Fresh repo, no klanky setup yet:

```
$ gh auth status
github.com
  ✓ Logged in to github.com account jp
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'
  # ^ no `project` scope

$ gh auth refresh -s project
# browser flow; approve project scope
✓ Authentication complete.

$ command -v claude
/Users/jp/.local/bin/claude

$ command -v klanky
/usr/local/bin/klanky

$ klanky init --repo myorg/myapp
wrote .klankyrc.json (repo: myorg/myapp, projects: 0)

$ cat .klankyrc.json
{
  "repo": "myorg/myapp",
  "projects": []
}
```

Now hand off to `klanky-project`.
