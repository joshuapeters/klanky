# Klanky Bootstrap & Creation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the four "creation surface" commands of Klanky (`init`, `project link`, `feature new`, `task add`) plus their supporting infrastructure (config, gh wrapper, schema validation). After this plan is done, you can plan a feature with a Claude Code agent and have it call into Klanky to create real GitHub issues conforming to the schema. The runner is out of scope — that is plan 2.

**Architecture:** Single-package Go binary (`package main` at repo root) using `spf13/cobra` for CLI, `os/exec` for shelling out to the `gh` CLI, and `encoding/json` for parsing gh output. All GitHub interaction goes through `gh` (no Octokit, no direct REST/GraphQL HTTP — `gh api graphql` is the GraphQL escape hatch). All commands take a `Runner` interface so unit tests can inject a `FakeRunner` instead of executing real shell commands.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra` v1.8+, stdlib only otherwise (`os/exec`, `encoding/json`, `testing`). No testify, no go-cmp — keep dependencies minimal until a real pain point emerges.

---

## Context (locked design decisions you need to know)

This plan implements decisions made in a long design grilling on 2026-04-25. The full record is in user memory under `project_locked_design.md` and supporting files. Key points the executing engineer needs without reading those:

**The contract (from SCHEMA.md, which doesn't exist yet but is implicit in the code):**
- `Phase`: Projects v2 Number custom field on each task.
- `Status`: Projects v2 Single-select with options in this exact order and case: `Todo`, `In Progress`, `In Review`, `Needs Attention`, `Done`. Title Case with spaces (matches GitHub defaults).
- `klanky:feature`: a label on the repo, applied only to Feature issues (never tasks).
- Feature: a normal GitHub issue with the `klanky:feature` label, added to the project. Phase and Status fields are NULL on Features.
- Task: a GitHub *sub-issue* under a Feature parent. No label. Added to project. Phase set, Status set to `Todo` initially.

**`.klankyrc.json` lives at repo root** and contains the resolved Projects v2 IDs (project node, field IDs, single-select option IDs by name). It already exists for this repo — read its actual content before writing tests against it. Schema is shaped like:

```json
{
  "schema_version": 1,
  "repo": { "owner": "joshuapeters", "name": "klanky" },
  "project": {
    "url": "https://github.com/users/joshuapeters/projects/6",
    "number": 6,
    "node_id": "PVT_kwHOAI1Lus4BVtZD",
    "owner_login": "joshuapeters",
    "owner_type": "User",
    "fields": {
      "phase":  { "id": "PVTF_...",   "name": "Phase" },
      "status": { "id": "PVTSSF_...", "name": "Status",
        "options": {
          "Todo": "f75ad846", "In Progress": "47fc9ee4",
          "In Review": "b2a13a83", "Needs Attention": "c1b7c6d3",
          "Done": "98236657"
        }
      }
    }
  },
  "feature_label": { "name": "klanky:feature" }
}
```

**Output contract for `feature new` and `task add`:** single-line JSON to stdout, e.g. `{"feature_id": 117, "url": "https://..."}`. The planning agent parses this to chain calls.

**GitHub auth:** assume `gh` is installed and authenticated with `project` scope. Klanky doesn't manage tokens.

**TDD:** every task in this plan follows Red → Green → Commit. No "implement, then write tests later." If you find yourself wanting to skip the test-first step, the `tdd` skill (https://raw.githubusercontent.com/mattpocock/skills/main/tdd/SKILL.md) governs.

---

## File Structure

Single `package main` at the repo root. All files named for their responsibility, not their layer.

| File | Responsibility |
|---|---|
| `go.mod`, `go.sum` | Module definition (`github.com/joshuapeters/klanky`) |
| `.gitignore` | Ignore `.klanky/logs/`, `.klanky/runner-*.lock`, `.klanky/specs/`, build artifacts |
| `main.go` | Cobra root command, registers subcommands, dispatches |
| `config.go` | `Config` struct, `LoadConfig(path)`, `SaveConfig(path)` |
| `config_test.go` | Tests for config load/save round-trip and validation errors |
| `ghcli.go` | `Runner` interface, `RealRunner` impl (wraps `os/exec`), `FakeRunner` for tests, generic `RunGraphQL[T]` helper |
| `ghcli_test.go` | Tests for `FakeRunner` recording, `RunGraphQL` parsing, error paths |
| `schema.go` | Schema constants (status option names in order, field names, label name, schema version), `ProjectFields` struct mirroring gh's `field-list --format json` output, `ValidateProject(ProjectFields)` returning a structured diff |
| `schema_test.go` | Validation tests against conforming and non-conforming inputs |
| `output.go` | `PrintJSONLine(w, data)` helper for the planning-agent-facing output contract |
| `output_test.go` | Tests for output format |
| `cmd_init.go` | `klanky init` command: creates project, fields, label, writes config |
| `cmd_init_test.go` | Tests with `FakeRunner` for the gh call sequence |
| `cmd_projectlink.go` | `klanky project link <url>` command: validates existing project, writes config |
| `cmd_projectlink_test.go` | Tests for URL parsing, validation, config writing |
| `cmd_featurenew.go` | `klanky feature new --title ...` command |
| `cmd_featurenew_test.go` | Tests for the gh issue create + project add sequence |
| `cmd_taskadd.go` | `klanky task add ...` command (issue create + sub-issue link + project add + Phase + Status) |
| `cmd_taskadd_test.go` | Tests for the multi-call sequence |

16 files. The flat layout is deliberate — for ~1500 LOC of CLI code, splitting into `internal/...` packages adds friction without value. Refactor when a second binary or library consumer appears.

---

## Task 1: Bootstrap Go module and Cobra root

**Files:**
- Create: `/Users/jp/Source/klanky/go.mod`
- Create: `/Users/jp/Source/klanky/.gitignore`
- Create: `/Users/jp/Source/klanky/main.go`
- Create: `/Users/jp/Source/klanky/main_test.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/jp/Source/klanky
go mod init github.com/joshuapeters/klanky
go get github.com/spf13/cobra@v1.8.1
```

Expected: `go.mod` and `go.sum` created; `go.mod` declares `github.com/joshuapeters/klanky` and lists cobra as a dependency.

- [ ] **Step 2: Create `.gitignore`**

```
# Build artifacts
/klanky
/klanky.exe

# Klanky runtime state
.klanky/logs/
.klanky/runner-*.lock
.klanky/specs/

# Editor / OS
.DS_Store
.idea/
.vscode/
```

- [ ] **Step 3: Write the failing test for the root command**

Create `main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_Help_ListsAllSubcommands(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := newRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --help to succeed, got: %v", err)
	}

	helpText := out.String()
	for _, want := range []string{"init", "project", "feature", "task"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("expected --help to mention %q; got:\n%s", want, helpText)
		}
	}
}
```

- [ ] **Step 4: Run the test and confirm it fails**

Run: `go test ./...`
Expected: compile error — `newRootCmd undefined`.

- [ ] **Step 5: Implement `newRootCmd` minimally**

Create `main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "klanky",
		Short: "Orchestrate parallel coding agents against a GitHub-issue task graph",
		SilenceUsage: true,
	}

	// Subcommand stubs (real implementations land in later tasks).
	root.AddCommand(&cobra.Command{Use: "init", Short: "Bootstrap a new project for this repo"})
	root.AddCommand(&cobra.Command{Use: "project", Short: "Manage project linkage"})
	root.AddCommand(&cobra.Command{Use: "feature", Short: "Manage features"})
	root.AddCommand(&cobra.Command{Use: "task", Short: "Manage tasks"})

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Run the test and confirm it passes**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore main.go main_test.go
git commit -m "feat: bootstrap go module and cobra root command"
```

---

## Task 2: Config struct and loader

**Files:**
- Create: `/Users/jp/Source/klanky/config.go`
- Create: `/Users/jp/Source/klanky/config_test.go`

- [ ] **Step 1: Write failing tests for `LoadConfig`**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ReadsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	contents := `{
		"schema_version": 1,
		"repo": {"owner": "alice", "name": "myproj"},
		"project": {
			"url": "https://github.com/users/alice/projects/1",
			"number": 1,
			"node_id": "PVT_x",
			"owner_login": "alice",
			"owner_type": "User",
			"fields": {
				"phase":  {"id": "PVTF_p", "name": "Phase"},
				"status": {"id": "PVTSSF_s", "name": "Status",
					"options": {
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e"
					}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.Repo.Owner != "alice" {
		t.Errorf("Repo.Owner = %q, want alice", cfg.Repo.Owner)
	}
	if cfg.Project.NodeID != "PVT_x" {
		t.Errorf("Project.NodeID = %q, want PVT_x", cfg.Project.NodeID)
	}
	if got := cfg.Project.Fields.Status.Options["In Review"]; got != "c" {
		t.Errorf(`Status.Options["In Review"] = %q, want "c"`, got)
	}
}

func TestLoadConfig_MissingFile_ReturnsHelpfulError(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/.klankyrc.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), ".klankyrc.json") {
		t.Errorf("error message should mention .klankyrc.json: %v", err)
	}
}

func TestLoadConfig_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure (`LoadConfig undefined`)**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `Config` and `LoadConfig`**

Create `config.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	SchemaVersion int            `json:"schema_version"`
	Repo          ConfigRepo     `json:"repo"`
	Project       ConfigProject  `json:"project"`
	FeatureLabel  ConfigLabel    `json:"feature_label"`
}

type ConfigRepo struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type ConfigProject struct {
	URL        string         `json:"url"`
	Number     int            `json:"number"`
	NodeID     string         `json:"node_id"`
	OwnerLogin string         `json:"owner_login"`
	OwnerType  string         `json:"owner_type"`
	Fields     ConfigFields   `json:"fields"`
}

type ConfigFields struct {
	Phase  ConfigField       `json:"phase"`
	Status ConfigStatusField `json:"status"`
}

type ConfigField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ConfigStatusField struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Options map[string]string `json:"options"` // option name -> option ID
}

type ConfigLabel struct {
	Name string `json:"name"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .klankyrc.json at %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse .klankyrc.json at %s: %w", path, err)
	}
	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests; confirm they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Add a round-trip test for SaveConfig**

Append to `config_test.go`:

```go
func TestSaveConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")

	original := &Config{
		SchemaVersion: 1,
		Repo:          ConfigRepo{Owner: "alice", Name: "proj"},
		Project: ConfigProject{
			URL: "https://x", Number: 7, NodeID: "PVT_y",
			OwnerLogin: "alice", OwnerType: "User",
			Fields: ConfigFields{
				Phase:  ConfigField{ID: "p", Name: "Phase"},
				Status: ConfigStatusField{ID: "s", Name: "Status", Options: map[string]string{"Todo": "t"}},
			},
		},
		FeatureLabel: ConfigLabel{Name: "klanky:feature"},
	}
	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Project.NodeID != original.Project.NodeID {
		t.Errorf("NodeID round-trip failed: got %q, want %q", loaded.Project.NodeID, original.Project.NodeID)
	}
	if loaded.Project.Fields.Status.Options["Todo"] != "t" {
		t.Errorf("Status options round-trip failed")
	}
}
```

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config.go config_test.go
git commit -m "feat: add config struct, LoadConfig, SaveConfig with round-trip tests"
```

---

## Task 3: gh Runner interface, real impl, and FakeRunner

**Files:**
- Create: `/Users/jp/Source/klanky/ghcli.go`
- Create: `/Users/jp/Source/klanky/ghcli_test.go`

- [ ] **Step 1: Write failing tests for `FakeRunner`**

Create `ghcli_test.go`:

```go
package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFakeRunner_RecordsCallAndReturnsStubbedOutput(t *testing.T) {
	fake := NewFakeRunner()
	fake.Stub([]string{"gh", "issue", "view", "117"}, []byte(`{"number":117}`), nil)

	out, err := fake.Run(context.Background(), "gh", "issue", "view", "117")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != `{"number":117}` {
		t.Errorf("unexpected output: %s", out)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.Calls))
	}
	if got := strings.Join(fake.Calls[0], " "); got != "gh issue view 117" {
		t.Errorf("recorded call = %q", got)
	}
}

func TestFakeRunner_UnstubbedCall_ReturnsError(t *testing.T) {
	fake := NewFakeRunner()
	_, err := fake.Run(context.Background(), "gh", "unknown")
	if err == nil {
		t.Fatal("expected error for unstubbed call, got nil")
	}
	if !strings.Contains(err.Error(), "no stub") {
		t.Errorf("error should mention 'no stub': %v", err)
	}
}

func TestFakeRunner_StubbedError_IsReturned(t *testing.T) {
	fake := NewFakeRunner()
	want := errors.New("simulated gh failure")
	fake.Stub([]string{"gh", "fail"}, nil, want)

	_, err := fake.Run(context.Background(), "gh", "fail")
	if !errors.Is(err, want) {
		t.Errorf("expected stubbed error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `Runner`, `RealRunner`, `FakeRunner`**

Create `ghcli.go`:

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner abstracts subprocess execution so commands can be unit-tested
// against a FakeRunner without invoking real shell commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RealRunner shells out via os/exec. Stderr is captured into the returned
// error on non-zero exit so callers see what gh complained about.
type RealRunner struct{}

func (RealRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%s %s: %w; stderr: %s",
			name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// FakeRunner records all calls and returns stubbed responses.
// Unstubbed calls return an error rather than silently succeeding,
// to surface incomplete test setup.
type FakeRunner struct {
	Calls [][]string
	stubs []fakeStub
}

type fakeStub struct {
	args []string
	out  []byte
	err  error
}

func NewFakeRunner() *FakeRunner {
	return &FakeRunner{}
}

func (f *FakeRunner) Stub(argv []string, out []byte, err error) {
	f.stubs = append(f.stubs, fakeStub{args: argv, out: out, err: err})
}

func (f *FakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	full := append([]string{name}, args...)
	f.Calls = append(f.Calls, full)
	for _, s := range f.stubs {
		if argsEqual(s.args, full) {
			return s.out, s.err
		}
	}
	return nil, fmt.Errorf("no stub for: %s", strings.Join(full, " "))
}

func argsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Compile-time check that errors.Is works on RealRunner errors.
var _ = errors.Is
```

- [ ] **Step 4: Run tests; confirm they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ghcli.go ghcli_test.go
git commit -m "feat: add Runner interface with RealRunner and FakeRunner"
```

---

## Task 4: GraphQL helper

**Files:**
- Modify: `/Users/jp/Source/klanky/ghcli.go` (add `RunGraphQL`)
- Modify: `/Users/jp/Source/klanky/ghcli_test.go` (add tests)

- [ ] **Step 1: Write failing tests for `RunGraphQL`**

Append to `ghcli_test.go`:

```go
func TestRunGraphQL_ParsesDataIntoTarget(t *testing.T) {
	fake := NewFakeRunner()
	resp := `{"data":{"repository":{"issue":{"number":117,"title":"hi"}}}}`
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=QUERY"},
		[]byte(resp), nil,
	)

	type result struct {
		Repository struct {
			Issue struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
			} `json:"issue"`
		} `json:"repository"`
	}
	var got result
	if err := RunGraphQL(context.Background(), fake, "QUERY", nil, &got); err != nil {
		t.Fatalf("RunGraphQL: %v", err)
	}
	if got.Repository.Issue.Number != 117 {
		t.Errorf("number = %d, want 117", got.Repository.Issue.Number)
	}
	if got.Repository.Issue.Title != "hi" {
		t.Errorf("title = %q, want hi", got.Repository.Issue.Title)
	}
}

func TestRunGraphQL_ReturnsErrorOnGraphQLErrors(t *testing.T) {
	fake := NewFakeRunner()
	resp := `{"data":null,"errors":[{"message":"NOT_FOUND"}]}`
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=QUERY"},
		[]byte(resp), nil,
	)

	var dest struct{}
	err := RunGraphQL(context.Background(), fake, "QUERY", nil, &dest)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Errorf("error should contain GraphQL message: %v", err)
	}
}

func TestRunGraphQL_PassesVariablesAsFields(t *testing.T) {
	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=Q", "-F", "num=117", "-f", "name=alice"},
		[]byte(`{"data":{}}`), nil,
	)

	var dest struct{}
	err := RunGraphQL(context.Background(), fake, "Q",
		map[string]any{"num": 117, "name": "alice"}, &dest)
	if err != nil {
		t.Fatalf("RunGraphQL: %v", err)
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error — `RunGraphQL undefined`.

- [ ] **Step 3: Implement `RunGraphQL`**

Append to `ghcli.go`:

```go
import (
	"encoding/json"
	"sort"
)

// RunGraphQL executes a GraphQL query via `gh api graphql`, parses the response,
// and unmarshals .data into dest. Variables are passed via -F (typed) for
// numeric/bool values and -f (string) for strings. Map iteration order is
// non-deterministic in Go, so variables are emitted in sorted key order to
// keep test stubs predictable.
func RunGraphQL(ctx context.Context, r Runner, query string, vars map[string]any, dest any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}

	// Sort keys for deterministic argv order.
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := vars[k].(type) {
		case string:
			args = append(args, "-f", fmt.Sprintf("%s=%s", k, v))
		case int, int64, float64, bool:
			args = append(args, "-F", fmt.Sprintf("%s=%v", k, v))
		default:
			return fmt.Errorf("unsupported variable type for %q: %T", k, v)
		}
	}

	out, err := r.Run(ctx, "gh", args...)
	if err != nil {
		return fmt.Errorf("graphql call: %w", err)
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return fmt.Errorf("parse graphql envelope: %w", err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	if dest != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, dest); err != nil {
			return fmt.Errorf("parse graphql data: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests; confirm they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ghcli.go ghcli_test.go
git commit -m "feat: add RunGraphQL helper with variable typing and error parsing"
```

---

## Task 5: Schema constants and validation

**Files:**
- Create: `/Users/jp/Source/klanky/schema.go`
- Create: `/Users/jp/Source/klanky/schema_test.go`

- [ ] **Step 1: Write failing tests for `ValidateProject`**

Create `schema_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func conformingFields() ProjectFields {
	return ProjectFields{Fields: []ProjectField{
		{Name: "Phase", Type: "ProjectV2Field"},
		{Name: "Status", Type: "ProjectV2SingleSelectField", Options: []ProjectFieldOption{
			{Name: "Todo"}, {Name: "In Progress"}, {Name: "In Review"},
			{Name: "Needs Attention"}, {Name: "Done"},
		}},
	}}
}

func TestValidateProject_Conforming_ReturnsNoErrors(t *testing.T) {
	if errs := ValidateProject(conformingFields()); len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateProject_MissingPhaseField(t *testing.T) {
	pf := conformingFields()
	pf.Fields = pf.Fields[1:] // drop Phase
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(errs[0], "Phase") {
		t.Errorf("error should mention Phase: %q", errs[0])
	}
}

func TestValidateProject_PhaseFieldWrongType(t *testing.T) {
	pf := conformingFields()
	pf.Fields[0].Type = "ProjectV2SingleSelectField"
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
}

func TestValidateProject_StatusMissingOption(t *testing.T) {
	pf := conformingFields()
	pf.Fields[1].Options = pf.Fields[1].Options[:4] // drop "Done"
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "Done") {
		t.Errorf("error should mention missing Done option: %s", joined)
	}
}

func TestValidateProject_StatusOptionWrongCase(t *testing.T) {
	pf := conformingFields()
	pf.Fields[1].Options[0].Name = "todo" // lowercase
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
}
```

- [ ] **Step 2: Run tests; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement schema constants and validator**

Create `schema.go`:

```go
package main

import "fmt"

const (
	SchemaVersion    = 1
	FieldNamePhase   = "Phase"
	FieldNameStatus  = "Status"
	FieldTypeNumber  = "ProjectV2Field" // gh reports both NUMBER and TEXT as ProjectV2Field
	FieldTypeSelect  = "ProjectV2SingleSelectField"
	LabelFeatureName = "klanky:feature"
)

// StatusOptions lists the required Status options in the order they should
// appear on the kanban (left-to-right). Klanky validates exact name match.
var StatusOptions = []string{"Todo", "In Progress", "In Review", "Needs Attention", "Done"}

// ProjectFields mirrors `gh project field-list --format json` output.
type ProjectFields struct {
	Fields []ProjectField `json:"fields"`
}

type ProjectField struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Type    string               `json:"type"`
	Options []ProjectFieldOption `json:"options,omitempty"`
}

type ProjectFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ValidateProject returns a list of human-readable error messages describing
// every way the given project's field configuration deviates from the schema.
// Empty slice means conforming.
func ValidateProject(pf ProjectFields) []string {
	var errs []string

	phase := findField(pf.Fields, FieldNamePhase)
	if phase == nil {
		errs = append(errs, fmt.Sprintf("missing required field %q (expected type %s)", FieldNamePhase, FieldTypeNumber))
	} else if phase.Type != FieldTypeNumber {
		errs = append(errs, fmt.Sprintf("field %q has type %q, want %q", FieldNamePhase, phase.Type, FieldTypeNumber))
	}

	status := findField(pf.Fields, FieldNameStatus)
	if status == nil {
		errs = append(errs, fmt.Sprintf("missing required field %q (expected type %s)", FieldNameStatus, FieldTypeSelect))
	} else {
		if status.Type != FieldTypeSelect {
			errs = append(errs, fmt.Sprintf("field %q has type %q, want %q", FieldNameStatus, status.Type, FieldTypeSelect))
		}
		present := make(map[string]bool, len(status.Options))
		for _, o := range status.Options {
			present[o.Name] = true
		}
		for _, want := range StatusOptions {
			if !present[want] {
				errs = append(errs, fmt.Sprintf("Status field is missing option %q (case-sensitive)", want))
			}
		}
	}

	return errs
}

func findField(fs []ProjectField, name string) *ProjectField {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests; confirm they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add schema.go schema_test.go
git commit -m "feat: add schema constants and ValidateProject"
```

---

## Task 6: JSON-line output helper

**Files:**
- Create: `/Users/jp/Source/klanky/output.go`
- Create: `/Users/jp/Source/klanky/output_test.go`

- [ ] **Step 1: Write failing test**

Create `output_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSONLine_WritesSingleLineJSON(t *testing.T) {
	out := &bytes.Buffer{}
	err := PrintJSONLine(out, map[string]any{
		"feature_id": 117,
		"url":        "https://example.com/issues/117",
	})
	if err != nil {
		t.Fatalf("PrintJSONLine: %v", err)
	}
	got := out.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output should end with newline, got %q", got)
	}
	// Must be a single line.
	if strings.Count(strings.TrimRight(got, "\n"), "\n") != 0 {
		t.Errorf("output must be single-line JSON: %q", got)
	}
	if !strings.Contains(got, `"feature_id":117`) {
		t.Errorf("missing feature_id: %q", got)
	}
}
```

- [ ] **Step 2: Run; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `PrintJSONLine`**

Create `output.go`:

```go
package main

import (
	"encoding/json"
	"io"
)

// PrintJSONLine writes a single-line JSON encoding of data to w, terminated
// by a newline. This is the planning-agent-facing output contract for
// `feature new` and `task add` — the agent parses this to chain calls.
func PrintJSONLine(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(data)
}
```

(Note: `json.Encoder.Encode` writes a trailing newline by default, which satisfies our newline requirement and produces single-line output for non-nested map values.)

- [ ] **Step 4: Run; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add output.go output_test.go
git commit -m "feat: add PrintJSONLine for planning-agent output contract"
```

---

## Task 7: `klanky feature new` command

**Files:**
- Create: `/Users/jp/Source/klanky/cmd_featurenew.go`
- Create: `/Users/jp/Source/klanky/cmd_featurenew_test.go`
- Modify: `/Users/jp/Source/klanky/main.go` (register command)

- [ ] **Step 1: Write failing tests**

Create `cmd_featurenew_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, ".klankyrc.json")
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
					"options": {"Todo":"t","In Progress":"ip","In Review":"ir","Needs Attention":"na","Done":"d"}}
			}
		},
		"feature_label": {"name": "klanky:feature"}
	}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFeatureNew_CallsGhIssueCreateAndProjectAdd_AndPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	fake := NewFakeRunner()
	// gh issue create returns the URL of the created issue on stdout.
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "Login overhaul",
			"--label", "klanky:feature",
			"--body", ""},
		[]byte("https://github.com/alice/proj/issues/42\n"), nil,
	)
	// gh issue view to get number and node id (called after create).
	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":42,"id":"I_xyz","url":"https://github.com/alice/proj/issues/42"}`), nil,
	)
	// gh project item-add returns the item id on stdout.
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/42",
			"--format", "json"},
		[]byte(`{"id":"PVTI_abc"}`), nil,
	)

	out := &bytes.Buffer{}
	err = RunFeatureNew(context.Background(), fake, cfg, FeatureNewOptions{
		Title: "Login overhaul",
	}, out)
	if err != nil {
		t.Fatalf("RunFeatureNew: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"feature_id":42`) {
		t.Errorf("output missing feature_id: %s", got)
	}
	if !strings.Contains(got, "https://github.com/alice/proj/issues/42") {
		t.Errorf("output missing url: %s", got)
	}
}

func TestFeatureNew_WithBodyFile_ReadsAndPasses(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir)
	cfg, _ := LoadConfig(cfgPath)
	bodyPath := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyPath, []byte("Long context goes here."), 0644); err != nil {
		t.Fatal(err)
	}

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "X",
			"--label", "klanky:feature",
			"--body", "Long context goes here."},
		[]byte("https://github.com/alice/proj/issues/43\n"), nil,
	)
	fake.Stub(
		[]string{"gh", "issue", "view", "43", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":43,"id":"I_x","url":"https://github.com/alice/proj/issues/43"}`), nil,
	)
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/43",
			"--format", "json"},
		[]byte(`{"id":"PVTI_y"}`), nil,
	)

	out := &bytes.Buffer{}
	err := RunFeatureNew(context.Background(), fake, cfg, FeatureNewOptions{
		Title:    "X",
		BodyFile: bodyPath,
	}, out)
	if err != nil {
		t.Fatalf("RunFeatureNew: %v", err)
	}
}

func TestFeatureNew_TitleRequired(t *testing.T) {
	cfg := &Config{}
	out := &bytes.Buffer{}
	err := RunFeatureNew(context.Background(), NewFakeRunner(), cfg, FeatureNewOptions{}, out)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should mention title: %v", err)
	}
}
```

- [ ] **Step 2: Run; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `RunFeatureNew` and the cobra wiring**

Create `cmd_featurenew.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type FeatureNewOptions struct {
	Title    string
	BodyFile string
}

func newFeatureCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feature",
		Short: "Manage features",
	}
	cmd.AddCommand(newFeatureNewCmd(cfgPath))
	return cmd
}

func newFeatureNewCmd(cfgPath string) *cobra.Command {
	var opts FeatureNewOptions
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new Feature issue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}
			return RunFeatureNew(cmd.Context(), RealRunner{}, cfg, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Title, "title", "", "Title of the feature (required)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Path to a markdown file for the issue body")
	return cmd
}

// RunFeatureNew creates a Feature issue, adds it to the configured project,
// and writes a single-line JSON {"feature_id": N, "url": "..."} to out.
func RunFeatureNew(ctx context.Context, r Runner, cfg *Config, opts FeatureNewOptions, out io.Writer) error {
	if opts.Title == "" {
		return fmt.Errorf("--title is required")
	}

	body := ""
	if opts.BodyFile != "" {
		data, err := os.ReadFile(opts.BodyFile)
		if err != nil {
			return fmt.Errorf("read --body-file %s: %w", opts.BodyFile, err)
		}
		body = string(data)
	}

	repoSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name

	// 1. Create the issue with the feature label. gh writes the new issue's
	//    URL to stdout; we parse the number out of it.
	createOut, err := r.Run(ctx, "gh", "issue", "create",
		"--repo", repoSlug,
		"--title", opts.Title,
		"--label", cfg.FeatureLabel.Name,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	number := lastIssueNumberFromURL(string(createOut))
	if number == 0 {
		return fmt.Errorf("could not parse issue number from gh output: %q", string(createOut))
	}

	// 2. Re-query the issue for structured fields (number, node id, canonical url).
	viewOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(number),
		"--repo", repoSlug,
		"--json", "number,id,url",
	)
	if err != nil {
		return fmt.Errorf("gh issue view: %w", err)
	}
	var issue struct {
		Number int    `json:"number"`
		ID     string `json:"id"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(viewOut, &issue); err != nil {
		return fmt.Errorf("parse issue view: %w", err)
	}

	// 3. Add the issue to the project.
	if _, err := r.Run(ctx, "gh", "project", "item-add", strconv.Itoa(cfg.Project.Number),
		"--owner", cfg.Project.OwnerLogin,
		"--url", issue.URL,
		"--format", "json",
	); err != nil {
		return fmt.Errorf("gh project item-add: %w", err)
	}

	return PrintJSONLine(out, map[string]any{
		"feature_id": issue.Number,
		"url":        issue.URL,
	})
}

// lastIssueNumberFromURL extracts the trailing /issues/<n> number from a URL.
// Returns 0 if no such pattern is found. Used by both feature new and task add.
func lastIssueNumberFromURL(s string) int {
	const marker = "/issues/"
	i := strings.LastIndex(s, marker)
	if i == -1 {
		return 0
	}
	rest := s[i+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}
```

Note: the test in step 1 stubs `gh issue create` once and `gh issue view` once. Make sure the stub argv exactly matches the implementation's argv (order of `--repo`, `--title`, `--label`, `--body` matters because FakeRunner does a positional comparison).

- [ ] **Step 4: Wire `feature` subcommand into root**

Edit `main.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultConfigPath = ".klankyrc.json"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "klanky",
		Short: "Orchestrate parallel coding agents against a GitHub-issue task graph",
		SilenceUsage: true,
	}

	cfgPath := defaultConfigPath
	if abs, err := filepath.Abs(defaultConfigPath); err == nil {
		cfgPath = abs
	}

	root.AddCommand(&cobra.Command{Use: "init", Short: "Bootstrap a new project for this repo"})
	root.AddCommand(&cobra.Command{Use: "project", Short: "Manage project linkage"})
	root.AddCommand(newFeatureCmd(cfgPath))
	root.AddCommand(&cobra.Command{Use: "task", Short: "Manage tasks"})

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS for all tests in cmd_featurenew_test.go and the previously-passing tests.

- [ ] **Step 6: Update root help test if it broke**

The root test added in Task 1 should still pass since we kept the `feature` subcommand. Verify with `go test -run TestRootCommand ./...`.

- [ ] **Step 7: Commit**

```bash
git add cmd_featurenew.go cmd_featurenew_test.go main.go
git commit -m "feat: add 'klanky feature new' command"
```

---

## Task 8: `klanky task add` command

**Files:**
- Create: `/Users/jp/Source/klanky/cmd_taskadd.go`
- Create: `/Users/jp/Source/klanky/cmd_taskadd_test.go`
- Modify: `/Users/jp/Source/klanky/main.go` (register command)

`task add` is the most complex command in this plan: it does five sequential gh calls (create issue, link as sub-issue via GraphQL, add to project, set Phase, set Status).

- [ ] **Step 1: Write failing tests**

Create `cmd_taskadd_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskAdd_FullSequence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir) // helper from cmd_featurenew_test.go
	cfg, _ := LoadConfig(cfgPath)

	specPath := filepath.Join(dir, "spec.md")
	specBody := "## Context\nWhy.\n## Acceptance criteria\n- [ ] X\n## Out of scope\nY\n"
	if err := os.WriteFile(specPath, []byte(specBody), 0644); err != nil {
		t.Fatal(err)
	}

	// We need the Feature's node ID to call addSubIssue. The runner queries
	// it via `gh issue view` before creating the task.
	fake := NewFakeRunner()

	// 1. Look up parent (feature) node ID.
	fake.Stub(
		[]string{"gh", "issue", "view", "42", "--repo", "alice/proj", "--json", "id"},
		[]byte(`{"id":"I_parent"}`), nil,
	)

	// 2. Create the task issue.
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", "alice/proj",
			"--title", "Add login form",
			"--body", specBody},
		[]byte("https://github.com/alice/proj/issues/119\n"), nil,
	)

	// 3. View the new issue to get its number, id, url.
	fake.Stub(
		[]string{"gh", "issue", "view", "119", "--repo", "alice/proj", "--json", "number,id,url"},
		[]byte(`{"number":119,"id":"I_child","url":"https://github.com/alice/proj/issues/119"}`), nil,
	)

	// 4. Link as sub-issue via GraphQL (variables sorted alphabetically).
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + addSubIssueMutation,
			"-f", "issueId=I_parent",
			"-f", "subIssueId=I_child"},
		[]byte(`{"data":{"addSubIssue":{"issue":{"number":42}}}}`), nil,
	)

	// 5. Add to project, capturing item ID.
	fake.Stub(
		[]string{"gh", "project", "item-add", "1",
			"--owner", "alice",
			"--url", "https://github.com/alice/proj/issues/119",
			"--format", "json"},
		[]byte(`{"id":"PVTI_item"}`), nil,
	)

	// 6. Set Phase = 1.
	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTF_p",
			"--project-id", "PVT_x",
			"--number", "1"},
		[]byte(`{}`), nil,
	)

	// 7. Set Status = Todo.
	fake.Stub(
		[]string{"gh", "project", "item-edit",
			"--id", "PVTI_item",
			"--field-id", "PVTSSF_s",
			"--project-id", "PVT_x",
			"--single-select-option-id", "t"},
		[]byte(`{}`), nil,
	)

	out := &bytes.Buffer{}
	err := RunTaskAdd(context.Background(), fake, cfg, TaskAddOptions{
		FeatureID: 42,
		Phase:     1,
		Title:     "Add login form",
		SpecFile:  specPath,
	}, out)
	if err != nil {
		t.Fatalf("RunTaskAdd: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"task_id":119`) {
		t.Errorf("output missing task_id: %s", got)
	}
}

func TestTaskAdd_RequiresAllFlags(t *testing.T) {
	cases := []struct {
		name string
		opts TaskAddOptions
	}{
		{"no feature", TaskAddOptions{Phase: 1, Title: "x", SpecFile: "x"}},
		{"no phase", TaskAddOptions{FeatureID: 1, Title: "x", SpecFile: "x"}},
		{"no title", TaskAddOptions{FeatureID: 1, Phase: 1, SpecFile: "x"}},
		{"no spec-file", TaskAddOptions{FeatureID: 1, Phase: 1, Title: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RunTaskAdd(context.Background(), NewFakeRunner(), &Config{}, c.opts, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
```

- [ ] **Step 2: Run; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `RunTaskAdd`**

Create `cmd_taskadd.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

type TaskAddOptions struct {
	FeatureID int
	Phase     int
	Title     string
	SpecFile  string
}

// GraphQL mutation to link a child issue as a sub-issue of a parent.
// Used both by tests (as a stub key) and by RunTaskAdd.
const addSubIssueMutation = `mutation($issueId: ID!, $subIssueId: ID!) { addSubIssue(input: {issueId: $issueId, subIssueId: $subIssueId}) { issue { number } } }`

func newTaskCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(newTaskAddCmd(cfgPath))
	return cmd
}

func newTaskAddCmd(cfgPath string) *cobra.Command {
	var opts TaskAddOptions
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new Task sub-issue under a Feature",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}
			return RunTaskAdd(cmd.Context(), RealRunner{}, cfg, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&opts.FeatureID, "feature", 0, "Parent feature issue number (required)")
	cmd.Flags().IntVar(&opts.Phase, "phase", 0, "Phase number (required, >= 1)")
	cmd.Flags().StringVar(&opts.Title, "title", "", "Title of the task (required)")
	cmd.Flags().StringVar(&opts.SpecFile, "spec-file", "", "Path to a markdown spec file (required)")
	return cmd
}

func RunTaskAdd(ctx context.Context, r Runner, cfg *Config, opts TaskAddOptions, out io.Writer) error {
	if opts.FeatureID == 0 {
		return fmt.Errorf("--feature is required")
	}
	if opts.Phase < 1 {
		return fmt.Errorf("--phase is required (>= 1)")
	}
	if opts.Title == "" {
		return fmt.Errorf("--title is required")
	}
	if opts.SpecFile == "" {
		return fmt.Errorf("--spec-file is required")
	}

	specBytes, err := os.ReadFile(opts.SpecFile)
	if err != nil {
		return fmt.Errorf("read --spec-file %s: %w", opts.SpecFile, err)
	}
	body := string(specBytes)

	repoSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name

	// 1. Look up the Feature parent's node ID.
	parentOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(opts.FeatureID),
		"--repo", repoSlug, "--json", "id")
	if err != nil {
		return fmt.Errorf("look up parent feature #%d: %w", opts.FeatureID, err)
	}
	var parent struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(parentOut, &parent); err != nil {
		return fmt.Errorf("parse parent issue view: %w", err)
	}

	// 2. Create the task issue.
	createOut, err := r.Run(ctx, "gh", "issue", "create",
		"--repo", repoSlug,
		"--title", opts.Title,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	number := lastIssueNumberFromURL(string(createOut))
	if number == 0 {
		return fmt.Errorf("could not parse issue number: %q", string(createOut))
	}

	// 3. Get task issue's node ID and URL.
	viewOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(number),
		"--repo", repoSlug, "--json", "number,id,url")
	if err != nil {
		return fmt.Errorf("gh issue view (task): %w", err)
	}
	var task struct {
		Number int    `json:"number"`
		ID     string `json:"id"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(viewOut, &task); err != nil {
		return fmt.Errorf("parse task issue view: %w", err)
	}

	// 4. Link as sub-issue via GraphQL.
	var subResult struct {
		AddSubIssue struct {
			Issue struct {
				Number int `json:"number"`
			} `json:"issue"`
		} `json:"addSubIssue"`
	}
	if err := RunGraphQL(ctx, r, addSubIssueMutation,
		map[string]any{"issueId": parent.ID, "subIssueId": task.ID},
		&subResult,
	); err != nil {
		return fmt.Errorf("link sub-issue: %w", err)
	}

	// 5. Add to project; capture item ID.
	addOut, err := r.Run(ctx, "gh", "project", "item-add", strconv.Itoa(cfg.Project.Number),
		"--owner", cfg.Project.OwnerLogin,
		"--url", task.URL,
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project item-add: %w", err)
	}
	var added struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(addOut, &added); err != nil {
		return fmt.Errorf("parse item-add output: %w", err)
	}

	// 6. Set Phase.
	if _, err := r.Run(ctx, "gh", "project", "item-edit",
		"--id", added.ID,
		"--field-id", cfg.Project.Fields.Phase.ID,
		"--project-id", cfg.Project.NodeID,
		"--number", strconv.Itoa(opts.Phase),
	); err != nil {
		return fmt.Errorf("set Phase: %w", err)
	}

	// 7. Set Status = Todo.
	todoOptionID, ok := cfg.Project.Fields.Status.Options["Todo"]
	if !ok {
		return fmt.Errorf("config missing Status option Todo; re-run `klanky project link`")
	}
	if _, err := r.Run(ctx, "gh", "project", "item-edit",
		"--id", added.ID,
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", todoOptionID,
	); err != nil {
		return fmt.Errorf("set Status: %w", err)
	}

	return PrintJSONLine(out, map[string]any{
		"task_id": task.Number,
		"url":     task.URL,
	})
}
```

- [ ] **Step 4: Wire into root**

Modify `main.go` — replace the `task` stub:

```go
root.AddCommand(newTaskCmd(cfgPath))
```

(Replace the line `root.AddCommand(&cobra.Command{Use: "task", Short: "Manage tasks"})`.)

- [ ] **Step 5: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd_taskadd.go cmd_taskadd_test.go main.go
git commit -m "feat: add 'klanky task add' command with sub-issue linking"
```

---

## Task 9: `klanky project link` command

**Files:**
- Create: `/Users/jp/Source/klanky/cmd_projectlink.go`
- Create: `/Users/jp/Source/klanky/cmd_projectlink_test.go`
- Modify: `/Users/jp/Source/klanky/main.go`

- [ ] **Step 1: Write failing tests**

Create `cmd_projectlink_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProjectURL(t *testing.T) {
	cases := []struct {
		url           string
		wantOwner     string
		wantNumber    int
		wantOwnerType string
	}{
		{"https://github.com/users/alice/projects/4", "alice", 4, "User"},
		{"https://github.com/orgs/wistia/projects/12", "wistia", 12, "Organization"},
		{"https://github.com/orgs/wistia/projects/12/views/1", "wistia", 12, "Organization"},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			owner, num, ot, err := ParseProjectURL(c.url)
			if err != nil {
				t.Fatalf("ParseProjectURL: %v", err)
			}
			if owner != c.wantOwner || num != c.wantNumber || ot != c.wantOwnerType {
				t.Errorf("got (%q, %d, %q), want (%q, %d, %q)",
					owner, num, ot, c.wantOwner, c.wantNumber, c.wantOwnerType)
			}
		})
	}
}

func TestParseProjectURL_Invalid(t *testing.T) {
	bad := []string{
		"",
		"not a url",
		"https://github.com/alice/foo/issues/1",
		"https://github.com/users/alice/projects/notanumber",
	}
	for _, u := range bad {
		if _, _, _, err := ParseProjectURL(u); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
}

func TestProjectLink_ValidatesAndWritesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()
	// gh project view --owner alice 4 --format json
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"id":"PVT_x","number":4,"url":"https://github.com/users/alice/projects/4","title":"T"}`), nil,
	)
	// gh project field-list 4 --owner alice --format json
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTF_p","name":"Phase","type":"ProjectV2Field"},
			{"id":"PVTSSF_s","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"t","name":"Todo"},{"id":"ip","name":"In Progress"},
				{"id":"ir","name":"In Review"},{"id":"na","name":"Needs Attention"},
				{"id":"d","name":"Done"}
			]}
		]}`), nil,
	)
	// gh label list --repo alice/proj --search klanky:feature --json name
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", "alice/proj", "--search", "klanky:feature", "--json", "name"},
		[]byte(`[{"name":"klanky:feature"}]`), nil,
	)

	out := &bytes.Buffer{}
	err := RunProjectLink(context.Background(), fake, ProjectLinkOptions{
		ProjectURL: "https://github.com/users/alice/projects/4",
		RepoSlug:   "alice/proj",
		ConfigPath: cfgPath,
	}, out)
	if err != nil {
		t.Fatalf("RunProjectLink: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.Project.NodeID != "PVT_x" {
		t.Errorf("NodeID not stored: got %q", cfg.Project.NodeID)
	}
	if cfg.Project.Fields.Status.Options["Done"] != "d" {
		t.Errorf("Status options not stored")
	}
	if cfg.Repo.Owner != "alice" || cfg.Repo.Name != "proj" {
		t.Errorf("Repo not stored: %+v", cfg.Repo)
	}
}

func TestProjectLink_NonConforming_PrintsDiff(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"id":"PVT_x","number":4,"url":"https://github.com/users/alice/projects/4"}`), nil,
	)
	// Status missing two options
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", "alice", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTF_p","name":"Phase","type":"ProjectV2Field"},
			{"id":"PVTSSF_s","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"t","name":"Todo"},{"id":"ip","name":"In Progress"},{"id":"d","name":"Done"}
			]}
		]}`), nil,
	)
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", "alice/proj", "--search", "klanky:feature", "--json", "name"},
		[]byte(`[{"name":"klanky:feature"}]`), nil,
	)

	out := &bytes.Buffer{}
	err := RunProjectLink(context.Background(), fake, ProjectLinkOptions{
		ProjectURL: "https://github.com/users/alice/projects/4",
		RepoSlug:   "alice/proj",
		ConfigPath: cfgPath,
	}, out)
	if err == nil {
		t.Fatal("expected error for non-conforming project")
	}
	if !strings.Contains(err.Error(), "In Review") || !strings.Contains(err.Error(), "Needs Attention") {
		t.Errorf("error should list missing options: %v", err)
	}
}
```

- [ ] **Step 2: Run; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `RunProjectLink` and `ParseProjectURL`**

Create `cmd_projectlink.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type ProjectLinkOptions struct {
	ProjectURL string
	RepoSlug   string // "owner/name" of the repo to link
	ConfigPath string
}

func newProjectCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage project linkage",
	}
	cmd.AddCommand(newProjectLinkCmd(cfgPath))
	return cmd
}

func newProjectLinkCmd(cfgPath string) *cobra.Command {
	var opts ProjectLinkOptions
	cmd := &cobra.Command{
		Use:   "link <project-url>",
		Short: "Validate and link an existing conformant Projects v2 project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ProjectURL = args[0]
			opts.ConfigPath = cfgPath
			return RunProjectLink(cmd.Context(), RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.RepoSlug, "repo", "", "owner/name of the repo (required if not in a git checkout)")
	return cmd
}

// ParseProjectURL extracts owner, project number, and owner type from a
// Projects v2 URL like:
//   https://github.com/users/alice/projects/4
//   https://github.com/orgs/wistia/projects/12
// Trailing path segments (e.g., /views/1) are ignored.
func ParseProjectURL(s string) (owner string, number int, ownerType string, err error) {
	u, perr := url.Parse(s)
	if perr != nil || u.Host == "" {
		return "", 0, "", fmt.Errorf("invalid URL: %q", s)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	// Need at least: ["users"|"orgs", "<owner>", "projects", "<n>"].
	if len(parts) < 4 || parts[2] != "projects" {
		return "", 0, "", fmt.Errorf("not a Projects v2 URL: %q", s)
	}
	switch parts[0] {
	case "users":
		ownerType = "User"
	case "orgs":
		ownerType = "Organization"
	default:
		return "", 0, "", fmt.Errorf("URL must contain /users/ or /orgs/: %q", s)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, "", fmt.Errorf("project number %q is not an integer", parts[3])
	}
	return parts[1], n, ownerType, nil
}

func RunProjectLink(ctx context.Context, r Runner, opts ProjectLinkOptions, out io.Writer) error {
	if opts.RepoSlug == "" {
		return fmt.Errorf("--repo is required (owner/name)")
	}
	repoParts := strings.SplitN(opts.RepoSlug, "/", 2)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return fmt.Errorf("--repo must be owner/name")
	}

	owner, number, ownerType, err := ParseProjectURL(opts.ProjectURL)
	if err != nil {
		return err
	}

	// 1. Project header (id, url, title).
	headerOut, err := r.Run(ctx, "gh", "project", "view", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project view: %w", err)
	}
	var header struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(headerOut, &header); err != nil {
		return fmt.Errorf("parse project view: %w", err)
	}

	// 2. Field list.
	fieldOut, err := r.Run(ctx, "gh", "project", "field-list", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project field-list: %w", err)
	}
	var pf ProjectFields
	if err := json.Unmarshal(fieldOut, &pf); err != nil {
		return fmt.Errorf("parse field-list: %w", err)
	}

	// 3. Label existence check.
	labelOut, err := r.Run(ctx, "gh", "label", "list",
		"--repo", opts.RepoSlug,
		"--search", LabelFeatureName,
		"--json", "name")
	if err != nil {
		return fmt.Errorf("gh label list: %w", err)
	}
	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(labelOut, &labels); err != nil {
		return fmt.Errorf("parse label list: %w", err)
	}

	// 4. Aggregate validation errors.
	var validationErrs []string
	validationErrs = append(validationErrs, ValidateProject(pf)...)

	labelFound := false
	for _, l := range labels {
		if l.Name == LabelFeatureName {
			labelFound = true
			break
		}
	}
	if !labelFound {
		validationErrs = append(validationErrs,
			fmt.Sprintf("repo %s missing label %q", opts.RepoSlug, LabelFeatureName))
	}

	if len(validationErrs) > 0 {
		return fmt.Errorf("project not conformant:\n  - %s", strings.Join(validationErrs, "\n  - "))
	}

	// 5. Build and write the config.
	phase := findField(pf.Fields, FieldNamePhase)
	status := findField(pf.Fields, FieldNameStatus)
	options := make(map[string]string, len(status.Options))
	for _, o := range status.Options {
		options[o.Name] = o.ID
	}

	cfg := &Config{
		SchemaVersion: SchemaVersion,
		Repo:          ConfigRepo{Owner: repoParts[0], Name: repoParts[1]},
		Project: ConfigProject{
			URL:        header.URL,
			Number:     header.Number,
			NodeID:     header.ID,
			OwnerLogin: owner,
			OwnerType:  ownerType,
			Fields: ConfigFields{
				Phase:  ConfigField{ID: phase.ID, Name: phase.Name},
				Status: ConfigStatusField{ID: status.ID, Name: status.Name, Options: options},
			},
		},
		FeatureLabel: ConfigLabel{Name: LabelFeatureName},
	}
	if err := SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\n", opts.ConfigPath)
	return nil
}
```

- [ ] **Step 4: Wire into root**

In `main.go`, replace the `project` stub:

```go
root.AddCommand(newProjectCmd(cfgPath))
```

- [ ] **Step 5: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd_projectlink.go cmd_projectlink_test.go main.go
git commit -m "feat: add 'klanky project link' command with schema validation"
```

---

## Task 10: `klanky init` command

**Files:**
- Create: `/Users/jp/Source/klanky/cmd_init.go`
- Create: `/Users/jp/Source/klanky/cmd_init_test.go`
- Modify: `/Users/jp/Source/klanky/main.go`

`init` is the most stateful command — it creates a project, then the Phase field, then updates the Status field via GraphQL (since it pre-exists with default 3 options), then creates the label, then writes config.

- [ ] **Step 1: Write failing test**

Create `cmd_init_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestInit_FullSequence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := NewFakeRunner()

	// 1. Create project.
	fake.Stub(
		[]string{"gh", "project", "create", "--owner", "@me", "--title", "Klanky", "--format", "json"},
		[]byte(`{"id":"PVT_new","number":7,"url":"https://github.com/users/joshuapeters/projects/7","owner":{"login":"joshuapeters","type":"User"}}`),
		nil,
	)
	// 2. Create Phase field.
	fake.Stub(
		[]string{"gh", "project", "field-create", "7",
			"--owner", "@me", "--name", "Phase", "--data-type", "NUMBER", "--format", "json"},
		[]byte(`{"id":"PVTF_phase","name":"Phase","type":"ProjectV2Field"}`),
		nil,
	)
	// 3. Field-list to find the default Status field's ID and existing options.
	fake.Stub(
		[]string{"gh", "project", "field-list", "7", "--owner", "@me", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTSSF_status","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"opt_todo","name":"Todo"},
				{"id":"opt_inp","name":"In Progress"},
				{"id":"opt_done","name":"Done"}
			]},
			{"id":"PVTF_phase","name":"Phase","type":"ProjectV2Field"}
		]}`), nil,
	)
	// 4. Update Status options via GraphQL (preserving existing IDs).
	//    The mutation string is built deterministically from existing IDs;
	//    we use the same builder the production code uses to construct the
	//    expected stub argv.
	expectedMutation := buildUpdateStatusOptionsMutation(map[string]string{
		"Todo":        "opt_todo",
		"In Progress": "opt_inp",
		"Done":        "opt_done",
	})
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + expectedMutation,
			"-f", "fieldId=PVTSSF_status"},
		[]byte(`{"data":{"updateProjectV2Field":{"projectV2Field":{"id":"PVTSSF_status","options":[
			{"id":"opt_todo","name":"Todo"},
			{"id":"opt_inp","name":"In Progress"},
			{"id":"new_ir","name":"In Review"},
			{"id":"new_na","name":"Needs Attention"},
			{"id":"opt_done","name":"Done"}
		]}}}}`), nil,
	)
	// 5. Create label on the repo.
	fake.Stub(
		[]string{"gh", "label", "create", "klanky:feature",
			"--repo", "joshuapeters/klanky",
			"--description", "Marks an issue as a Klanky feature (parent of task sub-issues)",
			"--color", "0E8A16"},
		[]byte(""), nil,
	)

	out := &bytes.Buffer{}
	err := RunInit(context.Background(), fake, InitOptions{
		Owner:      "@me",
		Title:      "Klanky",
		RepoSlug:   "joshuapeters/klanky",
		ConfigPath: cfgPath,
	}, out)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.Project.NodeID != "PVT_new" {
		t.Errorf("NodeID = %q, want PVT_new", cfg.Project.NodeID)
	}
	if cfg.Project.Fields.Phase.ID != "PVTF_phase" {
		t.Errorf("Phase ID = %q", cfg.Project.Fields.Phase.ID)
	}
	if cfg.Project.Fields.Status.Options["In Review"] != "new_ir" {
		t.Errorf(`Status options["In Review"] = %q, want new_ir`,
			cfg.Project.Fields.Status.Options["In Review"])
	}
}
```

- [ ] **Step 2: Run; confirm compile failure**

Run: `go test ./...`
Expected: compile error.

- [ ] **Step 3: Implement `RunInit`**

Create `cmd_init.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
)

type InitOptions struct {
	Owner       string
	Title       string
	Description string
	RepoSlug    string
	ConfigPath  string
}

// Color mapping for Status options on the kanban.
var statusColors = map[string]string{
	"Todo":            "GRAY",
	"In Progress":     "YELLOW",
	"In Review":       "BLUE",
	"Needs Attention": "RED",
	"Done":            "GREEN",
}

// buildUpdateStatusOptionsMutation returns a fully-substituted GraphQL
// mutation string that updates the Status single-select field's options
// to klanky's required 5-option set. Existing option IDs are preserved
// (passed via `existing` map) so already-assigned items don't lose their
// status; missing options are created.
//
// The options array is inlined into the mutation rather than passed as a
// variable because GraphQL variables for nested input lists are awkward
// to express via `gh api graphql -F`. Keeping the full string as the
// query is simpler and lets tests assert exact stub equality.
func buildUpdateStatusOptionsMutation(existing map[string]string) string {
	var b strings.Builder
	b.WriteString(`mutation($fieldId: ID!) { updateProjectV2Field(input: {fieldId: $fieldId, singleSelectOptions: [`)
	for i, name := range StatusOptions {
		if i > 0 {
			b.WriteString(",")
		}
		color := statusColors[name]
		if id, ok := existing[name]; ok {
			fmt.Fprintf(&b, `{id: "%s", name: "%s", color: %s, description: ""}`, id, name, color)
		} else {
			fmt.Fprintf(&b, `{name: "%s", color: %s, description: ""}`, name, color)
		}
	}
	b.WriteString(`]}) { projectV2Field { ... on ProjectV2SingleSelectField { id options { id name } } } } }`)
	return b.String()
}

func newInitCmd(cfgPath string) *cobra.Command {
	var opts InitOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new project for this repo (creates project, fields, label)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunInit(cmd.Context(), RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Owner, "owner", "@me",
		"Project owner (@me for current user, or an org login)")
	cmd.Flags().StringVar(&opts.Title, "title", "Klanky", "Project title")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Project description (optional)")
	cmd.Flags().StringVar(&opts.RepoSlug, "repo", "",
		"owner/name of the repo to link (required)")
	return cmd
}

func RunInit(ctx context.Context, r Runner, opts InitOptions, out io.Writer) error {
	if opts.RepoSlug == "" {
		return fmt.Errorf("--repo owner/name is required")
	}

	// 1. Create project.
	createOut, err := r.Run(ctx, "gh", "project", "create",
		"--owner", opts.Owner,
		"--title", opts.Title,
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project create: %w", err)
	}
	var created struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		URL    string `json:"url"`
		Owner  struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(createOut, &created); err != nil {
		return fmt.Errorf("parse project create: %w", err)
	}

	// 2. Create Phase field.
	phaseOut, err := r.Run(ctx, "gh", "project", "field-create", strconv.Itoa(created.Number),
		"--owner", opts.Owner,
		"--name", FieldNamePhase,
		"--data-type", "NUMBER",
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project field-create Phase: %w", err)
	}
	var phaseField ConfigField
	if err := json.Unmarshal(phaseOut, &phaseField); err != nil {
		return fmt.Errorf("parse Phase field-create: %w", err)
	}

	// 3. Discover the default Status field and its existing options.
	flOut, err := r.Run(ctx, "gh", "project", "field-list", strconv.Itoa(created.Number),
		"--owner", opts.Owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project field-list: %w", err)
	}
	var fl ProjectFields
	if err := json.Unmarshal(flOut, &fl); err != nil {
		return fmt.Errorf("parse field-list: %w", err)
	}
	status := findField(fl.Fields, FieldNameStatus)
	if status == nil {
		return fmt.Errorf("Projects v2 didn't create the default Status field — re-run init or contact GitHub support")
	}
	existingOptIDs := map[string]string{}
	for _, o := range status.Options {
		existingOptIDs[o.Name] = o.ID
	}

	// 4. Build the full mutation string (preserving existing option IDs) and send.
	mutation := buildUpdateStatusOptionsMutation(existingOptIDs)

	var updateResult struct {
		UpdateProjectV2Field struct {
			ProjectV2Field struct {
				ID      string `json:"id"`
				Options []ProjectFieldOption `json:"options"`
			} `json:"projectV2Field"`
		} `json:"updateProjectV2Field"`
	}
	if err := RunGraphQL(ctx, r, mutation,
		map[string]any{"fieldId": status.ID},
		&updateResult,
	); err != nil {
		return fmt.Errorf("update Status options: %w", err)
	}

	// 5. Create the label on the repo.
	if _, err := r.Run(ctx, "gh", "label", "create", LabelFeatureName,
		"--repo", opts.RepoSlug,
		"--description", "Marks an issue as a Klanky feature (parent of task sub-issues)",
		"--color", "0E8A16",
	); err != nil {
		return fmt.Errorf("gh label create: %w", err)
	}

	// 6. Build resolved options map from the GraphQL response.
	options := make(map[string]string, len(updateResult.UpdateProjectV2Field.ProjectV2Field.Options))
	for _, o := range updateResult.UpdateProjectV2Field.ProjectV2Field.Options {
		options[o.Name] = o.ID
	}

	// 7. Write config.
	repoParts := strings.SplitN(opts.RepoSlug, "/", 2)
	if len(repoParts) != 2 {
		return fmt.Errorf("--repo must be owner/name")
	}
	cfg := &Config{
		SchemaVersion: SchemaVersion,
		Repo:          ConfigRepo{Owner: repoParts[0], Name: repoParts[1]},
		Project: ConfigProject{
			URL:        created.URL,
			Number:     created.Number,
			NodeID:     created.ID,
			OwnerLogin: created.Owner.Login,
			OwnerType:  created.Owner.Type,
			Fields: ConfigFields{
				Phase:  ConfigField{ID: phaseField.ID, Name: phaseField.Name},
				Status: ConfigStatusField{ID: status.ID, Name: FieldNameStatus, Options: options},
			},
		},
		FeatureLabel: ConfigLabel{Name: LabelFeatureName},
	}
	if err := SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\nProject: %s\n", opts.ConfigPath, created.URL)
	return nil
}

// (buildUpdateStatusOptionsMutation is defined above, near statusColors.)
```

Add `"strings"` to the imports.

The test in step 1 already calls `buildUpdateStatusOptionsMutation` with the same existing-option map the production code derives, so the stub argv matches the real call exactly. No placeholder substitution at the call site.

- [ ] **Step 4: Wire into root**

In `main.go`, replace the `init` stub:

```go
root.AddCommand(newInitCmd(cfgPath))
```

- [ ] **Step 5: Run tests; confirm pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd_init.go cmd_init_test.go main.go
git commit -m "feat: add 'klanky init' command for project bootstrap"
```

---

## Task 11: Verify the binary builds and shows help cleanly

**Files:**
- Modify: `/Users/jp/Source/klanky/main.go` if needed

- [ ] **Step 1: Build the binary**

Run: `go build -o klanky .`
Expected: `klanky` binary exists in repo root.

- [ ] **Step 2: Run `--help` and inspect output**

Run: `./klanky --help`
Expected output includes the four commands `init`, `project`, `feature`, `task` with short descriptions.

- [ ] **Step 3: Verify each subcommand's help**

Run each:
- `./klanky init --help` — should show `--owner`, `--title`, `--description`, `--repo` flags.
- `./klanky project link --help` — should show usage `<project-url>` and `--repo` flag.
- `./klanky feature new --help` — should show `--title`, `--body-file` flags.
- `./klanky task add --help` — should show `--feature`, `--phase`, `--title`, `--spec-file` flags.

Expected: every flag described above appears.

- [ ] **Step 4: Run `go test -race ./...` to catch any race conditions**

Run: `go test -race ./...`
Expected: PASS with no race warnings. (The CLI shouldn't have any concurrency in plan 1, but it's good hygiene.)

- [ ] **Step 5: Run `go vet ./...` and address any warnings**

Run: `go vet ./...`
Expected: no output (no warnings).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: verify binary builds and all command help renders correctly" --allow-empty
```

(Use `--allow-empty` because steps 1-5 are pure verification — no code changes expected unless help text was wrong.)

---

## Task 12: End-to-end smoke test against real GitHub

This task is verification, not code. It ensures the unit-tested code actually works against the live `gh` CLI and Projects v2 API. We use the already-bootstrapped `joshuapeters/klanky` project and the `.klankyrc.json` already in the repo.

- [ ] **Step 1: Sanity check `gh` and config**

```bash
gh auth status
cat .klankyrc.json
```

Expected: gh is authenticated with `project` scope; `.klankyrc.json` contains the resolved IDs already (created during the bootstrap phase before this plan was written).

- [ ] **Step 2: Create a smoke-test feature**

```bash
./klanky feature new --title "[smoke] plan 1 verification"
```

Expected output: a single-line JSON like `{"feature_id":N,"url":"https://github.com/joshuapeters/klanky/issues/N"}`.

Verify on GitHub:
- The issue exists with the title `[smoke] plan 1 verification`.
- The issue has the `klanky:feature` label.
- The issue appears on the project board (https://github.com/users/joshuapeters/projects/6).

- [ ] **Step 3: Create a spec file**

```bash
mkdir -p .klanky/specs
cat > .klanky/specs/smoke-task-1.md <<'EOF'
## Context
Smoke test of `klanky task add`.

## Acceptance criteria
- [ ] Issue is created as a sub-issue of the smoke feature.
- [ ] Issue has Phase = 1 and Status = Todo on the project.

## Out of scope
Anything else.
EOF
```

- [ ] **Step 4: Add the smoke-test task**

```bash
# Substitute N below with the feature_id from step 2.
./klanky task add --feature N --phase 1 --title "[smoke] task 1" --spec-file .klanky/specs/smoke-task-1.md
```

Expected output: `{"task_id":M,"url":"https://github.com/joshuapeters/klanky/issues/M"}`.

Verify on GitHub:
- The issue exists with the title `[smoke] task 1` and the spec body.
- The issue appears as a sub-issue under the smoke feature (visible in the parent issue's sub-issue panel and via `Parent issue` field on the project).
- On the project board, this item shows `Phase = 1` and `Status = Todo`.

- [ ] **Step 5: Test `klanky project link` against the same project (idempotency check)**

```bash
# Save the existing config first so we can restore.
cp .klankyrc.json /tmp/.klankyrc.json.bak

./klanky project link "https://github.com/users/joshuapeters/projects/6" --repo joshuapeters/klanky
```

Expected: command succeeds, prints `Wrote .klankyrc.json`. Diff the result against the backup:

```bash
diff /tmp/.klankyrc.json.bak .klankyrc.json
```

Expected: no semantic diff (whitespace differences are OK).

Restore: `mv /tmp/.klankyrc.json.bak .klankyrc.json` if any spurious differences appear.

- [ ] **Step 6: Clean up smoke-test artifacts (optional)**

If you want to keep the project tidy:
- Close the smoke feature issue and the smoke task issue on GitHub.
- Delete `.klanky/specs/smoke-task-1.md`.

Or leave them as evidence that plan 1 worked end-to-end.

- [ ] **Step 7: Commit any plan-of-record artifacts**

If `.klanky/specs/` was created during smoke testing:

```bash
git add .klanky/specs/.gitkeep || true
git commit -m "chore: add .klanky/specs directory" --allow-empty
```

---

## Self-Review Checklist (run after the plan, before handoff)

**1. Spec coverage:**

| Locked design decision | Task that implements it |
|---|---|
| `Config` struct + `.klankyrc.json` shape | Task 2 |
| gh-only stack, no Octokit | Task 3 (Runner interface wraps `gh`) |
| GraphQL via `gh api graphql` | Task 4 |
| Schema validation rules | Task 5 |
| JSON-line output contract | Task 6 |
| `klanky feature new` | Task 7 |
| `klanky task add` (5-call sequence with sub-issue link) | Task 8 |
| `klanky project link` validates user-conformant projects | Task 9 |
| `klanky init` bootstraps everything via API | Task 10 |
| Cobra root + 5-command surface | Task 1, 11 |
| End-to-end verification on real GitHub | Task 12 |

Decisions intentionally NOT in this plan (deferred to plan 2 or later):
- Worktree management (`klanky run` only)
- Lock file + reconcile (`klanky run` only)
- Agent invocation (`klanky run` only)
- Status transitions during execution (`klanky run` only)

**2. Placeholder scan:** No "TODO," "TBD," "fill in details." Every code step has actual code.

**3. Type consistency:** Function names used across tasks:
- `LoadConfig`, `SaveConfig` (Task 2; consumed in Tasks 7, 8, 9, 10)
- `Runner`, `RealRunner`, `FakeRunner`, `NewFakeRunner` (Task 3; consumed everywhere)
- `RunGraphQL` (Task 4; consumed in Tasks 8, 10)
- `ValidateProject`, `ProjectFields`, `findField` (Task 5; consumed in Task 9, 10)
- `PrintJSONLine` (Task 6; consumed in Tasks 7, 8)
- `lastIssueNumberFromURL` defined in Task 7's `cmd_featurenew.go`, also used by Task 8's `cmd_taskadd.go` — both files are in `package main` so this works.
- `addSubIssueMutation`, `updateStatusOptionsMutation` constants — defined in their respective `cmd_*.go` files, package-level so accessible from tests.

All consistent.

**4. Known subtleties to flag for the executing engineer:**
- `gh issue create` writes the URL to stdout; we re-parse the issue number from it. If gh's output format ever changes, `lastIssueNumberFromURL` is the single point to update.
- `FakeRunner` does positional argv matching. When stubbing, argv order in tests must EXACTLY match the order the production code passes arguments. If a test fails with "no stub for: ...", diff the recorded `fake.Calls[N]` against the stub argv to find the mismatch.
- Task 10's `RunInit` requires the user to have the `project` gh scope. If the smoke test (Task 12) returns a 403, run `gh auth refresh -s project` before retrying.
- The smoke test (Task 12) creates real GitHub state. Use `[smoke]` prefixes in titles so they're easy to find and close after.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-04-25-klanky-bootstrap-and-creation.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration. Each subagent gets a clean context and only sees the task it's working on plus the locked design memory. Best for catching design drift early.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints for review. More context kept across tasks, fewer dispatch overhead, but this conversation is already long; subagent-driven gives a fresh start per task.

**Which approach?**
