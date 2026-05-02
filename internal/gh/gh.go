package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
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

// RunGraphQL executes a GraphQL query via `gh api graphql`, parses the response,
// and unmarshals .data into dest. Variables are emitted in sorted key order so
// test stubs can rely on deterministic argv. Numbers/bools use -F (typed),
// strings use -f.
func RunGraphQL(ctx context.Context, r Runner, query string, vars map[string]any, dest any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}

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
