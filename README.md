# Klanky

## What is Klanky?

Klanky is a personal CLI that orchestrates parallel Claude Code agents against a task graph stored as GitHub issues on a GitHub Project. The intended workflow is: you plan a feature in conversation with a Claude Code agent, the agent decomposes it into a Feature → Phases → Tasks structure by calling `klanky` subcommands, and then you run `klanky run <feature-id>` to execute the current phase — Klanky spawns one isolated `claude -p` per task in its own git worktree, each agent commits and opens a PR, and the GitHub Project board reflects the result.

## Why this exists

Existing AI-coding tools tend to either lock you into one agent at a time (interactive sessions, single thread of work) or one big monolithic plan (autonomous "do the whole feature" runs that are hard to review and harder to recover from when one step is wrong). Klanky is the substrate in between: a small, opinionated runner that turns "here's a feature; break it down into independently-shippable tasks and run that work in parallel" into something you can actually execute and inspect on a kanban board.

## Prerequisites

- Go 1.22+ — only required if you build from source. Otherwise, download a release binary from the [releases page](https://github.com/joshuapeters/klanky/releases).
- `gh` CLI installed and authenticated. The token needs the `project` scope:
  ```bash
  gh auth refresh -s project
  ```
- `claude` CLI on your `PATH`. Klanky shells out to `claude -p` per task.
- A GitHub repository you own (or have write access to) and a GitHub Project to attach. `klanky init` can create the project for you; otherwise you can link an existing conformant project.

## Install

### From a release (recommended)

Download the appropriate tarball for your platform from the [releases page](https://github.com/joshuapeters/klanky/releases), extract it, and put the `klanky` binary on your `PATH`:

```bash
# Replace VERSION, OS (darwin|linux), and ARCH (amd64|arm64) for your platform.
curl -L "https://github.com/joshuapeters/klanky/releases/download/vVERSION/klanky_VERSION_OS_ARCH.tar.gz" \
  | tar -xz -C /usr/local/bin klanky
```

Verify with:

```bash
klanky version
```

### From source

```bash
git clone https://github.com/joshuapeters/klanky.git
cd klanky
go build -o klanky .
```

Move the resulting `klanky` binary somewhere on your `PATH`. Source builds report `version: dev`.

## Quick start

Run all commands from the root of the repository you want Klanky to manage. State is written to `.klankyrc.json` in that directory (gitignored — it's per-repo local state).

1. Bootstrap a new GitHub Project with the required schema, or link an existing one.

   ```bash
   klanky init --owner @me --repo owner/name --title "My Project"
   ```

   `--owner` accepts `@me` (the current GitHub user) or an organization login. If you already have a conformant project:

   ```bash
   klanky project link <project-url> --repo owner/name
   ```

   Either command writes `.klankyrc.json` and is the prerequisite for everything below.

2. Create a Feature. The output is a single line of JSON your planning agent can parse.

   ```bash
   klanky feature new --title "Some feature"
   # {"feature_id":42,"url":"https://github.com/owner/name/issues/42"}
   ```

   `--body-file <path>` is optional and attaches a markdown body to the issue.

3. Add tasks under the feature. Each task is a sub-issue with a phase number and a markdown spec file that becomes the task's body.

   ```bash
   klanky task add --feature 42 --phase 1 --title "Wire up X" --spec-file specs/wire-x.md
   # {"task_id":43,"url":"https://github.com/owner/name/issues/43"}
   ```

   In normal use you don't run this by hand — you talk to a planning agent in a chat, and it calls `klanky task add` for each task it derives from the feature. (Klanky will eventually ship a `PLANNING.md` prompt fragment for the planning agent to read; for now the planning conversation is freeform.)

4. Execute the current phase.

   ```bash
   klanky run 42
   ```

   Klanky picks the lowest open phase, spawns one agent per work-eligible task in parallel (capped at 5), and prints a summary table when every agent has either landed a PR or hit the per-task timeout.

## What `klanky run` actually does

For a given feature ID, in order:

1. **Lock.** Acquires a per-feature lock file at `.klanky/runner-<F>.lock`. If another `klanky run <F>` is in flight, this one exits immediately.
2. **Snapshot.** Fetches the project board state for the feature in one batched GraphQL call plus one `gh pr list`, so the entire run operates on a consistent view.
3. **Reconcile.** Compares each task's `Status` field against the underlying issue/PR state and corrects drift — for example, if a PR was merged outside Klanky, the corresponding task's Status is moved to `Done`. Reconcile actions leave a breadcrumb comment on the affected issue.
4. **Select work.** Picks the lowest phase that has any non-`Done` tasks. Within that phase, work-eligible tasks are those in `Todo` or `Needs Attention`. Tasks already in `In Review` are reported in the summary but not re-run.
5. **Spawn agents in parallel.** For each eligible task, up to 5 at a time:
   - Creates a fresh worktree at `~/.klanky/worktrees/<repo>/feat-<F>/task-<T>/` (existing path is wiped first).
   - Sets `Status = In Progress` on the task's project item.
   - Spawns `claude -p` in the worktree with a per-task envelope describing the feature, task, and required output (commit + push + PR).
   - Waits up to 20 minutes per task.
6. **Verify and report.** After each agent exits, Klanky checks the branch has commits beyond `main` and a PR exists. If yes, sets `Status = In Review` and records the PR link. If not (or on timeout, spawn failure, or worktree-setup failure), sets `Status = Needs Attention` and posts a breadcrumb comment with the tail of the agent's log so you can see what happened.
7. **Summary.** Prints a one-line-per-task table with the outcome and the PR or issue URL.

## The schema

Klanky depends on a specific GitHub Project layout:

| Element | Type | Required values |
| --- | --- | --- |
| `Phase` field | Number | (any positive integer per task) |
| `Status` field | Single-select | `Todo`, `In Progress`, `In Review`, `Needs Attention`, `Done` (exact strings, case-sensitive) |
| `klanky:feature` label | Repo label | Applied to parent feature issues |
| Task issues | Sub-issues of features | Linked via the GitHub sub-issue relation |

`klanky init` creates all of this. If you'd rather wire it up by hand on an existing project and then `klanky project link` it, the table above is the contract — `project link` validates and refuses to write `.klankyrc.json` if anything is missing.

## Status mirror, not source of truth

The truth for "is this task done?" is whether the underlying GitHub issue is closed. Closing happens automatically when a PR with `Closes #<task-id>` in its body is merged. The `Status` field on the project board is a mirror that Klanky maintains for visibility — every `klanky run` reconciles drift between the field and the issue/PR state before doing anything else. This means it's safe to merge PRs and close issues outside of Klanky; the next run will fix up the board.

## Branch and PR conventions

- Branches are named `klanky/feat-<F>/task-<T>`.
- The base branch is hard-coded to `main`. Worktrees branch from `main` at the time of the run.
- The agent is responsible for putting `Closes #<task-id>` in its PR body. Klanky does not enforce this, but without it the issue won't auto-close on merge and the next run will see the task as still open.

## What Klanky does not do

- No daemon. `klanky run` is a one-shot command; nothing watches the board between runs.
- No auto-advance between phases. When phase 1 is fully `Done`, you re-run `klanky run <F>` to start phase 2.
- No auto-merge of PRs. Reviewing and merging is always a human call.
- No plugin system, no hooks, no extension points.
- No support for non-GitHub backends. Issues, PRs, Projects v2, and the `gh` CLI are baked in.
- No support for non-Claude agents. The runner shells out to `claude -p` specifically.

## Status

v1 personal tool. The user (you, or me, depending on who you are) is using it on your personal machine. It is in production use but tightly scoped to one person, one repo, one machine. Generalization will happen if multiple people start asking for it. File issues if something is broken; don't expect a polished response.
