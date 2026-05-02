# AGENTS.md

Guidance for AI coding agents working in this repository.

## Working files go in `.docs/`

Put plans, design notes, brainstorming, scratch analysis, and any other
intermediate files you generate into `.docs/`. It is gitignored —
nothing under it is tracked or committed. Create the directory if it
doesn't already exist.

Do not commit working files, and do not place them at the repo root or
under any tracked directory.

## Klanky local state lives in two places

Klanky writes to two distinct locations, on purpose:

| State                                        | Location                                              | Sharing scope                              |
| -------------------------------------------- | ----------------------------------------------------- | ------------------------------------------ |
| Project bindings (slug ↔ project URL and IDs) | `<repo>/.klankyrc.json`                              | Shared — travels with `git clone`.        |
| Per-run ephemera (logs, locks, worktrees)    | `~/.klanky/<class>/<owner>/<repo>/<slug>/...`         | Per-machine, per-run.                      |

The asymmetry is deliberate. Project bindings are durable, identical
across machines, and need to be present on any machine that runs klanky
against this repo — including ephemeral compute (CI runners, EC2
instances spun up for autonomous agents) where `git clone && klanky run`
should just work without an out-of-band bootstrap. Logs and locks have
no inter-machine meaning and would only clutter the repo.

When in doubt: if a piece of state would be identical across every
checkout of this repo, it belongs in the repo. If it'd legitimately
differ per-machine, it belongs under `~/.klanky/`.

## `skills/` is the published consumer bundle

The `skills/` directory at the repo root contains agent skills that ship
to *consumer* repos via `npx skills add joshuapeters/klanky`. Their
audience is agents using klanky against arbitrary repos, not contributors
working on klanky itself.

When editing files in `skills/`, treat the audience accordingly:

- Assume the agent has zero prior context for klanky beyond the README
  and the skill it's reading.
- Don't reference klanky's source code, internal packages, or
  contributor conventions — they aren't visible to a consumer-side agent.
- Keep each skill self-contained. Skills are not loaded as a chain;
  any prerequisite knowledge has to live in the skill that needs it.
