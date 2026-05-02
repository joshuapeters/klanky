# klanky skills

Agent skills bundled with klanky, installable in any consumer repo via:

```
npx skills add joshuapeters/klanky
```

## Skills

- **`klanky-init`** — Bootstrap klanky in a fresh repo (one-time): verify
  prerequisites, run `klanky init`, point at next steps.
- **`klanky-project`** — Create, link, and inspect klanky projects. Teaches
  the project schema (Status field + `klanky:tracked` label + GH-native
  `blocked by` deps) and the new-vs-link decision.
- **`klanky-plan`** — Plan a workstream as a klanky issue DAG. Sizing
  rubric, body checklist, decomposition decision tree, and a worked Auth
  example.

## Audience

These skills teach an agent how to *use* klanky in any repo where it's
installed — including klanky's own source repo, which is dogfooded with
klanky. Klanky must already be on `PATH`; see the top-level README for
install instructions.
