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

These skills are for agents working *with* klanky in consumer repos — not
for agents hacking on klanky's own source. Klanky must already be installed
and on `PATH`; see the top-level README for install instructions.
