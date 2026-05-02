# Klanky

## What is Klanky?

Klanky is a personal CLI that orchestrates parallel Claude Code agents against issue DAGs hosted on GitHub Project v2 boards. You link one or more Projects to a repo, label the issues you want managed with `klanky:tracked`, and order them with GitHub's native "blocked by" relations. `klanky run --project <slug>` picks every eligible issue (open, tracked, unblocked), spawns one isolated `claude -p` per issue in its own git worktree, and each agent commits and opens a PR. The board reflects the result.

## Why this exists

AI coding tools tend toward two extremes: one interactive agent at a time, or one big autonomous "do the whole feature" run that's hard to review and harder to recover from. Klanky sits in between — a small runner that turns "here's a workstream; break it into independently-shippable issues with explicit dependencies, then run the unblocked ones in parallel" into something you can execute and inspect on a kanban board. One repo can host many boards (one per workstream), so multiple efforts don't crowd each other's lanes.

## Prerequisites

- `gh` CLI installed and authenticated. The token needs the `project` scope:
  ```bash
  gh auth refresh -s project
  ```
- `claude` CLI on your `PATH`. Klanky shells out to `claude -p` per issue.
- A GitHub repo you have write access to.
- Go 1.22+ — only if building from source.

## Install

### From a release (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/joshuapeters/klanky/main/install.sh | bash
```

Verify with `klanky version`.

### From source

```bash
git clone https://github.com/joshuapeters/klanky.git
cd klanky
go build -o klanky ./cmd/klanky
```

Move the resulting `klanky` binary somewhere on your `PATH`. Source builds report `version: dev`.

## Using Klanky

Run all commands from the root of the repo you want Klanky to manage. Per-repo state is written to `.klankyrc.json`.

1. **Bootstrap the repo.** Writes a minimal `.klankyrc.json` with no projects linked.

   ```bash
   klanky init --repo owner/name
   ```

2. **Register a project.** Either create a new conformant board or link an existing one. Each project is identified by a klanky `slug` (`[a-z0-9-]+`) that you'll pass to every project-scoped command afterward.

   ```bash
   klanky project new --slug auth --title "Auth System" --owner @me
   # or
   klanky project link <project-url> --slug auth
   ```

   `klanky project list` shows everything currently registered.

3. **Add issues.** Each `klanky issue add` creates a GitHub issue, applies `klanky:tracked`, attaches it to the project at `Status=Todo`, and wires up any "blocked by" relations. In normal use you don't run this by hand — your planning agent decomposes a workstream and calls `klanky issue add` for each issue it derives, with `--depends-on` expressing the order.

   ```bash
   klanky issue add --project auth --title "Wire up X" --body-file specs/wire-x.md
   klanky issue add --project auth --title "Wire up Y" --depends-on 42
   ```

4. **Run the project.** Klanky picks every issue that is open, tracked, and has no remaining open blockers; spawns one agent per eligible issue (capped at 5 concurrent); and prints a summary when each agent has either landed a PR or hit the per-issue timeout.

   ```bash
   klanky run --project auth
   ```

5. **Review and merge.** Reviewing PRs is always a human call. A merged PR with `Closes #N` in its body auto-closes the issue; the next `klanky run` reconciles the board.

For machine-readable output, pass `--output json` (or set `default_output: json` in `.klankyrc.json`) to any stdout-emitting command.

## Skills

Klanky ships agent skills that teach an AI coding agent how to use it.
They live under [`skills/`](skills/) in this repo and install in any
consumer repo via the open agent-skills CLI:

```
npx skills add joshuapeters/klanky
```

Three skills are bundled:

- `klanky-init` — bootstrap klanky in a fresh repo (one-time).
- `klanky-project` — create, link, and inspect klanky projects.
- `klanky-plan` — plan a workstream as a sized, dep'd klanky issue DAG.

These are for agents working *with* klanky in consumer repos. See
[`skills/README.md`](skills/README.md) for details.

## Contributing

PRs welcome. Conventions:

- **Conventional commit style** for PR titles and commit messages (`feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `ci:`).
- One logical change per PR. Keep them small enough to review.
- Tests for new behavior; existing tests must pass (`go test ./...`).

File issues if something is broken; don't expect a polished response — this is a personal tool in production use for one person.
