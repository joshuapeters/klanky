---
name: klanky-project
description: Create, link, and inspect klanky projects. One klanky project equals one workstream's GitHub Project v2 board, and a repo can host many. Use when a user wants to start a new klanky-managed initiative, link an existing GitHub Project to klanky, list registered projects, or understand what a klanky project is. Teaches the project schema (Status field + klanky:tracked label + GH-native blocked-by deps) and the new-vs-link decision.
---

# klanky-project

A **klanky project** is one GitHub Projects v2 board scoped to one
workstream or initiative. A repo can have many klanky projects — they
keep parallel workstreams from crowding each other on a single kanban
board, and the runner operates on one project at a time
(`klanky run --project <slug>`).

## What's in a project

Every klanky project has three required components on the GitHub side:

1. **A `Status` single-select field** with the values `Todo`,
   `In Progress`, and `Done`. Klanky reads and writes this to track issue
   lifecycle.
2. **A `klanky:tracked` repo-level label.** This label is the *allowlist* —
   klanky only acts on issues that carry it. Issues without the label are
   invisible to klanky, even if they're on the board.
3. **GitHub-native `blocked by` issue relations.** Klanky uses these as
   DAG edges. A tracked issue is *eligible* for the runner only when all
   of its `blocked by` issues are closed.

Together, these make the GitHub board itself the contract — klanky stores
no shadow state of its own about issue status or dependencies. If you can
read the board, you can predict what klanky will do.

## Slug rules

Every klanky project has a **slug** that you pass to all project-scoped
commands (`--project <slug>`). It's also embedded in branch paths
(`klanky/<slug>/issue-<N>`), so it must be filesystem-and-git-safe.

- Charset: `[a-z0-9-]+`.
- Stable: pick something you won't want to rename. The slug is a permanent
  identifier locally; renames mean editing `.klankyrc.json` and migrating
  branch names by hand.
- Short: it shows up in branch names and CLI flags. `auth` beats
  `authentication-system-rewrite-2026`.

## The new-vs-link decision

| Situation | Command |
|---|---|
| No board exists yet for this workstream. Klanky should create a fresh, conformant one. | `klanky project new --slug <s> --title "..." --owner @me` |
| A board already exists on GitHub (someone made it, or it predates klanky). | `klanky project link <project-url> --slug <s>` |

`project new` creates the board with the required schema baked in: Status
field, `klanky:tracked` label, ready to go.

`project link` validates the existing board against the schema. If the
board is missing the Status field with the right values, or the
`klanky:tracked` label, link will tell you what's missing and exit. Fix
the board on GitHub, then re-run.

When in doubt, prefer `project new`. Linking is for cases where humans on
the team are already using a board you want to keep.

## List

```
klanky project list
```

Prints every project registered in `.klankyrc.json` with slug, title, and
GitHub Project URL. Use this to confirm registration after `new` or
`link`, or to remind yourself what slugs exist.

## Inline example

Starting an `auth` initiative in a freshly-bootstrapped repo:

```
$ klanky project new --slug auth --title "Auth System" --owner @me
created project: https://github.com/users/jp/projects/12
registered slug: auth
schema: ✓ Status field, ✓ klanky:tracked label

$ klanky project list
SLUG  TITLE        URL
auth  Auth System  https://github.com/users/jp/projects/12
```

The `auth` project is now ready. It's empty — no issues yet. The next
step is populating it with a sized, dep'd issue DAG.

## Next step

Hand off to the `klanky-plan` skill. Given a workstream description, it
walks through decomposing it into right-sized tracked issues with proper
dependencies and runner-ready bodies, then calls `klanky issue add` for
each.
