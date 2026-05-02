---
name: klanky-plan
description: Plan a workstream as a klanky issue DAG. Decompose the work into right-sized, vertically-sliced issues with explicit `--depends-on` ordering, and write each issue body to be a sufficient brief for the per-issue runner agent. Use when a user has a workstream (a feature, a migration, an initiative) they want to break into shippable issues for klanky's runner, or when they want to add issues to an existing klanky project.
---

# klanky-plan

Turn a workstream into a runnable klanky issue DAG. The output is a set
of GitHub issues, each labeled `klanky:tracked`, attached to one project,
with explicit `blocked by` relations forming a DAG that the runner can
pick from.

This skill assumes a klanky project already exists. If one doesn't, run
the `klanky-project` skill first.

## What the runner sees

Crucial framing: the per-issue runner agent only sees the issue *body*.
It gets a sealed envelope with that body and instructions to ship a PR.
It does NOT see the workstream description, the other issues, or this
planning conversation.

Everything the runner needs to do its job has to live in the body of the
issue you write. That's why body quality is load-bearing — see
[Body checklist](#body-checklist) below.

## Sizing rubric

A good klanky tracked issue is:

1. **Small** — narrow in scope.
2. **A vertical slice** — touches whatever layers it needs (DB, API, UI,
   …) to deliver something independently shippable. Not "build the
   schema", then "build the API", then "build the UI" as separate issues.
3. **Human-reviewable** — the resulting PR can be read end-to-end by a
   person in one sitting.
4. **Fits the agent's smart zone** — the issue's work plus the
   surrounding repo context an agent will load to do it stays inside the
   first ~100–200k tokens of context window, where the model is sharpest.

These constraints are **soft** — there's no lint rule. But if a planned
issue clearly violates one, that's the trigger to decompose it further.

## Body checklist

Every tracked issue body must contain enough for the runner agent to do
the work without upstream context. **Required:**

- **Outcome.** One sentence: what's true after this issue is merged.
- **Acceptance criteria.** Bullet list, testable.
- **Scope boundaries.** What's in. What's NOT in (deferred to which
  other issues, or out of scope entirely).
- **File / area pointers.** Where in the repo this work lives.
- **Test expectations.** What tests must exist or pass for this to be
  done.

**Optional, when relevant:**

- Pointers to related issues, design context, or prior PRs.
- External references (RFCs, library docs, ADRs).
- Sample inputs / expected outputs for tricky logic.

This is a checklist, not a literal template — assemble a body that
contains all required elements in whatever shape suits the issue.

## Mechanics

For each node in the DAG:

1. Write the body to a file. Convention:
   `.docs/issues/<slug>-<short-description>.md`. Keeps a paper trail and
   avoids shell-escaping pain.
2. Run:

   ```
   klanky issue add \
     --project <slug> \
     --title "<one-line title>" \
     --body-file .docs/issues/<slug>-<short>.md
   ```

3. For each predecessor, add `--depends-on <issue-number>` (repeatable).
   The flag takes the GitHub issue number that this new issue is blocked
   by. You'll know predecessor numbers because they were added earlier in
   this session and `klanky issue add` prints the new issue number.
4. After all nodes are added, verify the DAG by browsing the project
   board on GitHub or running `klanky project list` (then visiting the
   URL).

Always add foundation issues (no deps) first, then their dependents in
topological order. Don't try to add an issue with `--depends-on N` before
issue N exists.

## Decomposition decision tree

When a candidate issue feels too big, work through these in order:

1. **Can I cut horizontally (DB → API → UI)?** → No. Vertical slices
   only. Horizontal cuts can't ship in isolation and defer integration
   risk to the end of the workstream.
2. **Can I cut by feature surface (one auth provider at a time, one
   resource type at a time)?** → Usually yes. Share a foundation issue
   and `--depends-on` it from each surface.
3. **Can I cut by user-visible flow (signup vs. login vs. password
   change)?** → Usually yes. Each flow tends to be its own vertical
   slice.
4. **Can I cut by data scope (read path vs. write path)?** → Sometimes;
   works best when the read path can ship and be useful to users before
   writes exist.

If none of these apply and the issue still violates the rubric, the
workstream itself may need rethinking — flag it to the user before
proceeding.

## Anti-patterns

- **Horizontal slicing** — "build the database", "build the API", "build
  the UI" as separate issues. Nothing ships independently, and
  integration is deferred.
- **"Spike" or "research" issues** — the planner's job is to know what
  to build. Spikes belong in interactive sessions, not the runner queue.
- **Empty bodies** — the runner only sees the body. Empty body =
  unrunnable. Confirm every issue has all required body elements before
  adding.
- **Missing deps** — the runner picks unblocked issues in parallel.
  Missing deps mean two agents will trample each other and produce
  conflict-ridden PRs.
- **Mega-issues that "we'll just split later"** — split now. Once an
  issue is in flight, it's hard to retract.

## Worked example: Auth as a DAG

**Workstream:** "Add user authentication to this app."

A naive single-issue framing fails the rubric on every count: not
vertically sliceable as a whole, blows past the smart zone, the resulting
PR would not be human-reviewable. Decompose.

| #  | Issue                                  | Deps  | Why it's a vertical slice                                              |
|----|----------------------------------------|-------|------------------------------------------------------------------------|
| 1  | Auth backend skeleton                  | —     | `User` table + password-hashing util + session model + tests. No UI.   |
| 2  | User-pass signup + login UI            | 1     | Forms → backend → session cookie → redirect. End-to-end signup flow.   |
| 3  | Account confirmation email flow        | 1     | Token gen + email send + confirm endpoint. Independent of #2's UI.     |
| 4  | Logout + session expiry                | 2     | Endpoint + UI affordance. Builds on the active-session world from #2.  |
| 5  | Username / password change             | 2     | Settings form + current-password re-verify + update endpoint.          |
| 6  | OAuth (Google) sign-in                 | 1, 2  | Alternate IdP plugged into the same login surface from #2.             |
| 7  | SSO / SAML                             | 6     | Extends OAuth scaffolding to enterprise providers.                     |

Why this works:

- Each issue is a **vertical slice**: it touches whatever layers it
  needs to deliver something a user can exercise (or, for #1, a
  foundation other issues can build on).
- Deps prevent premature parallelism. #4 needs the active-session world
  from #2; #6 needs both #1's user model and #2's login surface.
- #2 and #3 are dep'd on #1 but **not on each other** — the runner can
  pick them up in parallel once #1 lands.
- Every issue is reviewable as one PR.
- Every issue's surface area fits comfortably inside the smart zone.

### A sample body for issue #1 (Auth backend skeleton)

```
# Outcome
The codebase has the data + crypto primitives needed to authenticate
users with username + password, with no UI yet.

# Acceptance criteria
- A `users` table exists with: `id`, `email` (unique, indexed),
  `password_hash`, `created_at`, `updated_at`.
- A `sessions` table exists with: `id`, `user_id` (FK → users.id),
  `expires_at`, `created_at`.
- A `hashPassword(plain) -> hashed` utility uses bcrypt with cost 12.
- A `verifyPassword(plain, hashed) -> bool` utility round-trips.
- A `createSession(userId) -> sessionId` utility writes to the table.
- A `getActiveSession(sessionId) -> Session | null` reads and respects
  `expires_at`.

# Scope boundaries
- **In:** schema migrations, password hashing, session create/read.
- **NOT in:** any HTTP routes, any UI, email confirmation (issue #3),
  session invalidation / logout (issue #4), OAuth (issue #6).

# File / area pointers
- `db/migrations/` for the schema migrations.
- `internal/auth/password.{go,ts,py}` for hashing utils.
- `internal/auth/session.{go,ts,py}` for session CRUD.

# Test expectations
- Migration up + down round-trips cleanly.
- `verifyPassword(p, hashPassword(p))` returns true.
- `verifyPassword("wrong", hashPassword(p))` returns false.
- `getActiveSession` returns null after `expires_at` passes.
- All `go test ./internal/auth/...` (or equivalent) pass.
```

The body is dense, but every section earns its keep — the runner agent
has no way to ask follow-up questions, so this is everything it gets.

### Building the DAG

In sequence:

```
klanky issue add --project auth \
  --title "Auth backend skeleton" \
  --body-file .docs/issues/auth-backend-skeleton.md
# → created issue #1

klanky issue add --project auth \
  --title "User-pass signup + login UI" \
  --body-file .docs/issues/auth-userpass-ui.md \
  --depends-on 1
# → created issue #2

klanky issue add --project auth \
  --title "Account confirmation email flow" \
  --body-file .docs/issues/auth-confirm-email.md \
  --depends-on 1
# → created issue #3

# ... and so on, each with the correct --depends-on numbers
```

Verify on the board:

```
klanky project list
# visit the URL for slug=auth and confirm the DAG looks right
```

Now the project is ready. The user runs `klanky run --project auth` to
spawn agents against unblocked issues.
