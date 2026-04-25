package main

import (
	"bytes"
	"context"
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
