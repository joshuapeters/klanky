# Klanky Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `klanky run <feature-id>` command — the AFK orchestrator that picks up the current phase of a feature, spawns parallel `claude -p` agents (one per task), and lands either an in-review PR or a needs-attention breadcrumb for each one.

**Architecture:** Single-package addition to the existing `package main` at the repo root. New files cover snapshot fetching (one batched GraphQL + per-feature PR list), pure-function reconcile + work-queue selection, worktree management via shell-out to `git`, `claude -p` subprocess execution behind a `Spawner` interface, status writes with retry, breadcrumb posting, stderr progress lines, and `text/tabwriter` summary on stdout. All gh/git interaction goes through the existing `Runner` interface so unit tests inject `FakeRunner` instead of executing real shell commands; subprocess spawning gets a sibling `Spawner` interface with a `FakeSpawner` for tests.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra`, `golang.org/x/sync/errgroup`, `golang.org/x/sync/semaphore`, stdlib only otherwise (`os/exec`, `text/tabwriter`, `encoding/json`, `testing`).

---

## Context (locked design decisions you need to know)

This plan implements the runner spec locked in user memory under `project_runner_design.md` and `project_locked_design.md`. Key points the implementing engineer needs without reading those:

**Source of truth:** **issue-closed = Done.** The runner never asks "is this PR merged?" — it asks "is this issue closed?". The Status single-select field on the project is a runner-maintained mirror; reconcile catches up any drift on each run.

**Work eligibility:** tasks where `Status ∈ {Todo, Needs Attention}` are work-eligible. `needs-attention` tasks are auto-retried; the agent reads prior comments via the envelope's retry-context clause. The breadcrumb on each retry uses an `<!-- klanky-attempt -->` HTML-comment sentinel so attempt count is reliably derivable.

**Phase selection:** lowest phase number with at least one open issue. No `--phase` flag.

**Branch naming:** `klanky/feat-<F>/task-<T>`. All PRs target `main` (hard-coded, no `--base` flag).

**Worktree path:** `~/.klanky/worktrees/<repo-name>/feat-F/task-T/`. On retry, wipe and rebuild clean. needs-attention worktrees are *only* preserved at the moment of transition into needs-attention (so a human can inspect); subsequent retries rebuild.

**Log path:** `<repo>/.klanky/logs/task-<T>.log`, truncated on each attempt.

**Lock file:** `<repo>/.klanky/runner-<feature-id>.lock`, holds JSON `{"pid": N, "started_at": "..."}`. `O_CREATE|O_EXCL`. On EEXIST: alive PID → refuse; dead PID → silent takeover.

**Concurrency cap:** hard-coded 5 parallel agents via `errgroup` + `semaphore.NewWeighted(5)`.

**Per-task timeout:** 20 minutes via `context.WithTimeout`. On timeout: SIGKILL the process group.

**Subprocess:** `claude -p <envelope> --permission-mode bypassPermissions`, cwd = worktree, env includes `GH_REPO=<owner/name>`. Process group set so we can signal the whole tree.

**Reconcile matrix:** see `reconcile.go` task below for the 11-row table.

**Output split:** progress events on stderr, end-of-run table on stdout. Exit codes: 0 on any normal outcome, 1 on setup error, 130 on Ctrl-C.

**Existing surface (already built — do not modify unless a step says so):**
- `config.go` — `Config`, `LoadConfig`, `SaveConfig`. Already has all the field IDs you need.
- `ghcli.go` — `Runner` interface, `RealRunner`, `FakeRunner`, `RunGraphQL[T]` helper.
- `schema.go` — Schema constants (`StatusOptions`, `FieldNamePhase`, etc.).
- `output.go` — `PrintJSONLine`.
- `cmd_init.go`, `cmd_projectlink.go`, `cmd_featurenew.go`, `cmd_taskadd.go` — the 4 existing commands. The runner doesn't depend on these; they use the same `Runner`/`FakeRunner` pattern you should follow.
- `main.go` — `newRootCmd()` registers the 4 existing subcommands. You will modify this to register `run`.

**TDD:** every task in this plan follows Red → Green → Commit. No "implement, then write tests later." If you find yourself wanting to skip the test-first step, the `tdd` skill (https://raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md) governs.

---

## File Structure

13 new source files + 13 new test files, plus a 1-line modification to `main.go`. Flat layout matches the existing convention.

| File | Responsibility |
|---|---|
| `cmd_run.go` | Cobra `run` subcommand: parse `<feature-id>`, load config, dispatch to `RunFeature` |
| `cmd_run_test.go` | Tests for arg parsing, missing-config error, dispatch wiring |
| `snapshot.go` | `Snapshot`, `TaskInfo`, `PRInfo`, `FetchSnapshot()` — the one batched GraphQL + the PR list call |
| `snapshot_test.go` | Tests with `FakeRunner` covering field-value extraction (Phase number, Status single-select, missing values), missing project items, sub-issue cap |
| `lock.go` | `AcquireLock`, `ReleaseLock`, JSON lock file with PID + started_at, takeover semantics |
| `lock_test.go` | Tests for create / EEXIST-with-live-PID (refuse) / EEXIST-with-dead-PID (takeover) / cleanup |
| `reconcile.go` | Pure-function `Reconcile(Snapshot) []ReconcileAction` implementing the 11-row matrix |
| `reconcile_test.go` | Table-driven tests, one row per matrix entry |
| `workqueue.go` | Pure-function `SelectWork(Snapshot, []ReconcileAction) WorkQueueResult` returning current phase, eligible tasks, all-done flag |
| `workqueue_test.go` | Tests for phase selection, eligibility filtering, the 3 nothing-to-do scenarios |
| `worktree.go` | `EnsureCleanWorktree(ctx, runner, repoRoot, worktreePath, branch, base)` — rm-if-exists + git worktree prune + git worktree add |
| `worktree_test.go` | Tests with `FakeRunner` for fresh-create and wipe-then-create paths |
| `envelope.go` | `BuildEnvelope(EnvelopeData) string` — the locked template via `fmt.Sprintf` |
| `envelope_test.go` | Golden-output tests covering substitution and that all required sections are present |
| `agent.go` | `Spawner` interface, `RealSpawner`, `FakeSpawner`, `RunAgent(ctx, spawner, runner, ...)` end-to-end (spawn + verify branch + verify PR), returns `TaskResult` |
| `agent_test.go` | Tests for spawn args, timeout handling, post-exit verification (branch missing, PR missing, both present), `TaskResult` shape |
| `breadcrumb.go` | `BuildBreadcrumb(BreadcrumbData) string`, `CountPriorAttempts(ctx, runner, repo, taskNumber) int`, `PostBreadcrumb(ctx, runner, repo, taskNumber, body)` |
| `breadcrumb_test.go` | Format tests, attempt-count tests (0, 1, N comments), post-call shape tests |
| `statuswrite.go` | `WriteStatus(ctx, runner, cfg, itemID, statusName) error` with 3-retry exponential backoff |
| `statuswrite_test.go` | Success on first try, success after retries, give-up after 3 failures |
| `progress.go` | `ProgressLogger` writing `[HH:MM:SS] <message>` to a configurable `io.Writer` (stderr in real use); typed event methods |
| `progress_test.go` | Tests against a `bytes.Buffer` verifying line format and that each event method emits the right line |
| `summary.go` | `RenderSummary(SummaryData, io.Writer)` — `text/tabwriter` table + counts + dynamic next-step footer |
| `summary_test.go` | Golden tests for each outcome mix (all in-review, mixed, all needs-attention, feature complete) |
| `runner.go` | `RunFeature(ctx, runner, spawner, cfg, repoRoot, featureID, progress, summaryOut) error` — top-level orchestration: lock → fetch → reconcile → apply → workqueue → spawn errgroup → summary |
| `runner_test.go` | Integration tests with `FakeRunner` + `FakeSpawner` driving end-to-end happy path and edge cases |
| `main.go` (MODIFY) | Add `root.AddCommand(newRunCmd(cfgPath))` |

---

## Task 1: Bootstrap the `run` cobra command

**Files:**
- Create: `/Users/jp/Source/klanky/cmd_run.go`
- Create: `/Users/jp/Source/klanky/cmd_run_test.go`
- Modify: `/Users/jp/Source/klanky/main.go`

The goal of this task is to land a stub `klanky run <feature-id>` that loads config, validates the arg, and prints "TODO: not implemented" so the rest of the plan has a place to wire into.

- [ ] **Step 1: Write failing tests for arg parsing and config loading**

Create `cmd_run_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCmd_RequiresFeatureIDArg(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from missing arg, got nil")
	}
}

func TestRunCmd_RejectsNonNumericFeatureID(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"abc"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from non-numeric arg, got nil")
	}
	if !strings.Contains(err.Error(), "feature-id") {
		t.Errorf("error should mention feature-id: %v", err)
	}
}

func TestRunCmd_MissingConfig_ReturnsHelpfulError(t *testing.T) {
	cmd := newRunCmd("/nonexistent/.klankyrc.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"42"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from missing config, got nil")
	}
	if !strings.Contains(err.Error(), ".klankyrc.json") {
		t.Errorf("error should mention .klankyrc.json: %v", err)
	}
}

func TestRunCmd_ValidArgs_LoadsConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	contents := `{
		"schema_version": 1,
		"repo": {"owner": "alice", "name": "proj"},
		"project": {
			"url": "https://github.com/users/alice/projects/1",
			"number": 1, "node_id": "PVT_x",
			"owner_login": "alice", "owner_type": "User",
			"fields": {
				"phase":  {"id": "PVTF_p", "name": "Phase"},
				"status": {"id": "PVTSSF_s", "name": "Status",
					"options": {"Todo": "a", "In Progress": "b", "In Review": "c", "Needs Attention": "d", "Done": "e"}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(cfgPath, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	cmd := newRunCmd(cfgPath)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"42"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out.String(), "TODO: not implemented") {
		t.Errorf("expected stub output; got: %s", out.String())
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure (`newRunCmd undefined`)**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `cmd_run.go` with the stub**

Create `cmd_run.go`:

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newRunCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <feature-id>",
		Short: "Execute the current phase of a feature: spawn parallel agents, open PRs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			featureID, err := strconv.Atoi(args[0])
			if err != nil || featureID < 1 {
				return fmt.Errorf("feature-id must be a positive integer, got %q", args[0])
			}

			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"TODO: not implemented (would run feature #%d in repo %s/%s)\n",
				featureID, cfg.Repo.Owner, cfg.Repo.Name)
			return nil
		},
	}
	return cmd
}
```

- [ ] **Step 4: Wire into `main.go`**

Edit `main.go`:

```go
// Add this line to newRootCmd, after the existing AddCommand calls:
root.AddCommand(newRunCmd(cfgPath))
```

The full updated `newRootCmd` should read:

```go
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "klanky",
		Short:        "Orchestrate parallel coding agents against a GitHub-issue task graph",
		SilenceUsage: true,
	}

	cfgPath := defaultConfigPath
	if abs, err := filepath.Abs(defaultConfigPath); err == nil {
		cfgPath = abs
	}

	root.AddCommand(newInitCmd(cfgPath))
	root.AddCommand(newProjectCmd(cfgPath))
	root.AddCommand(newFeatureCmd(cfgPath))
	root.AddCommand(newTaskCmd(cfgPath))
	root.AddCommand(newRunCmd(cfgPath))

	return root
}
```

- [ ] **Step 5: Run tests and confirm they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd_run.go cmd_run_test.go main.go
git commit -m "feat: add klanky run stub command with arg validation"
```

---

## Task 2: Snapshot — fetch tasks + project field values + PRs in one shot

**Files:**
- Create: `/Users/jp/Source/klanky/snapshot.go`
- Create: `/Users/jp/Source/klanky/snapshot_test.go`

The runner reads everything it needs about a feature in two roundtrips: one GraphQL for sub-issues + Phase + Status + project item IDs, and one `gh pr list --search` for all klanky-pattern PRs on the feature. After this task, you can call `FetchSnapshot(ctx, runner, cfg, featureID)` and get a fully populated `Snapshot` ready for reconcile.

- [ ] **Step 1: Write failing tests for the snapshot fetch**

Create `snapshot_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
)

func mockConfig() *Config {
	return &Config{
		SchemaVersion: 1,
		Repo:          ConfigRepo{Owner: "alice", Name: "proj"},
		Project: ConfigProject{
			URL: "https://github.com/users/alice/projects/1", Number: 1,
			NodeID: "PVT_x", OwnerLogin: "alice", OwnerType: "User",
			Fields: ConfigFields{
				Phase:  ConfigField{ID: "PVTF_p", Name: "Phase"},
				Status: ConfigStatusField{ID: "PVTSSF_s", Name: "Status",
					Options: map[string]string{
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e",
					}},
			},
		},
		FeatureLabel: ConfigLabel{Name: "klanky:feature"},
	}
}

func TestFetchSnapshot_ParsesTasksAndProjectFields(t *testing.T) {
	r := NewFakeRunner()

	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "Auth refactor",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "Add login form", "body": "## Context\n...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [{
					"id": "PVTI_101",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}
					]}
				}]}
			},
			{
				"number": 102, "title": "Add session middleware", "body": "## Context\n...",
				"state": "CLOSED", "id": "I_102",
				"projectItems": {"nodes": [{
					"id": "PVTI_102",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Done", "optionId": "e"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)

	prResp := `[{"headRefName":"klanky/feat-100/task-101","number":201,"url":"https://github.com/alice/proj/pull/201","state":"OPEN","closed":false,"merged":false}]`
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(prResp), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}

	if snap.Feature.Number != 100 {
		t.Errorf("Feature.Number = %d, want 100", snap.Feature.Number)
	}
	if len(snap.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(snap.Tasks))
	}

	t101 := findTask(t, snap.Tasks, 101)
	if t101.State != "OPEN" {
		t.Errorf("task 101 State = %q, want OPEN", t101.State)
	}
	if t101.ItemID != "PVTI_101" {
		t.Errorf("task 101 ItemID = %q, want PVTI_101", t101.ItemID)
	}
	if t101.Phase == nil || *t101.Phase != 1 {
		t.Errorf("task 101 Phase = %v, want 1", t101.Phase)
	}
	if t101.Status != "Todo" {
		t.Errorf("task 101 Status = %q, want Todo", t101.Status)
	}

	t102 := findTask(t, snap.Tasks, 102)
	if t102.State != "CLOSED" {
		t.Errorf("task 102 State = %q, want CLOSED", t102.State)
	}
	if t102.Status != "Done" {
		t.Errorf("task 102 Status = %q, want Done", t102.Status)
	}

	pr, ok := snap.PRsByBranch["klanky/feat-100/task-101"]
	if !ok {
		t.Fatal("expected PR for klanky/feat-100/task-101")
	}
	if pr.Number != 201 || pr.State != "OPEN" {
		t.Errorf("PR = %+v, want number=201 state=OPEN", pr)
	}
}

func TestFetchSnapshot_HandlesMissingPhaseAndStatus(t *testing.T) {
	r := NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "T", "body": "...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [{
					"id": "PVTI_101",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": []}
				}]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.Tasks[0].Phase != nil {
		t.Errorf("expected Phase nil, got %v", snap.Tasks[0].Phase)
	}
	if snap.Tasks[0].Status != "" {
		t.Errorf("expected Status empty, got %q", snap.Tasks[0].Status)
	}
}

func TestFetchSnapshot_FiltersForeignProjectItems(t *testing.T) {
	r := NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "T", "body": "...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [
					{"id": "PVTI_other", "project": {"id": "PVT_other"},
					 "fieldValues": {"nodes": [{"field": {"name": "Status"}, "name": "Done", "optionId": "z"}]}},
					{"id": "PVTI_ours", "project": {"id": "PVT_x"},
					 "fieldValues": {"nodes": [{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}]}}
				]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.Tasks[0].ItemID != "PVTI_ours" {
		t.Errorf("ItemID = %q, want PVTI_ours (foreign project should be filtered)", snap.Tasks[0].ItemID)
	}
	if snap.Tasks[0].Status != "Todo" {
		t.Errorf("Status = %q, want Todo (foreign project's Done should not leak)", snap.Tasks[0].Status)
	}
}

func TestFetchSnapshot_RejectsTooManySubIssues(t *testing.T) {
	r := NewFakeRunner()

	var sb strings.Builder
	sb.WriteString(`{"data":{"repository":{"issue":{"number":100,"title":"F","subIssues":{"nodes":[`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"number":1,"title":"t","body":"","state":"OPEN","id":"x","projectItems":{"nodes":[]}}`)
	}
	sb.WriteString(`]}}}}}`)

	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(sb.String()), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	_, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("100 sub-issues should be allowed; got error: %v", err)
	}
}

// Helper used in tests above.
func findTask(t *testing.T, tasks []TaskInfo, number int) TaskInfo {
	t.Helper()
	for _, task := range tasks {
		if task.Number == number {
			return task
		}
	}
	t.Fatalf("no task with number %d", number)
	return TaskInfo{}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `FetchSnapshot`, `Snapshot`, `TaskInfo`, `snapshotQuery` undefined.

- [ ] **Step 3: Implement `snapshot.go`**

Create `snapshot.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Snapshot is the read-only view of a feature's state at the start of a run.
// Status writes during the run mutate GitHub directly; Snapshot is not updated.
type Snapshot struct {
	Feature     FeatureInfo
	Tasks       []TaskInfo
	PRsByBranch map[string]PRInfo
}

type FeatureInfo struct {
	Number int
	Title  string
}

type TaskInfo struct {
	Number int
	Title  string
	Body   string
	State  string // "OPEN" or "CLOSED"
	NodeID string
	ItemID string // project item ID (filtered to our project node)
	Phase  *int   // nil when not set on the project item
	Status string // "Todo" / "In Progress" / "In Review" / "Needs Attention" / "Done" / "" if unset
}

type PRInfo struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"`  // "OPEN" / "CLOSED" / "MERGED"
	Closed      bool   `json:"closed"`
	Merged      bool   `json:"merged"`
	HeadRefName string `json:"headRefName"`
}

const snapshotQuery = `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      number
      title
      subIssues(first: 100) {
        nodes {
          number
          title
          body
          state
          id
          projectItems(first: 5) {
            nodes {
              id
              project { id }
              fieldValues(first: 20) {
                nodes {
                  ... on ProjectV2ItemFieldNumberValue {
                    field { ... on ProjectV2Field { name } }
                    number
                  }
                  ... on ProjectV2ItemFieldSingleSelectValue {
                    field { ... on ProjectV2SingleSelectField { name } }
                    name
                    optionId
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// FetchSnapshot makes one GraphQL call for tasks+fields and one PR list call,
// then returns a populated Snapshot. The PR list filters to klanky-pattern
// branches under this feature.
func FetchSnapshot(ctx context.Context, r Runner, cfg *Config, featureID int) (*Snapshot, error) {
	var gqlResp struct {
		Repository struct {
			Issue struct {
				Number    int    `json:"number"`
				Title     string `json:"title"`
				SubIssues struct {
					Nodes []struct {
						Number       int    `json:"number"`
						Title        string `json:"title"`
						Body         string `json:"body"`
						State        string `json:"state"`
						ID           string `json:"id"`
						ProjectItems struct {
							Nodes []struct {
								ID      string `json:"id"`
								Project struct {
									ID string `json:"id"`
								} `json:"project"`
								FieldValues struct {
									Nodes []struct {
										Field struct {
											Name string `json:"name"`
										} `json:"field"`
										Number   *float64 `json:"number,omitempty"`
										Name     string   `json:"name,omitempty"`
										OptionID string   `json:"optionId,omitempty"`
									} `json:"nodes"`
								} `json:"fieldValues"`
							} `json:"nodes"`
						} `json:"projectItems"`
					} `json:"nodes"`
				} `json:"subIssues"`
			} `json:"issue"`
		} `json:"repository"`
	}

	if err := RunGraphQL(ctx, r, snapshotQuery, map[string]any{
		"owner":  cfg.Repo.Owner,
		"repo":   cfg.Repo.Name,
		"number": featureID,
	}, &gqlResp); err != nil {
		return nil, fmt.Errorf("fetch feature snapshot: %w", err)
	}

	feature := FeatureInfo{
		Number: gqlResp.Repository.Issue.Number,
		Title:  gqlResp.Repository.Issue.Title,
	}
	if feature.Number == 0 {
		return nil, fmt.Errorf("feature #%d not found in %s/%s", featureID, cfg.Repo.Owner, cfg.Repo.Name)
	}

	tasks := make([]TaskInfo, 0, len(gqlResp.Repository.Issue.SubIssues.Nodes))
	for _, n := range gqlResp.Repository.Issue.SubIssues.Nodes {
		ti := TaskInfo{
			Number: n.Number,
			Title:  n.Title,
			Body:   n.Body,
			State:  n.State,
			NodeID: n.ID,
		}
		// Find the project item belonging to OUR project (filter by node ID).
		for _, pi := range n.ProjectItems.Nodes {
			if pi.Project.ID != cfg.Project.NodeID {
				continue
			}
			ti.ItemID = pi.ID
			for _, fv := range pi.FieldValues.Nodes {
				switch fv.Field.Name {
				case FieldNamePhase:
					if fv.Number != nil {
						p := int(*fv.Number)
						ti.Phase = &p
					}
				case FieldNameStatus:
					ti.Status = fv.Name
				}
			}
			break
		}
		tasks = append(tasks, ti)
	}

	prSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name
	prSearch := fmt.Sprintf("head:klanky/feat-%d/", featureID)
	prOut, err := r.Run(ctx, "gh", "pr", "list",
		"--repo", prSlug,
		"--state", "all",
		"--search", prSearch,
		"--json", "headRefName,number,url,state,closed,merged",
		"--limit", "200",
	)
	if err != nil {
		return nil, fmt.Errorf("fetch PR list: %w", err)
	}
	var prs []PRInfo
	if err := json.Unmarshal(prOut, &prs); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	prsByBranch := make(map[string]PRInfo, len(prs))
	for _, pr := range prs {
		prsByBranch[pr.HeadRefName] = pr
	}

	return &Snapshot{
		Feature:     feature,
		Tasks:       tasks,
		PRsByBranch: prsByBranch,
	}, nil
}

// BranchForTask returns the branch name for a (feature, task) pair.
func BranchForTask(featureID, taskNumber int) string {
	return fmt.Sprintf("klanky/feat-%d/task-%d", featureID, taskNumber)
}

// itoa is a tiny helper so callers don't need strconv just to format ints in a single place.
func itoa(n int) string { return strconv.Itoa(n) }
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add snapshot.go snapshot_test.go
git commit -m "feat: snapshot fetch — one GraphQL + one PR list per feature"
```

---

## Task 3: Lock file with PID + takeover semantics

**Files:**
- Create: `/Users/jp/Source/klanky/lock.go`
- Create: `/Users/jp/Source/klanky/lock_test.go`

The lock file is `<repo>/.klanky/runner-<feature-id>.lock` with JSON `{"pid": N, "started_at": "..."}`. Atomic create via `O_CREATE|O_EXCL`. On EEXIST: alive PID → refuse; dead PID → silent takeover with overwrite.

- [ ] **Step 1: Write failing tests**

Create `lock_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAcquireLock_FreshCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		PID       int    `json:"pid"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatal(err)
	}
	if content.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", content.PID, os.Getpid())
	}
	if content.StartedAt == "" {
		t.Error("StartedAt empty")
	}
}

func TestAcquireLock_RefusesWhenAlive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	// Write a lock file claiming the current process owns it (definitely alive).
	content := []byte(`{"pid": ` + itoa(os.Getpid()) + `, "started_at": "2026-04-26T10:00:00Z"}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := AcquireLock(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "in progress") {
		t.Errorf("error should mention 'in progress': %v", err)
	}
}

func TestAcquireLock_TakesOverDeadPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	// PID 1 is init/launchd — definitely not us, but definitely alive on a real
	// system. We need a dead PID. Find one by scanning a high range; or trust
	// that PID 999999 is unlikely to exist.
	dead := findDeadPID(t)
	content := []byte(`{"pid": ` + itoa(dead) + `, "started_at": "2026-04-26T10:00:00Z"}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("expected silent takeover, got error: %v", err)
	}
	defer lock.Release()

	// Verify the lock was overwritten with our PID.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), itoa(os.Getpid())) {
		t.Errorf("lock file should contain our PID; got: %s", data)
	}
}

func TestAcquireLock_TakesOverCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("expected takeover on corrupt file, got: %v", err)
	}
	defer lock.Release()
}

func TestRelease_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("lock file should be removed; got err=%v", err)
	}
}

// findDeadPID returns a PID that is not currently alive on the system.
// Scans backward from a high number looking for one where kill(pid, 0) fails.
func findDeadPID(t *testing.T) int {
	t.Helper()
	for pid := 99999; pid > 1000; pid-- {
		if err := syscall.Kill(pid, 0); err != nil {
			return pid
		}
	}
	t.Fatal("could not find a dead PID for test")
	return 0
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `AcquireLock` undefined.

- [ ] **Step 3: Implement `lock.go`**

Create `lock.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

// Lock represents a held klanky runner lock. Call Release on graceful shutdown
// (typically via defer at the top of a run).
type Lock struct {
	path string
}

type lockContent struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

// AcquireLock attempts to create the lock file at path. On success it returns
// a Lock; the caller must call Release. If the file already exists:
//   - alive PID  → returns an error refusing to start
//   - dead PID   → silent takeover (overwrites with our PID), returns Lock
//   - corrupt    → silent takeover (treat as dead), returns Lock
func AcquireLock(path string) (*Lock, error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			content := lockContent{
				PID:       os.Getpid(),
				StartedAt: time.Now().UTC().Format(time.RFC3339),
			}
			data, _ := json.Marshal(content)
			if _, werr := f.Write(data); werr != nil {
				f.Close()
				os.Remove(path)
				return nil, fmt.Errorf("write lock %s: %w", path, werr)
			}
			f.Close()
			return &Lock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create lock %s: %w", path, err)
		}

		// File exists. Inspect it.
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read existing lock %s: %w", path, rerr)
		}
		var existing lockContent
		if jerr := json.Unmarshal(data, &existing); jerr != nil {
			// Corrupt — take over.
			if rmerr := os.Remove(path); rmerr != nil {
				return nil, fmt.Errorf("remove corrupt lock %s: %w", path, rmerr)
			}
			fmt.Fprintf(os.Stderr, "klanky: stale lock at %s (corrupt); recovering.\n", path)
			continue
		}

		if existing.PID > 0 && pidAlive(existing.PID) {
			return nil, fmt.Errorf(
				"another klanky run is in progress for this feature (pid %d, started %s). Exit it first, or wait.",
				existing.PID, existing.StartedAt)
		}

		// Dead PID → take over.
		if rmerr := os.Remove(path); rmerr != nil {
			return nil, fmt.Errorf("remove stale lock %s: %w", path, rmerr)
		}
		fmt.Fprintf(os.Stderr, "klanky: stale lock from pid %d (started %s); recovering.\n", existing.PID, existing.StartedAt)
	}
	return nil, fmt.Errorf("could not acquire lock at %s after retry", path)
}

// Release deletes the lock file. Safe to call multiple times.
func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	l.path = ""
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// pidAlive returns true if the process with the given PID is currently alive.
// Implemented via signal-0, which delivers no signal but performs error checking.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but we don't own it — still alive.
	if err == syscall.EPERM {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests and confirm pass**

Run: `go test ./...`
Expected: PASS. (If the dead-PID test is flaky due to a coincidentally-alive high PID, the helper retries down the range.)

- [ ] **Step 5: Commit**

```bash
git add lock.go lock_test.go
git commit -m "feat: per-feature lock file with PID liveness + silent takeover"
```

---

## Task 4: Reconcile — pure function over Snapshot

**Files:**
- Create: `/Users/jp/Source/klanky/reconcile.go`
- Create: `/Users/jp/Source/klanky/reconcile_test.go`

`Reconcile` is a pure function: takes a Snapshot, returns a list of `ReconcileAction` describing what status updates and breadcrumbs to apply. The runner then applies these via gh calls. Pure-function design makes the matrix easy to test row-by-row.

- [ ] **Step 1: Write failing tests covering the matrix**

Create `reconcile_test.go`:

```go
package main

import (
	"testing"
)

func ptrInt(n int) *int { return &n }

func TestReconcile_Row1_ClosedIssueGetsDone(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "CLOSED", Status: "In Review", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Done", "")
}

func TestReconcile_Row1_ClosedIssueAlreadyDone_NoAction(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row2_OpenTodoNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Todo", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row3_InProgressNoLivePIDGoesNeedsAttention(t *testing.T) {
	// Reconcile cannot detect a live PID directly; it infers crash from
	// "in-progress + no PR + a worktree exists" OR "in-progress + no PR" if we
	// trust the lock-file takeover already cleared the prior process.
	// Per locked design, lock takeover means any surviving in-progress is
	// definitively a crash — so we don't need to check worktree existence.
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "previous run crashed")
}

func TestReconcile_Row4_InProgressWithOpenPRGoesInReview(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "OPEN", URL: "u"},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "In Review", "")
}

func TestReconcile_Row6_InReviewWithOpenPRNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "OPEN"},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row7_InReviewWithClosedNotMergedGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "CLOSED", Closed: true, Merged: false},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "PR")
}

func TestReconcile_Row8_InReviewWithNoPRGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "")
}

func TestReconcile_Row9_NeedsAttentionNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Needs Attention", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row10_DoneOnOpenIssueIsInvariantViolation(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Done", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "invariant")
}

func TestReconcile_Row11_OpenWithMissingPhaseGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Todo", Phase: nil},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "Phase")
}

// mustHaveAction asserts there's exactly one action for the given task with
// the given status, and that the breadcrumb (if non-empty) contains the given
// substring.
func mustHaveAction(t *testing.T, actions []ReconcileAction, taskNumber int, wantStatus, breadcrumbContains string) {
	t.Helper()
	for _, a := range actions {
		if a.TaskNumber != taskNumber {
			continue
		}
		if a.NewStatus != wantStatus {
			t.Errorf("task %d: NewStatus = %q, want %q", taskNumber, a.NewStatus, wantStatus)
		}
		if breadcrumbContains != "" && !contains(a.Breadcrumb, breadcrumbContains) {
			t.Errorf("task %d: Breadcrumb = %q, want to contain %q", taskNumber, a.Breadcrumb, breadcrumbContains)
		}
		return
	}
	t.Errorf("no action found for task %d in %+v", taskNumber, actions)
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `Reconcile`, `ReconcileAction` undefined.

- [ ] **Step 3: Implement `reconcile.go`**

Create `reconcile.go`:

```go
package main

// ReconcileAction describes a single status mutation (and optional breadcrumb)
// the runner should apply during the reconcile phase. NewStatus is the literal
// Status option name (e.g. "Done", "Needs Attention").
type ReconcileAction struct {
	TaskNumber int
	ItemID     string
	NewStatus  string
	Breadcrumb string // freeform; empty means no breadcrumb to post
}

// Reconcile inspects the snapshot and returns the list of state updates needed
// to bring the runner-maintained Status mirror in sync with underlying truth
// (issue state + PR state). Implements the 11-row matrix from
// project_runner_design.md.
func Reconcile(snap *Snapshot, featureID int) []ReconcileAction {
	var actions []ReconcileAction
	for _, task := range snap.Tasks {
		// Row 11: missing Phase value — flag and skip further reconcile for this task.
		if task.Phase == nil {
			if task.Status != "Needs Attention" {
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "Task has no Phase value; set one in the project to re-arm.",
				})
			}
			continue
		}

		// Row 1: closed issue → Done.
		if task.State == "CLOSED" {
			if task.Status != "Done" {
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus: "Done",
				})
			}
			continue
		}

		branch := BranchForTask(featureID, task.Number)
		pr, hasPR := snap.PRsByBranch[branch]

		switch task.Status {
		case "", "Todo":
			// Row 2: nothing to reconcile.
		case "In Progress":
			if hasPR && pr.State == "OPEN" {
				// Row 4: agent landed PR but status flip didn't take.
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus: "In Review",
				})
			} else {
				// Row 3: crashed mid-task (lock takeover already cleared the process,
				// so any surviving In Progress is by definition stale).
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "previous run crashed mid-task before opening a PR; worktree preserved if it existed.",
				})
			}
		case "In Review":
			if !hasPR {
				// Row 8: status was set but PR is gone.
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "Status was In Review but no PR exists on the expected branch; was the PR deleted or the branch force-pushed?",
				})
			} else if pr.State != "OPEN" {
				// Row 7: PR closed without merge.
				actions = append(actions, ReconcileAction{
					TaskNumber: task.Number, ItemID: task.ItemID,
					NewStatus:  "Needs Attention",
					Breadcrumb: "PR #" + itoa(pr.Number) + " was closed without merging; review feedback may be in the PR thread.",
				})
			}
			// Row 6: open PR, leave alone.
		case "Needs Attention":
			// Row 9: eligible for work via the work queue; reconcile no-op.
		case "Done":
			// Row 10: invariant violation (Done implies closed).
			actions = append(actions, ReconcileAction{
				TaskNumber: task.Number, ItemID: task.ItemID,
				NewStatus:  "Needs Attention",
				Breadcrumb: "invariant violation: Status was Done but issue is open; was the issue manually re-opened?",
			})
		}
	}
	return actions
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add reconcile.go reconcile_test.go
git commit -m "feat: pure-function reconcile implementing the 11-row matrix"
```

---

## Task 5: Work-queue selection

**Files:**
- Create: `/Users/jp/Source/klanky/workqueue.go`
- Create: `/Users/jp/Source/klanky/workqueue_test.go`

After reconcile, the runner needs to pick the current phase and the eligible task list. This is also a pure function — easy to test against synthetic snapshots.

- [ ] **Step 1: Write failing tests**

Create `workqueue_test.go`:

```go
package main

import (
	"testing"
)

func TestSelectWork_PicksLowestOpenPhase(t *testing.T) {
	snap := &Snapshot{
		Tasks: []TaskInfo{
			{Number: 1, State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "Todo", Phase: ptrInt(2)},
			{Number: 3, State: "OPEN", Status: "Todo", Phase: ptrInt(3)},
		},
	}
	got := SelectWork(snap)
	if got.AllDone {
		t.Error("AllDone = true, want false")
	}
	if got.CurrentPhase != 2 {
		t.Errorf("CurrentPhase = %d, want 2", got.CurrentPhase)
	}
	if len(got.Eligible) != 1 || got.Eligible[0].Number != 2 {
		t.Errorf("Eligible = %+v, want one task #2", got.Eligible)
	}
}

func TestSelectWork_AllClosed_AllDone(t *testing.T) {
	snap := &Snapshot{
		Tasks: []TaskInfo{
			{Number: 1, State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
			{Number: 2, State: "CLOSED", Status: "Done", Phase: ptrInt(2)},
		},
	}
	got := SelectWork(snap)
	if !got.AllDone {
		t.Error("AllDone = false, want true")
	}
}

func TestSelectWork_OnlyInReviewInPhase(t *testing.T) {
	snap := &Snapshot{
		Tasks: []TaskInfo{
			{Number: 1, State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-7/task-1": {Number: 11, URL: "u1", State: "OPEN"},
			"klanky/feat-7/task-2": {Number: 12, URL: "u2", State: "OPEN"},
		},
	}
	got := SelectWork(snap)
	if got.AllDone {
		t.Error("AllDone = true, want false")
	}
	if got.CurrentPhase != 1 {
		t.Errorf("CurrentPhase = %d, want 1", got.CurrentPhase)
	}
	if len(got.Eligible) != 0 {
		t.Errorf("Eligible = %+v, want empty", got.Eligible)
	}
	if len(got.AwaitingReview) != 2 {
		t.Errorf("len(AwaitingReview) = %d, want 2", len(got.AwaitingReview))
	}
}

func TestSelectWork_IncludesNeedsAttentionInEligible(t *testing.T) {
	snap := &Snapshot{
		Tasks: []TaskInfo{
			{Number: 1, State: "OPEN", Status: "Todo", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "Needs Attention", Phase: ptrInt(1)},
		},
	}
	got := SelectWork(snap)
	if len(got.Eligible) != 2 {
		t.Errorf("Eligible len = %d, want 2 (todo + needs-attention)", len(got.Eligible))
	}
}

func TestSelectWork_FlagsSurvivingInProgress(t *testing.T) {
	snap := &Snapshot{
		Tasks: []TaskInfo{
			// In Progress that survived reconcile = bug. Caller surfaces as scenario C.
			{Number: 1, State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
	}
	got := SelectWork(snap)
	if len(got.SurvivingInProgress) != 1 {
		t.Errorf("len(SurvivingInProgress) = %d, want 1", len(got.SurvivingInProgress))
	}
}

func TestSelectWork_NoTasks_AllDone(t *testing.T) {
	snap := &Snapshot{Tasks: []TaskInfo{}}
	got := SelectWork(snap)
	if !got.AllDone {
		t.Error("AllDone should be true when no tasks exist")
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `SelectWork`, `WorkQueueResult` undefined.

- [ ] **Step 3: Implement `workqueue.go`**

Create `workqueue.go`:

```go
package main

// WorkQueueResult is the post-reconcile decision about what to actually run.
type WorkQueueResult struct {
	AllDone             bool       // every task in the feature has issue=closed
	CurrentPhase        int        // lowest phase number with any open issue; 0 when AllDone
	Eligible            []TaskInfo // current-phase tasks the runner will spawn agents for
	AwaitingReview      []TaskInfo // current-phase tasks with Status=In Review (informational, surfaced in messaging)
	SurvivingInProgress []TaskInfo // current-phase tasks with Status=In Progress that somehow survived reconcile (a bug — surfaced as scenario C)
}

// SelectWork picks the current phase and partitions its tasks into the
// queues the runner needs. Caller must have already applied reconcile actions
// (the Snapshot's Status fields reflect the pre-reconcile values).
//
// NOTE: This function operates on the snapshot Status values, not on a
// post-reconcile mutation. The caller is responsible for either passing in a
// snapshot whose Status fields have been updated to reflect the reconcile, or
// applying reconcile actions in their own code path before relying on these
// queues. Most callers will call ApplyReconcile first (Task 11) which mutates
// the snapshot in-memory.
func SelectWork(snap *Snapshot) WorkQueueResult {
	openByPhase := map[int][]TaskInfo{}
	hasOpen := false
	for _, task := range snap.Tasks {
		if task.State != "OPEN" {
			continue
		}
		hasOpen = true
		if task.Phase == nil {
			// Tasks without Phase are flagged by reconcile (Row 11) and set to
			// Needs Attention; they don't participate in phase selection.
			continue
		}
		openByPhase[*task.Phase] = append(openByPhase[*task.Phase], task)
	}

	if !hasOpen {
		return WorkQueueResult{AllDone: true}
	}

	// Lowest phase with any open task that has a Phase value.
	current := -1
	for phase := range openByPhase {
		if current == -1 || phase < current {
			current = phase
		}
	}
	// If every open task lacks a Phase, openByPhase is empty — degenerate case.
	if current == -1 {
		return WorkQueueResult{AllDone: false, CurrentPhase: 0}
	}

	res := WorkQueueResult{CurrentPhase: current}
	for _, task := range openByPhase[current] {
		switch task.Status {
		case "", "Todo", "Needs Attention":
			res.Eligible = append(res.Eligible, task)
		case "In Review":
			res.AwaitingReview = append(res.AwaitingReview, task)
		case "In Progress":
			res.SurvivingInProgress = append(res.SurvivingInProgress, task)
		}
	}
	return res
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add workqueue.go workqueue_test.go
git commit -m "feat: work-queue selection — current phase + eligibility partitioning"
```

---

## Task 6: Worktree management

**Files:**
- Create: `/Users/jp/Source/klanky/worktree.go`
- Create: `/Users/jp/Source/klanky/worktree_test.go`

The runner ensures a clean worktree at `~/.klanky/worktrees/<repo>/feat-F/task-T/` for every task. On retry, wipe the existing path, prune git's worktree registry, then create fresh.

- [ ] **Step 1: Write failing tests**

Create `worktree_test.go`:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCleanWorktree_FreshCreate(t *testing.T) {
	wtRoot := t.TempDir()
	wtPath := filepath.Join(wtRoot, "feat-7", "task-42")
	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/repo", "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", "/repo", "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

	if err := EnsureCleanWorktree(context.Background(), r, "/repo", wtPath, "klanky/feat-7/task-42", "main"); err != nil {
		t.Fatalf("EnsureCleanWorktree: %v", err)
	}
}

func TestEnsureCleanWorktree_WipesExistingPath(t *testing.T) {
	wtRoot := t.TempDir()
	wtPath := filepath.Join(wtRoot, "feat-7", "task-42")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "leftover.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/repo", "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", "/repo", "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

	if err := EnsureCleanWorktree(context.Background(), r, "/repo", wtPath, "klanky/feat-7/task-42", "main"); err != nil {
		t.Fatalf("EnsureCleanWorktree: %v", err)
	}

	// Path should have been removed before git was asked to recreate it.
	if _, err := os.Stat(filepath.Join(wtPath, "leftover.txt")); !os.IsNotExist(err) {
		t.Errorf("leftover.txt should be gone; got err=%v", err)
	}
}

func TestWorktreePath_StableLayout(t *testing.T) {
	got := WorktreePath("/home/u/.klanky/worktrees", "myrepo", 7, 42)
	want := "/home/u/.klanky/worktrees/myrepo/feat-7/task-42"
	if got != want {
		t.Errorf("WorktreePath = %q, want %q", got, want)
	}
}

func TestDefaultWorktreeRoot_UsesHome(t *testing.T) {
	root, err := DefaultWorktreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".klanky", "worktrees")
	if root != want {
		t.Errorf("DefaultWorktreeRoot = %q, want %q", root, want)
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `worktree.go`**

Create `worktree.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultWorktreeRoot returns ~/.klanky/worktrees, the locked-by-design root
// for klanky-managed worktrees.
func DefaultWorktreeRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".klanky", "worktrees"), nil
}

// WorktreePath assembles the per-task worktree path under the given root.
func WorktreePath(root, repoName string, featureID, taskNumber int) string {
	return filepath.Join(root, repoName, fmt.Sprintf("feat-%d", featureID), fmt.Sprintf("task-%d", taskNumber))
}

// EnsureCleanWorktree guarantees a fresh git worktree at wtPath on the given
// branch, branched from base. Wipes any existing path and prunes git's
// worktree registry first so retries always start clean.
//
// repoRoot is the absolute path to the main checkout (where .git lives).
func EnsureCleanWorktree(ctx context.Context, r Runner, repoRoot, wtPath, branch, base string) error {
	if err := os.RemoveAll(wtPath); err != nil {
		return fmt.Errorf("rm worktree path %s: %w", wtPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", wtPath, err)
	}
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "prune"); err != nil {
		return fmt.Errorf("git worktree prune: %w", err)
	}
	if _, err := r.Run(ctx, "git", "-C", repoRoot, "worktree", "add", wtPath, "-b", branch, base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add worktree.go worktree_test.go
git commit -m "feat: worktree management — wipe-and-rebuild for clean retries"
```

---

## Task 7: Envelope template

**Files:**
- Create: `/Users/jp/Source/klanky/envelope.go`
- Create: `/Users/jp/Source/klanky/envelope_test.go`

The locked envelope template (from `project_runner_design.md`), assembled via `fmt.Sprintf`.

- [ ] **Step 1: Write failing tests**

Create `envelope_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestBuildEnvelope_SubstitutesTaskFields(t *testing.T) {
	got := BuildEnvelope(EnvelopeData{
		FeatureID:    7,
		TaskNumber:   42,
		TaskTitle:    "Add login form",
		TaskBody:     "## Context\nWe need login.",
		WorktreePath: "/home/u/.klanky/worktrees/proj/feat-7/task-42",
	})

	wantSubstrs := []string{
		"task #42: Add login form",
		"branch `klanky/feat-7/task-42`",
		"branched from `main`",
		"/home/u/.klanky/worktrees/proj/feat-7/task-42",
		"## Context\nWe need login.",
		"gh pr create --base main",
		"Closes #42",
		"`.github/pull_request_template.md`",
		"`.github/PULL_REQUEST_TEMPLATE/`",
		"CLAUDE.md",
		"comment on issue #42",
		"prior comments from previous attempts",
		"Test-Driven Development",
		"raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md",
		"lint and test",
		"Do not push to any branch other than `klanky/feat-7/task-42`",
		"Do not merge any PR",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing substring %q", want)
		}
	}
}

func TestBuildEnvelope_BodyPlacedVerbatim(t *testing.T) {
	body := "## Context\nFoo\n\n## Acceptance criteria\n- [ ] Bar\n\n## Out of scope\nBaz"
	got := BuildEnvelope(EnvelopeData{
		FeatureID: 1, TaskNumber: 1, TaskTitle: "T",
		TaskBody: body, WorktreePath: "/wt",
	})
	if !strings.Contains(got, body) {
		t.Errorf("body not placed verbatim; envelope:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `envelope.go`**

Create `envelope.go`:

```go
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
		d.TaskBody,                                // # Task body
		d.TaskNumber, d.TaskNumber,                // two Closes #N references
		d.TaskNumber, d.TaskNumber,                // give-up + retry-context
		d.FeatureID, d.TaskNumber,                 // constraints branch reference
	)
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add envelope.go envelope_test.go
git commit -m "feat: envelope template for claude -p invocation"
```

---

## Task 8: Agent execution — Spawner interface, run + verify

**Files:**
- Create: `/Users/jp/Source/klanky/agent.go`
- Create: `/Users/jp/Source/klanky/agent_test.go`

Spawning `claude -p` is the one place the runner needs to do something `Runner` (which captures stdout into a buffer) can't do — long-running subprocess with stdout/stderr streamed to a file. We introduce a `Spawner` interface alongside `Runner`. After spawn, post-exit verification runs gh queries via `Runner` to check branch state and PR existence.

- [ ] **Step 1: Write failing tests**

Create `agent_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FakeSpawner records the subprocess invocations and returns scripted exit codes.
type FakeSpawner struct {
	Calls    []FakeSpawnCall
	exitCode int
	err      error
	stdout   string
	stderr   string
}

type FakeSpawnCall struct {
	Name string
	Args []string
	Cwd  string
	Env  []string
}

func (f *FakeSpawner) Stub(exitCode int, stdout, stderr string, err error) {
	f.exitCode = exitCode
	f.stdout = stdout
	f.stderr = stderr
	f.err = err
}

func (f *FakeSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	f.Calls = append(f.Calls, FakeSpawnCall{Name: name, Args: args, Cwd: opts.Cwd, Env: opts.Env})
	if opts.Stdout != nil && f.stdout != "" {
		opts.Stdout.Write([]byte(f.stdout))
	}
	if opts.Stderr != nil && f.stderr != "" {
		opts.Stderr.Write([]byte(f.stderr))
	}
	return f.exitCode, f.err
}

func TestRunAgent_HappyPath_ReturnsInReview(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "task-42.log")

	sp := &FakeSpawner{}
	sp.Stub(0, "did stuff\n", "", nil)

	r := NewFakeRunner()
	// Branch verification: at least one commit beyond main.
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("2\n"), nil)
	// PR verification.
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://github.com/alice/proj/pull/77","number":77}]`), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID:    7,
		Task:         TaskInfo{Number: 42, Title: "T", Body: "..."},
		WorktreePath: "/wt",
		LogPath:      logPath,
		RepoSlug:     "alice/proj",
		Timeout:      20 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeInReview {
		t.Errorf("Outcome = %v, want OutcomeInReview", res.Outcome)
	}
	if res.PR == nil || res.PR.Number != 77 {
		t.Errorf("PR = %+v, want number=77", res.PR)
	}

	// Spawn invocation should pass the envelope and the right flags.
	if len(sp.Calls) != 1 {
		t.Fatalf("expected 1 spawn call, got %d", len(sp.Calls))
	}
	call := sp.Calls[0]
	if call.Name != "claude" {
		t.Errorf("Name = %q, want claude", call.Name)
	}
	wantArgs := []string{"-p", "", "--permission-mode", "bypassPermissions"}
	if len(call.Args) != len(wantArgs) {
		t.Fatalf("Args length = %d, want %d", len(call.Args), len(wantArgs))
	}
	if call.Args[0] != "-p" {
		t.Errorf("Args[0] = %q, want -p", call.Args[0])
	}
	if !strings.Contains(call.Args[1], "task #42") {
		t.Errorf("Args[1] should contain envelope; got: %s", call.Args[1])
	}
	if call.Args[2] != "--permission-mode" || call.Args[3] != "bypassPermissions" {
		t.Errorf("permission flags wrong: %v", call.Args)
	}

	// GH_REPO must be in env.
	foundEnv := false
	for _, e := range call.Env {
		if e == "GH_REPO=alice/proj" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("env missing GH_REPO=alice/proj: %v", call.Env)
	}

	// Log file should contain spawn stdout.
	logged, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logged), "did stuff") {
		t.Errorf("log file missing spawn stdout; got: %s", logged)
	}
}

func TestRunAgent_NoCommits_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("0\n"), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no commits") {
		t.Errorf("OutcomeReason = %q, want to mention no commits", res.OutcomeReason)
	}
}

func TestRunAgent_NoPR_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("3\n"), nil)
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[]`), nil)

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "no PR") {
		t.Errorf("OutcomeReason = %q, want to mention no PR", res.OutcomeReason)
	}
}

func TestRunAgent_TimeoutKilled_ReturnsNeedsAttention(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(-1, "", "", context.DeadlineExceeded)

	r := NewFakeRunner()

	res, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunAgent should not propagate timeout as error: %v", err)
	}
	if res.Outcome != OutcomeNeedsAttention {
		t.Errorf("Outcome = %v, want OutcomeNeedsAttention", res.Outcome)
	}
	if !strings.Contains(res.OutcomeReason, "timeout") {
		t.Errorf("OutcomeReason = %q, want to mention timeout", res.OutcomeReason)
	}
}

func TestRunAgent_SpawnError_PropagatesAsError(t *testing.T) {
	dir := t.TempDir()
	sp := &FakeSpawner{}
	sp.Stub(-1, "", "", errors.New("exec: \"claude\": executable file not found in $PATH"))

	res, err := RunAgent(context.Background(), NewFakeRunner(), sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: filepath.Join(dir, "log"), RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err == nil {
		t.Fatalf("expected error from spawn failure, got result %+v", res)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention claude: %v", err)
	}
}

// Smoke check: log buffer is wired before spawn.
func TestRunAgent_LogFile_CreatedEvenOnEarlyExit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "task-42.log")
	sp := &FakeSpawner{}
	sp.Stub(0, "", "", nil)

	r := NewFakeRunner()
	r.Stub([]string{"git", "-C", "/wt", "rev-list", "--count", "main..HEAD"}, []byte("0\n"), nil)

	_, err := RunAgent(context.Background(), r, sp, AgentJob{
		FeatureID: 7, Task: TaskInfo{Number: 42}, WorktreePath: "/wt",
		LogPath: logPath, RepoSlug: "alice/proj",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

// Sanity check on bytes.Buffer assertion shape — ensures stdout writer wiring works.
var _ = bytes.NewBuffer
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `Spawner`, `RunAgent`, etc. undefined.

- [ ] **Step 3: Implement `agent.go`**

Create `agent.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Outcome enumerates the final state of an agent run for one task.
type Outcome int

const (
	OutcomeUnknown Outcome = iota
	OutcomeInReview
	OutcomeNeedsAttention
)

func (o Outcome) String() string {
	switch o {
	case OutcomeInReview:
		return "in-review"
	case OutcomeNeedsAttention:
		return "needs-attention"
	default:
		return "unknown"
	}
}

// AgentJob is the per-task input to RunAgent.
type AgentJob struct {
	FeatureID    int
	Task         TaskInfo
	WorktreePath string
	LogPath      string
	RepoSlug     string // owner/name
	Timeout      time.Duration
}

// TaskResult is the per-task output of RunAgent, consumed by the runner for
// status writes, summary rendering, and breadcrumb composition.
type TaskResult struct {
	TaskNumber    int
	Outcome       Outcome
	OutcomeReason string // freeform sentence describing why (used in breadcrumb)
	PR            *PRInfo
	StartedAt     time.Time
	Duration      time.Duration
}

// SpawnOpts is the per-spawn configuration for a Spawner.
type SpawnOpts struct {
	Cwd    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Spawner abstracts subprocess execution for long-running agents.
// Implementations are expected to honor ctx cancellation and return
// (-1, ctx.Err()) on timeout, with the process group SIGKILLed.
type Spawner interface {
	Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (exitCode int, err error)
}

// RealSpawner uses exec.CommandContext and sets the process group so the
// runner can SIGKILL the whole tree on timeout.
type RealSpawner struct{}

func (RealSpawner) Spawn(ctx context.Context, name string, args []string, opts SpawnOpts) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the whole process group rather than just the leader.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return os.ErrProcessDone
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err := cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// RunAgent spawns one agent for one task and verifies its result.
// Returns a non-error TaskResult for both success (in-review) and failure
// (needs-attention) outcomes — only setup errors (e.g. claude binary missing,
// log file un-creatable) are returned as err.
func RunAgent(ctx context.Context, r Runner, sp Spawner, job AgentJob) (*TaskResult, error) {
	if err := os.MkdirAll(filepath.Dir(job.LogPath), 0755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile, err := os.Create(job.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create log %s: %w", job.LogPath, err)
	}
	defer logFile.Close()

	envelope := BuildEnvelope(EnvelopeData{
		FeatureID:    job.FeatureID,
		TaskNumber:   job.Task.Number,
		TaskTitle:    job.Task.Title,
		TaskBody:     job.Task.Body,
		WorktreePath: job.WorktreePath,
	})

	args := []string{"-p", envelope, "--permission-mode", "bypassPermissions"}
	env := append(os.Environ(), "GH_REPO="+job.RepoSlug)

	res := &TaskResult{TaskNumber: job.Task.Number, StartedAt: time.Now()}

	spawnCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	exitCode, spawnErr := sp.Spawn(spawnCtx, "claude", args, SpawnOpts{
		Cwd: job.WorktreePath, Env: env,
		Stdout: logFile, Stderr: logFile,
	})
	res.Duration = time.Since(res.StartedAt)

	// A spawn-system error (binary missing, exec failure) is fatal for this task
	// and should be returned to the runner so it's surfaced rather than silently
	// becoming needs-attention.
	if spawnErr != nil && !errors.Is(spawnErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("spawn claude: %w", spawnErr)
	}

	timedOut := errors.Is(spawnErr, context.DeadlineExceeded) || spawnCtx.Err() == context.DeadlineExceeded
	if timedOut {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent timeout after %s", job.Timeout)
		return res, nil
	}

	// Verify branch has commits beyond main.
	commitOut, gitErr := r.Run(ctx, "git", "-C", job.WorktreePath, "rev-list", "--count", "main..HEAD")
	if gitErr != nil {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("could not verify branch state: %v", gitErr)
		return res, nil
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(commitOut)))
	if count < 1 {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent exited with code %d but the branch has no commits beyond main", exitCode)
		return res, nil
	}

	// Verify open PR exists for the branch.
	branch := BranchForTask(job.FeatureID, job.Task.Number)
	prOut, prErr := r.Run(ctx, "gh", "pr", "list",
		"--repo", job.RepoSlug,
		"--head", branch, "--state", "open",
		"--json", "url,number")
	if prErr != nil {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("could not check for PR: %v", prErr)
		return res, nil
	}
	var prs []struct {
		URL    string `json:"url"`
		Number int    `json:"number"`
	}
	if err := json.Unmarshal(prOut, &prs); err != nil || len(prs) == 0 {
		res.Outcome = OutcomeNeedsAttention
		res.OutcomeReason = fmt.Sprintf("agent exited with code %d but no open PR was found on %s", exitCode, branch)
		return res, nil
	}

	res.Outcome = OutcomeInReview
	res.PR = &PRInfo{Number: prs[0].Number, URL: prs[0].URL, State: "OPEN", HeadRefName: branch}
	return res, nil
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent.go agent_test.go
git commit -m "feat: agent execution — spawn claude -p, verify branch + PR"
```

---

## Task 9: Breadcrumb composition + posting

**Files:**
- Create: `/Users/jp/Source/klanky/breadcrumb.go`
- Create: `/Users/jp/Source/klanky/breadcrumb_test.go`

Breadcrumb posting is split into three pure-ish functions: count prior attempts (gh issue view comments), build the markdown body (pure), post via `gh issue comment`.

- [ ] **Step 1: Write failing tests**

Create `breadcrumb_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildBreadcrumb_ContainsAllRequiredSections(t *testing.T) {
	got := BuildBreadcrumb(BreadcrumbData{
		Attempt:       3,
		StartedAt:     time.Date(2026, 4, 26, 9, 32, 17, 0, time.UTC),
		Duration:      15*time.Minute + 54*time.Second,
		Outcome:       "agent exited cleanly but no PR was opened on klanky/feat-7/task-42",
		WorktreePath:  "/home/u/.klanky/worktrees/proj/feat-7/task-42",
		LogPath:       ".klanky/logs/task-42.log",
		LastLogLines:  []string{"line A", "line B", "line C"},
	})

	wantSubstrs := []string{
		"<!-- klanky-attempt -->",
		"Klanky attempt #3 — needs-attention",
		"2026-04-26",
		"15m54s",
		"agent exited cleanly",
		"/home/u/.klanky/worktrees/proj/feat-7/task-42",
		".klanky/logs/task-42.log",
		"line A",
		"line C",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(got, want) {
			t.Errorf("breadcrumb missing %q\n---\n%s", want, got)
		}
	}
}

func TestCountPriorAttempts_ZeroComments(t *testing.T) {
	r := NewFakeRunner()
	r.Stub([]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "comments"},
		[]byte(`{"comments":[]}`), nil)

	n, err := CountPriorAttempts(context.Background(), r, "alice/proj", 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestCountPriorAttempts_OnlyKlankySentinelCommentsCount(t *testing.T) {
	r := NewFakeRunner()
	r.Stub([]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "comments"},
		[]byte(`{"comments":[
			{"body":"<!-- klanky-attempt -->\n**Klanky attempt #1...**"},
			{"body":"some user comment"},
			{"body":"<!-- klanky-attempt -->\n**Klanky attempt #2...**"}
		]}`), nil)

	n, err := CountPriorAttempts(context.Background(), r, "alice/proj", 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

func TestPostBreadcrumb_CallsGhIssueComment(t *testing.T) {
	r := NewFakeRunner()
	r.Stub([]string{"gh", "issue", "comment", "42", "--repo", "alice/proj", "--body", "hello"}, nil, nil)

	if err := PostBreadcrumb(context.Background(), r, "alice/proj", 42, "hello"); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `breadcrumb.go`**

Create `breadcrumb.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const klankyAttemptSentinel = "<!-- klanky-attempt -->"

// BreadcrumbData is the substitution input for BuildBreadcrumb.
type BreadcrumbData struct {
	Attempt      int
	StartedAt    time.Time
	Duration     time.Duration
	Outcome      string
	WorktreePath string
	LogPath      string
	LastLogLines []string
}

// BuildBreadcrumb returns the markdown body of a needs-attention comment.
// Format is locked in project_runner_design.md and uses the
// `<!-- klanky-attempt -->` sentinel as the count anchor.
func BuildBreadcrumb(d BreadcrumbData) string {
	var b strings.Builder
	fmt.Fprintln(&b, klankyAttemptSentinel)
	fmt.Fprintf(&b, "**Klanky attempt #%d — needs-attention**\n\n", d.Attempt)
	fmt.Fprintf(&b, "- Started: %s\n", d.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Duration: %s\n", d.Duration.Round(time.Second).String())
	fmt.Fprintf(&b, "- Outcome: %s\n", d.Outcome)
	fmt.Fprintf(&b, "- Worktree: `%s` (preserved)\n", d.WorktreePath)
	fmt.Fprintf(&b, "- Log: `%s`\n\n", d.LogPath)
	fmt.Fprintln(&b, "**Last 20 log lines:**")
	fmt.Fprintln(&b, "```")
	for _, line := range d.LastLogLines {
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b, "```")
	return b.String()
}

// CountPriorAttempts returns the number of comments on the issue whose body
// starts with the klanky-attempt sentinel. The next attempt's number is
// returned-value + 1.
func CountPriorAttempts(ctx context.Context, r Runner, repoSlug string, taskNumber int) (int, error) {
	out, err := r.Run(ctx, "gh", "issue", "view", itoa(taskNumber),
		"--repo", repoSlug, "--json", "comments")
	if err != nil {
		return 0, fmt.Errorf("gh issue view: %w", err)
	}
	var resp struct {
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, fmt.Errorf("parse comments: %w", err)
	}
	count := 0
	for _, c := range resp.Comments {
		if strings.HasPrefix(strings.TrimSpace(c.Body), klankyAttemptSentinel) {
			count++
		}
	}
	return count, nil
}

// PostBreadcrumb posts a comment with the given body on the task issue.
func PostBreadcrumb(ctx context.Context, r Runner, repoSlug string, taskNumber int, body string) error {
	if _, err := r.Run(ctx, "gh", "issue", "comment", itoa(taskNumber),
		"--repo", repoSlug, "--body", body); err != nil {
		return fmt.Errorf("gh issue comment: %w", err)
	}
	return nil
}

// TailLines returns the last n lines of a string. If the string has fewer
// than n lines, returns all of them. Trailing empty lines are dropped.
func TailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add breadcrumb.go breadcrumb_test.go
git commit -m "feat: breadcrumb format + sentinel-based attempt counting"
```

---

## Task 10: Status writer with retry

**Files:**
- Create: `/Users/jp/Source/klanky/statuswrite.go`
- Create: `/Users/jp/Source/klanky/statuswrite_test.go`

A small focused helper that writes the Status field for a project item, with 3-retry exponential backoff. Best-effort: returns nil on success, returns an error after final retry; caller logs and continues.

- [ ] **Step 1: Write failing tests**

Create `statuswrite_test.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// retryFakeRunner is a custom fake that fails the first N calls then succeeds.
type retryFakeRunner struct {
	failuresLeft int
	calls        int
}

func (r *retryFakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	r.calls++
	if r.failuresLeft > 0 {
		r.failuresLeft--
		return nil, errors.New("transient error")
	}
	return nil, nil
}

func TestWriteStatus_SuccessOnFirstTry(t *testing.T) {
	r := &retryFakeRunner{failuresLeft: 0}
	cfg := mockConfig()

	err := WriteStatus(context.Background(), r, cfg, "ITEM", "Todo", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1", r.calls)
	}
}

func TestWriteStatus_RetriesAndSucceeds(t *testing.T) {
	r := &retryFakeRunner{failuresLeft: 2}
	cfg := mockConfig()

	err := WriteStatus(context.Background(), r, cfg, "ITEM", "Todo", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if r.calls != 3 {
		t.Errorf("calls = %d, want 3", r.calls)
	}
}

func TestWriteStatus_GivesUpAfterThreeFailures(t *testing.T) {
	r := &retryFakeRunner{failuresLeft: 99}
	cfg := mockConfig()

	err := WriteStatus(context.Background(), r, cfg, "ITEM", "Todo", time.Millisecond)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if r.calls != 3 {
		t.Errorf("calls = %d, want 3", r.calls)
	}
}

func TestWriteStatus_RejectsUnknownStatus(t *testing.T) {
	r := &retryFakeRunner{}
	cfg := mockConfig()

	err := WriteStatus(context.Background(), r, cfg, "ITEM", "Bogus", 0)
	if err == nil {
		t.Fatal("expected error for unknown status")
	}
	if r.calls != 0 {
		t.Errorf("calls = %d, want 0 (should fail before any gh call)", r.calls)
	}
}

// Sanity: ensure the status name → option ID lookup uses the config map.
func TestWriteStatus_PassesCorrectOptionID(t *testing.T) {
	cfg := mockConfig()
	want := cfg.Project.Fields.Status.Options["In Review"]
	if want == "" {
		t.Fatalf("test setup error: In Review option missing from mock config")
	}

	var capturedArgs []string
	captureRunner := captureRunnerFn(func(name string, args ...string) ([]byte, error) {
		capturedArgs = append([]string{name}, args...)
		return nil, nil
	})

	if err := WriteStatus(context.Background(), captureRunner, cfg, "ITEM-X", "In Review", 0); err != nil {
		t.Fatal(err)
	}

	joined := fmt.Sprintf("%v", capturedArgs)
	if !contains(joined, want) {
		t.Errorf("expected option ID %q in args; got: %s", want, joined)
	}
	if !contains(joined, "ITEM-X") {
		t.Errorf("expected ITEM-X in args; got: %s", joined)
	}
}

// captureRunnerFn is a one-line Runner implementation for capture tests.
type captureRunnerFn func(name string, args ...string) ([]byte, error)

func (f captureRunnerFn) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return f(name, args...)
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `WriteStatus` undefined.

- [ ] **Step 3: Implement `statuswrite.go`**

Create `statuswrite.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// WriteStatus sets the Status single-select field on a project item to the
// option named statusName. Retries up to 3 times with exponential backoff
// (1×, 2×, 4× of baseDelay). When baseDelay is 0 it defaults to 1s.
//
// Returns an error after exhausting retries; caller logs and continues
// (status writes are best-effort by design — reconcile fixes any drift).
func WriteStatus(ctx context.Context, r Runner, cfg *Config, itemID, statusName string, baseDelay time.Duration) error {
	optionID, ok := cfg.Project.Fields.Status.Options[statusName]
	if !ok {
		return fmt.Errorf("unknown Status option %q (config has %d options)",
			statusName, len(cfg.Project.Fields.Status.Options))
	}
	if baseDelay == 0 {
		baseDelay = time.Second
	}

	var lastErr error
	delay := baseDelay
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := r.Run(ctx, "gh", "project", "item-edit",
			"--id", itemID,
			"--field-id", cfg.Project.Fields.Status.ID,
			"--project-id", cfg.Project.NodeID,
			"--single-select-option-id", optionID,
		)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}
	return fmt.Errorf("set Status to %q on item %s after 3 attempts: %w", statusName, itemID, lastErr)
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add statuswrite.go statuswrite_test.go
git commit -m "feat: status writer with 3-retry exponential backoff"
```

---

## Task 11: Progress event logger

**Files:**
- Create: `/Users/jp/Source/klanky/progress.go`
- Create: `/Users/jp/Source/klanky/progress_test.go`

The `ProgressLogger` writes timestamped lines to a configurable `io.Writer` (stderr in production). Typed event methods make the runner code read clearly.

- [ ] **Step 1: Write failing tests**

Create `progress_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProgress_ReconcileLine(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 15, 0, time.Local)))

	p.Reconciled(4, "set #41 → Done (issue closed)")

	got := buf.String()
	if !strings.Contains(got, "[09:32:15]") {
		t.Errorf("missing timestamp; got: %s", got)
	}
	if !strings.Contains(got, "reconcile: 4 tasks scanned") {
		t.Errorf("missing reconcile body; got: %s", got)
	}
	if !strings.Contains(got, "set #41 → Done") {
		t.Errorf("missing per-task transition; got: %s", got)
	}
}

func TestProgress_PhaseSummary(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 16, 0, time.Local)))

	p.PhaseSelected(2, 3, 2, 0)

	got := buf.String()
	if !strings.Contains(got, "phase 2") {
		t.Errorf("missing phase number; got: %s", got)
	}
	if !strings.Contains(got, "5 tasks ready (3 todo, 2 needs-attention)") {
		t.Errorf("missing eligibility breakdown; got: %s", got)
	}
}

func TestProgress_TaskTransitions(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, fixedClock(time.Date(2026, 4, 26, 9, 32, 17, 0, time.Local)))

	p.TaskInProgress(42)
	p.TaskInReview(44, 88)
	p.TaskNeedsAttention(42, 3)

	got := buf.String()
	for _, want := range []string{
		"task #42 → in-progress",
		"task #44 → in-review (PR #88)",
		"task #42 → needs-attention (3rd attempt)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestProgress_OrdinalSuffixes(t *testing.T) {
	cases := map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd"}
	for n, want := range cases {
		got := ordinal(n)
		if got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

// fixedClock returns a clock function that always returns the given time.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `progress.go`**

Create `progress.go`:

```go
package main

import (
	"fmt"
	"io"
	"time"
)

// Progress emits human-readable timestamped event lines. Output goes to a
// configurable writer (stderr in production) so that the end-of-run summary
// table on stdout stays clean for shell composition.
type Progress struct {
	w     io.Writer
	clock func() time.Time
}

// NewProgress returns a Progress writing to w using clock for timestamps.
// Pass time.Now in production; tests inject a fixed clock.
func NewProgress(w io.Writer, clock func() time.Time) *Progress {
	if clock == nil {
		clock = time.Now
	}
	return &Progress{w: w, clock: clock}
}

func (p *Progress) line(format string, args ...any) {
	fmt.Fprintf(p.w, "[%s] %s\n", p.clock().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// Reconciled summarizes the reconcile pass.
func (p *Progress) Reconciled(scanned int, summary string) {
	if summary == "" {
		p.line("reconcile: %d tasks scanned, no changes", scanned)
	} else {
		p.line("reconcile: %d tasks scanned, %s", scanned, summary)
	}
}

// PhaseSelected reports the chosen phase and the work-queue breakdown.
func (p *Progress) PhaseSelected(phase, todo, needsAttention, awaitingReview int) {
	p.line("phase %d: %d tasks ready (%d todo, %d needs-attention), %d awaiting review",
		phase, todo+needsAttention, todo, needsAttention, awaitingReview)
}

// TaskInProgress logs that a task started executing.
func (p *Progress) TaskInProgress(taskNumber int) {
	p.line("task #%d → in-progress", taskNumber)
}

// TaskInReview logs that a task succeeded with an open PR.
func (p *Progress) TaskInReview(taskNumber, prNumber int) {
	p.line("task #%d → in-review (PR #%d)", taskNumber, prNumber)
}

// TaskNeedsAttention logs that a task ended in needs-attention with the
// running attempt count.
func (p *Progress) TaskNeedsAttention(taskNumber, attempt int) {
	p.line("task #%d → needs-attention (%s attempt)", taskNumber, ordinal(attempt))
}

// Note logs an arbitrary line (used for setup messages, summaries, etc.).
func (p *Progress) Note(format string, args ...any) {
	p.line(format, args...)
}

// ordinal formats a positive integer with its English ordinal suffix.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add progress.go progress_test.go
git commit -m "feat: progress logger with timestamped event lines"
```

---

## Task 12: Summary table

**Files:**
- Create: `/Users/jp/Source/klanky/summary.go`
- Create: `/Users/jp/Source/klanky/summary_test.go`

`text/tabwriter` table on stdout, plus footer counts and dynamic next-step line.

- [ ] **Step 1: Write failing tests**

Create `summary_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderSummary_AllInReview(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase:    2,
		Duration: 18*time.Minute + 43*time.Second,
		Rows: []SummaryRow{
			{Task: 43, Status: "in-review", Link: "https://github.com/o/r/pull/89"},
			{Task: 44, Status: "in-review", Link: "https://github.com/o/r/pull/88"},
		},
	}, buf)

	got := buf.String()
	for _, want := range []string{
		"Phase 2 run complete in 18m43s",
		"#43",
		"https://github.com/o/r/pull/89",
		"2 tasks attempted: 2 in-review, 0 needs-attention",
		"review the 2 PRs above",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSummary_MixedOutcomes(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase:    2,
		Duration: 5 * time.Minute,
		Rows: []SummaryRow{
			{Task: 42, Status: "needs-attention", Link: "https://github.com/o/r/issues/42", Note: "3rd attempt"},
			{Task: 43, Status: "in-review", Link: "https://github.com/o/r/pull/89"},
		},
	}, buf)

	got := buf.String()
	if !strings.Contains(got, "1 in-review, 1 needs-attention") {
		t.Errorf("counts wrong:\n%s", got)
	}
	if !strings.Contains(got, "address") {
		t.Errorf("missing address-needs-attention note:\n%s", got)
	}
}

func TestRenderSummary_FeatureComplete(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{FeatureComplete: true, FeatureNumber: 7, TotalTasks: 12}, buf)

	got := buf.String()
	for _, want := range []string{
		"Feature #7 is complete",
		"all 12 tasks closed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSummary_OnlyAwaitingReview(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderSummary(SummaryData{
		Phase: 2,
		AwaitingReviewLinks: []string{
			"#42 https://github.com/o/r/pull/77",
			"#43 https://github.com/o/r/pull/78",
		},
	}, buf)

	got := buf.String()
	for _, want := range []string{
		"Phase 2 has 2 PRs awaiting your review",
		"https://github.com/o/r/pull/77",
		"Merge or close them",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `summary.go`**

Create `summary.go`:

```go
package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// SummaryData feeds RenderSummary. Use the FeatureComplete branch when the
// whole feature is done, the AwaitingReviewLinks branch when the phase only
// has open PRs (no eligible work was found), or Rows otherwise.
type SummaryData struct {
	// Branch 1: whole feature is closed.
	FeatureComplete bool
	FeatureNumber   int
	TotalTasks      int

	// Branch 2: phase has only awaiting-review tasks (no work was attempted).
	AwaitingReviewLinks []string // pre-formatted "#42 https://..."

	// Branch 3: tasks were attempted.
	Phase    int
	Duration time.Duration
	Rows     []SummaryRow
}

// SummaryRow is one line in the attempted-tasks table.
type SummaryRow struct {
	Task   int
	Status string // "in-review" or "needs-attention"
	Link   string
	Note   string // optional (e.g. "3rd attempt")
}

// RenderSummary writes the end-of-run summary to w. Branch order matters —
// FeatureComplete and AwaitingReviewLinks are checked before Rows.
func RenderSummary(d SummaryData, w io.Writer) {
	if d.FeatureComplete {
		fmt.Fprintf(w, "Feature #%d is complete: all %d tasks closed.\n", d.FeatureNumber, d.TotalTasks)
		return
	}
	if len(d.AwaitingReviewLinks) > 0 {
		fmt.Fprintf(w, "Phase %d has %d PRs awaiting your review:\n", d.Phase, len(d.AwaitingReviewLinks))
		for _, link := range d.AwaitingReviewLinks {
			fmt.Fprintf(w, "  - %s\n", link)
		}
		fmt.Fprintln(w, "Merge or close them, then re-run.")
		return
	}

	fmt.Fprintf(w, "Phase %d run complete in %s.\n\n", d.Phase, d.Duration.Round(time.Second))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TASK\tSTATUS\tLINK\tNOTE")
	inReview := 0
	needsAttention := 0
	for _, row := range d.Rows {
		switch row.Status {
		case "in-review":
			inReview++
		case "needs-attention":
			needsAttention++
		}
		fmt.Fprintf(tw, "  #%d\t%s\t%s\t%s\n", row.Task, row.Status, row.Link, row.Note)
	}
	tw.Flush()

	fmt.Fprintf(w, "\n%d tasks attempted: %d in-review, %d needs-attention.\n",
		len(d.Rows), inReview, needsAttention)

	switch {
	case inReview > 0 && needsAttention > 0:
		fmt.Fprintf(w, "Next: review the %d PRs above. Re-run `klanky run <F>` after merging.\n", inReview)
		fmt.Fprintf(w, "      Also: address %d task(s) in needs-attention before re-running (or they'll auto-retry).\n", needsAttention)
	case inReview > 0:
		fmt.Fprintf(w, "Next: review the %d PRs above. Re-run `klanky run <F>` after merging.\n", inReview)
	case needsAttention > 0:
		fmt.Fprintln(w, "Next: all attempts failed; inspect the breadcrumbs and re-run.")
	}
}
```

- [ ] **Step 4: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add summary.go summary_test.go
git commit -m "feat: end-of-run summary with text/tabwriter table"
```

---

## Task 13: Top-level orchestration in `runner.go`

**Files:**
- Create: `/Users/jp/Source/klanky/runner.go`
- Create: `/Users/jp/Source/klanky/runner_test.go`
- Modify: `/Users/jp/Source/klanky/cmd_run.go`

This task wires every component together: lock → fetch → reconcile (apply) → workqueue → spawn errgroup → summary. After this task, `klanky run <feature-id>` works end-to-end.

- [ ] **Step 1: Write failing integration test for the happy path**

Create `runner_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunFeature_HappyPath_OneEligibleTaskOpensPR(t *testing.T) {
	repoRoot := t.TempDir()
	klankyDir := filepath.Join(repoRoot, ".klanky")
	if err := os.MkdirAll(klankyDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := mockConfig()
	r := NewFakeRunner()

	// Snapshot fetch.
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 7, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 42, "title": "T", "body": "## Context\n...",
				"state": "OPEN", "id": "I_42",
				"projectItems": {"nodes": [{
					"id": "PVTI_42", "project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state,closed,merged",
		"--limit", "200"},
		[]byte(`[]`), nil)

	// Worktree creation.
	wtPath := WorktreePath(filepath.Join(repoRoot, "wt-root"), "proj", 7, 42)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "prune"}, nil, nil)
	r.Stub([]string{"git", "-C", repoRoot, "worktree", "add", wtPath, "-b", "klanky/feat-7/task-42", "main"}, nil, nil)

	// Status writes (in-progress, then in-review). Both succeed first try.
	r.Stub([]string{"gh", "project", "item-edit",
		"--id", "PVTI_42",
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", cfg.Project.Fields.Status.Options["In Progress"]}, nil, nil)
	r.Stub([]string{"gh", "project", "item-edit",
		"--id", "PVTI_42",
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", cfg.Project.Fields.Status.Options["In Review"]}, nil, nil)

	// Post-spawn verification.
	r.Stub([]string{"git", "-C", wtPath, "rev-list", "--count", "main..HEAD"}, []byte("3\n"), nil)
	r.Stub([]string{"gh", "pr", "list", "--repo", "alice/proj",
		"--head", "klanky/feat-7/task-42", "--state", "open",
		"--json", "url,number"},
		[]byte(`[{"url":"https://github.com/alice/proj/pull/77","number":77}]`), nil)

	sp := &FakeSpawner{}
	sp.Stub(0, "ok\n", "", nil)

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := RunFeature(context.Background(), RunFeatureDeps{
		Runner: r, Spawner: sp, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     NewProgress(progBuf, fixedClock(time.Now())),
		SummaryOut:   sumBuf,
		Timeout:      time.Minute,
	})
	if err != nil {
		t.Fatalf("RunFeature: %v", err)
	}

	if !strings.Contains(sumBuf.String(), "1 in-review") {
		t.Errorf("summary missing in-review count:\n%s", sumBuf.String())
	}
	if !strings.Contains(progBuf.String(), "task #42 → in-progress") {
		t.Errorf("progress missing in-progress event:\n%s", progBuf.String())
	}
	if !strings.Contains(progBuf.String(), "task #42 → in-review (PR #77)") {
		t.Errorf("progress missing in-review event:\n%s", progBuf.String())
	}
}

func TestRunFeature_FeatureComplete_ShortCircuits(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".klanky"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := mockConfig()
	r := NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 7, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 42, "title": "T", "body": "...",
				"state": "CLOSED", "id": "I_42",
				"projectItems": {"nodes": [{
					"id": "PVTI_42", "project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Done", "optionId": "e"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub([]string{"gh", "api", "graphql",
		"-f", "query=" + snapshotQuery,
		"-F", "number=7",
		"-f", "owner=alice",
		"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub([]string{"gh", "pr", "list",
		"--repo", "alice/proj",
		"--state", "all",
		"--search", "head:klanky/feat-7/",
		"--json", "headRefName,number,url,state,closed,merged",
		"--limit", "200"},
		[]byte(`[]`), nil)

	progBuf := &bytes.Buffer{}
	sumBuf := &bytes.Buffer{}

	err := RunFeature(context.Background(), RunFeatureDeps{
		Runner: r, Spawner: &FakeSpawner{}, Config: cfg,
		RepoRoot: repoRoot, FeatureID: 7,
		WorktreeRoot: filepath.Join(repoRoot, "wt-root"),
		Progress:     NewProgress(progBuf, fixedClock(time.Now())),
		SummaryOut:   sumBuf,
		Timeout:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sumBuf.String(), "Feature #7 is complete") {
		t.Errorf("summary missing feature-complete message:\n%s", sumBuf.String())
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `RunFeature`, `RunFeatureDeps` undefined.

- [ ] **Step 3: Implement `runner.go`**

Create `runner.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const concurrencyLimit = 5

// RunFeatureDeps bundles the dependencies of RunFeature so the call signature
// stays manageable as features are added.
type RunFeatureDeps struct {
	Runner       Runner
	Spawner      Spawner
	Config       *Config
	RepoRoot     string
	FeatureID    int
	WorktreeRoot string // typically ~/.klanky/worktrees
	Progress     *Progress
	SummaryOut   io.Writer
	Timeout      time.Duration // per-task agent timeout
}

// RunFeature is the top-level orchestrator for `klanky run <feature-id>`.
// Sequence: lock → fetch snapshot → reconcile (apply) → select work →
// spawn agents in parallel → render summary → release lock.
func RunFeature(ctx context.Context, d RunFeatureDeps) error {
	// 1. Lock.
	lockPath := filepath.Join(d.RepoRoot, ".klanky", fmt.Sprintf("runner-%d.lock", d.FeatureID))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("mkdir .klanky: %w", err)
	}
	lock, err := AcquireLock(lockPath)
	if err != nil {
		return err
	}
	defer lock.Release()

	// 2. Fetch snapshot.
	snap, err := FetchSnapshot(ctx, d.Runner, d.Config, d.FeatureID)
	if err != nil {
		return err
	}

	// 3. Reconcile: compute and apply actions, mutate snapshot in-memory so
	// SelectWork sees the post-reconcile state.
	actions := Reconcile(snap, d.FeatureID)
	reconcileSummary := applyReconcile(ctx, d.Runner, d.Config, snap, actions, d.FeatureID)
	d.Progress.Reconciled(len(snap.Tasks), reconcileSummary)

	// 4. Pick work.
	wq := SelectWork(snap)

	// 5. Handle nothing-to-do scenarios.
	if wq.AllDone {
		RenderSummary(SummaryData{
			FeatureComplete: true,
			FeatureNumber:   d.FeatureID,
			TotalTasks:      len(snap.Tasks),
		}, d.SummaryOut)
		return nil
	}
	if len(wq.SurvivingInProgress) > 0 {
		return fmt.Errorf("phase %d has %d tasks in unexpected in-progress state — this is a bug; see logs",
			wq.CurrentPhase, len(wq.SurvivingInProgress))
	}
	if len(wq.Eligible) == 0 {
		// Only awaiting-review tasks in this phase.
		links := make([]string, 0, len(wq.AwaitingReview))
		for _, t := range wq.AwaitingReview {
			pr, ok := snap.PRsByBranch[BranchForTask(d.FeatureID, t.Number)]
			if ok {
				links = append(links, fmt.Sprintf("#%d %s", t.Number, pr.URL))
			} else {
				links = append(links, fmt.Sprintf("#%d (no PR found)", t.Number))
			}
		}
		RenderSummary(SummaryData{
			Phase:               wq.CurrentPhase,
			AwaitingReviewLinks: links,
		}, d.SummaryOut)
		return nil
	}

	d.Progress.PhaseSelected(
		wq.CurrentPhase,
		countByStatus(wq.Eligible, "Todo")+countByStatus(wq.Eligible, ""),
		countByStatus(wq.Eligible, "Needs Attention"),
		len(wq.AwaitingReview),
	)

	// 6. Spawn agents in parallel, capped at 5.
	eg, gctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(concurrencyLimit)
	results := make([]TaskResult, len(wq.Eligible))
	var resultsMu sync.Mutex
	startedAt := time.Now()

	repoSlug := d.Config.Repo.Owner + "/" + d.Config.Repo.Name

	for i, task := range wq.Eligible {
		i, task := i, task
		eg.Go(func() error {
			if err := sem.Acquire(gctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			res := runOneTask(gctx, d, snap, task, repoSlug)
			resultsMu.Lock()
			results[i] = res
			resultsMu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// 7. Render summary.
	rows := make([]SummaryRow, 0, len(results))
	for _, r := range results {
		row := SummaryRow{Task: r.TaskNumber, Status: r.Outcome.String()}
		switch r.Outcome {
		case OutcomeInReview:
			if r.PR != nil {
				row.Link = r.PR.URL
			}
		case OutcomeNeedsAttention:
			row.Link = fmt.Sprintf("https://github.com/%s/issues/%d", repoSlug, r.TaskNumber)
		}
		rows = append(rows, row)
	}
	RenderSummary(SummaryData{
		Phase: wq.CurrentPhase, Duration: time.Since(startedAt), Rows: rows,
	}, d.SummaryOut)

	return nil
}

func runOneTask(ctx context.Context, d RunFeatureDeps, snap *Snapshot, task TaskInfo, repoSlug string) TaskResult {
	wtPath := WorktreePath(d.WorktreeRoot, d.Config.Repo.Name, d.FeatureID, task.Number)
	branch := BranchForTask(d.FeatureID, task.Number)
	logPath := filepath.Join(d.RepoRoot, ".klanky", "logs", fmt.Sprintf("task-%d.log", task.Number))

	if err := EnsureCleanWorktree(ctx, d.Runner, d.RepoRoot, wtPath, branch, "main"); err != nil {
		return TaskResult{
			TaskNumber: task.Number, Outcome: OutcomeNeedsAttention,
			OutcomeReason: fmt.Sprintf("worktree setup failed: %v", err),
		}
	}

	if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Progress", time.Second); err != nil {
		// Best-effort; log via progress and continue.
		d.Progress.Note("warn: could not set Status=In Progress for #%d: %v", task.Number, err)
	}
	d.Progress.TaskInProgress(task.Number)

	res, err := RunAgent(ctx, d.Runner, d.Spawner, AgentJob{
		FeatureID: d.FeatureID, Task: task,
		WorktreePath: wtPath, LogPath: logPath, RepoSlug: repoSlug,
		Timeout: d.Timeout,
	})
	if err != nil {
		return TaskResult{
			TaskNumber: task.Number, Outcome: OutcomeNeedsAttention,
			OutcomeReason: fmt.Sprintf("agent error: %v", err),
		}
	}

	switch res.Outcome {
	case OutcomeInReview:
		if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "In Review", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=In Review for #%d: %v", task.Number, err)
		}
		prNum := 0
		if res.PR != nil {
			prNum = res.PR.Number
		}
		d.Progress.TaskInReview(task.Number, prNum)

	case OutcomeNeedsAttention:
		if err := WriteStatus(ctx, d.Runner, d.Config, task.ItemID, "Needs Attention", time.Second); err != nil {
			d.Progress.Note("warn: could not set Status=Needs Attention for #%d: %v", task.Number, err)
		}
		// Compose and post breadcrumb (best-effort).
		prior, _ := CountPriorAttempts(ctx, d.Runner, repoSlug, task.Number)
		attempt := prior + 1
		body := BuildBreadcrumb(BreadcrumbData{
			Attempt: attempt, StartedAt: res.StartedAt, Duration: res.Duration,
			Outcome: res.OutcomeReason, WorktreePath: wtPath, LogPath: logPath,
			LastLogLines: tailLog(logPath, 20),
		})
		if err := PostBreadcrumb(ctx, d.Runner, repoSlug, task.Number, body); err != nil {
			d.Progress.Note("warn: could not post breadcrumb for #%d: %v", task.Number, err)
		}
		d.Progress.TaskNeedsAttention(task.Number, attempt)
	}
	return *res
}

func tailLog(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return TailLines(string(data), n)
}

// applyReconcile mutates snap in-place to reflect the reconcile actions, then
// applies them to GitHub. Returns a short human-readable summary string used
// in the progress event line, or "" when no actions were applied.
func applyReconcile(ctx context.Context, r Runner, cfg *Config, snap *Snapshot, actions []ReconcileAction, featureID int) string {
	if len(actions) == 0 {
		return ""
	}
	for _, a := range actions {
		// Update in-memory snapshot so SelectWork sees post-reconcile state.
		for i := range snap.Tasks {
			if snap.Tasks[i].Number == a.TaskNumber {
				snap.Tasks[i].Status = a.NewStatus
				break
			}
		}
		// Best-effort writes; failures are logged via stderr by WriteStatus internals.
		if err := WriteStatus(ctx, r, cfg, a.ItemID, a.NewStatus, time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "klanky: reconcile WriteStatus #%d → %s failed: %v\n", a.TaskNumber, a.NewStatus, err)
		}
		if a.Breadcrumb != "" {
			body := fmt.Sprintf("%s\n**Klanky reconcile**\n\n%s\n", klankyAttemptSentinel, a.Breadcrumb)
			if err := PostBreadcrumb(ctx, r, cfg.Repo.Owner+"/"+cfg.Repo.Name, a.TaskNumber, body); err != nil {
				fmt.Fprintf(os.Stderr, "klanky: reconcile PostBreadcrumb #%d failed: %v\n", a.TaskNumber, err)
			}
		}
	}
	first := actions[0]
	if len(actions) == 1 {
		return fmt.Sprintf("set #%d → %s", first.TaskNumber, first.NewStatus)
	}
	return fmt.Sprintf("applied %d status updates (first: #%d → %s)", len(actions), first.TaskNumber, first.NewStatus)
}

func countByStatus(tasks []TaskInfo, status string) int {
	n := 0
	for _, t := range tasks {
		if t.Status == status {
			n++
		}
	}
	return n
}
```

- [ ] **Step 4: Add the `golang.org/x/sync` dependency**

Run:
```bash
cd /Users/jp/Source/klanky
go get golang.org/x/sync@v0.10.0
```

Expected: `go.mod` and `go.sum` updated to include `golang.org/x/sync`.

- [ ] **Step 5: Wire `RunFeature` into `cmd_run.go`**

Replace the stub body of the `RunE` in `cmd_run.go`. Open `/Users/jp/Source/klanky/cmd_run.go` and replace its full contents with:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

const defaultTaskTimeout = 20 * time.Minute

func newRunCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <feature-id>",
		Short: "Execute the current phase of a feature: spawn parallel agents, open PRs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			featureID, err := strconv.Atoi(args[0])
			if err != nil || featureID < 1 {
				return fmt.Errorf("feature-id must be a positive integer, got %q", args[0])
			}

			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			repoRoot, err := filepath.Abs(filepath.Dir(cfgPath))
			if err != nil {
				return fmt.Errorf("resolve repo root: %w", err)
			}

			wtRoot, err := DefaultWorktreeRoot()
			if err != nil {
				return err
			}

			progress := NewProgress(os.Stderr, time.Now)

			return RunFeature(cmd.Context(), RunFeatureDeps{
				Runner:       RealRunner{},
				Spawner:      RealSpawner{},
				Config:       cfg,
				RepoRoot:     repoRoot,
				FeatureID:    featureID,
				WorktreeRoot: wtRoot,
				Progress:     progress,
				SummaryOut:   cmd.OutOrStdout(),
				Timeout:      defaultTaskTimeout,
			})
		},
	}
	return cmd
}
```

This replaces the stub from Task 1. Note: the `cmd_run_test.go` test
`TestRunCmd_ValidArgs_LoadsConfig` from Task 1 expected the stub to print
"TODO: not implemented" — that test will now fail. Update it.

- [ ] **Step 6: Update `cmd_run_test.go` to reflect the real behavior**

Replace `TestRunCmd_ValidArgs_LoadsConfig` in `cmd_run_test.go` with:

```go
func TestRunCmd_ValidArgs_AttemptsToRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	contents := `{
		"schema_version": 1,
		"repo": {"owner": "alice", "name": "proj"},
		"project": {
			"url": "https://github.com/users/alice/projects/1",
			"number": 1, "node_id": "PVT_x",
			"owner_login": "alice", "owner_type": "User",
			"fields": {
				"phase":  {"id": "PVTF_p", "name": "Phase"},
				"status": {"id": "PVTSSF_s", "name": "Status",
					"options": {"Todo": "a", "In Progress": "b", "In Review": "c", "Needs Attention": "d", "Done": "e"}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(cfgPath, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	cmd := newRunCmd(cfgPath)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"42"})

	err := cmd.Execute()
	// We expect this to fail because the real `gh` will reject the call (no auth
	// in test env) — but it must NOT fail at config-load time, and the error
	// message should not be the "TODO: not implemented" stub.
	if err == nil {
		// Could also succeed in environments where gh is set up; that's fine.
		return
	}
	if strings.Contains(err.Error(), "TODO") {
		t.Errorf("stub still wired; err: %v", err)
	}
}
```

- [ ] **Step 7: Run all tests; confirm pass**

Run: `go test ./...`
Expected: PASS for unit tests. The new `cmd_run_test.go` happy-path test only verifies "didn't fail at config load" — actual end-to-end execution requires a live gh + claude environment, validated manually in Task 14 below.

- [ ] **Step 8: Commit**

```bash
git add runner.go runner_test.go cmd_run.go cmd_run_test.go go.mod go.sum
git commit -m "feat: top-level runner orchestration — lock, reconcile, parallel spawn, summary"
```

---

## Task 14: Manual end-to-end smoke test

**Files:** none — verification step only.

After all unit tests pass, smoke-test against the real klanky repo's own GitHub Project.

- [ ] **Step 1: Confirm prerequisites**

```bash
which claude
which gh
gh auth status
```

Expected: all three succeed; `gh auth status` shows you're authenticated with `project` scope.

- [ ] **Step 2: Pick (or create) a tiny disposable feature in the klanky project**

Either reuse the existing smoke spec at `.klanky/specs/smoke-task-1.md` or create a new one. Then create the feature + a single trivial task:

```bash
cd /Users/jp/Source/klanky
go build -o klanky .
./klanky feature new --title "Runner smoke test"
# Note the feature_id from the JSON output, e.g. 42.

./klanky task add --feature 42 --phase 1 \
  --title "Add a TODO comment to README" \
  --spec-file .klanky/specs/smoke-task-1.md
# Note the task_id, e.g. 43.
```

- [ ] **Step 3: Run the runner**

```bash
./klanky run 42
```

Expected:
- Stderr shows the reconcile line, phase-selection line, and `task #43 → in-progress` event.
- Some time later (could be a few minutes for the agent to do trivial work), `task #43 → in-review (PR #N)`.
- Stdout shows the summary table with one row in `in-review` status and a PR URL.
- Visit the project board: task should be in the "In Review" lane.
- Visit the task issue: should have a sub-issue link from the feature.
- Visit the PR: should target `main`, body should contain `Closes #43`.

- [ ] **Step 4: Verify reconcile catches up after merge**

Manually merge the PR. Then:

```bash
./klanky run 42
```

Expected:
- Stderr shows reconcile line mentioning the task transition.
- Summary on stdout: `Feature #42 is complete: all 1 tasks closed.`
- Project board: task in "Done" lane.

- [ ] **Step 5: (Optional) Test the needs-attention path**

Create a task whose spec is impossible to satisfy in the timeout:

```bash
echo "## Context\nProve the Riemann hypothesis." > /tmp/impossible.md
./klanky task add --feature 42 --phase 2 --title "Riemann" --spec-file /tmp/impossible.md
./klanky run 42
```

After 20 minutes, expect:
- `task → needs-attention (1st attempt)` on stderr.
- Summary row showing `needs-attention` with the issue URL.
- The task issue should have a `<!-- klanky-attempt -->` comment with the breadcrumb.
- The worktree at `~/.klanky/worktrees/klanky/feat-42/task-N/` should still exist.

Re-run:

```bash
./klanky run 42
```

Expected on the second invocation:
- Reconcile leaves needs-attention alone (Row 9).
- The same task is picked up again as eligible.
- After it fails again: `task → needs-attention (2nd attempt)`.

If everything above behaves correctly, the runner is shippable.

- [ ] **Step 6: Cleanup**

Close any disposable issues and delete any stale worktrees:

```bash
git -C /Users/jp/Source/klanky worktree list
git -C /Users/jp/Source/klanky worktree remove <path>  # for each smoke worktree
```

---

## Self-review checklist

After completing all 14 tasks above, the executing engineer should confirm:

1. **All locked design decisions implemented:**
   - [x] Source of truth = issue-closed (Task 4 reconcile)
   - [x] Auto-retry of needs-attention (Task 5 SelectWork includes "Needs Attention" in Eligible)
   - [x] Attempt-count in summary (Task 13 + Task 9 sentinel counting)
   - [x] 11-row reconcile matrix (Task 4)
   - [x] Batched fetch ≤ 100 sub-issues (Task 2)
   - [x] Lock file with PID + silent takeover (Task 3)
   - [x] Worktree at `~/.klanky/worktrees/...`, wipe on retry (Task 6)
   - [x] `claude -p` with `--permission-mode bypassPermissions` (Task 8)
   - [x] 20m timeout, SIGKILL process group on timeout (Task 8)
   - [x] Post-exit verification: branch + PR (Task 8)
   - [x] Status writer with 3-retry (Task 10)
   - [x] Breadcrumb format with `<!-- klanky-attempt -->` sentinel (Task 9)
   - [x] Progress on stderr, summary on stdout (Task 11 + 12 + 13)
   - [x] errgroup + semaphore.NewWeighted(5) (Task 13)
   - [x] Hard-coded `main` base, no `--base` flag (Task 7 + 13)
   - [x] Envelope contents match locked contract (Task 7)

2. **Exit codes:**
   - [x] 0 on normal completion / nothing-to-do (Task 13)
   - [x] 1 on setup error (Tasks 1, 13)
   - [x] cobra default 130 on Ctrl-C (no special handling needed)

3. **No placeholders, no TODOs in committed code.**

4. **All `go test ./...` tests pass.**

5. **Manual smoke test (Task 14) succeeded against the live repo.**
