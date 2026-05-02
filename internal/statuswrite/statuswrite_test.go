package statuswrite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/config"
)

func mockConfig() *config.Config {
	return &config.Config{
		SchemaVersion: 1,
		Repo:          config.ConfigRepo{Owner: "alice", Name: "proj"},
		Project: config.ConfigProject{
			URL: "https://github.com/users/alice/projects/1", Number: 1,
			NodeID: "PVT_x", OwnerLogin: "alice", OwnerType: "User",
			Fields: config.ConfigFields{
				Phase: config.ConfigField{ID: "PVTF_p", Name: "Phase"},
				Status: config.ConfigStatusField{ID: "PVTSSF_s", Name: "Status",
					Options: map[string]string{
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e",
					}},
			},
		},
		FeatureLabel: config.ConfigLabel{Name: "klanky:feature"},
	}
}

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
	if !strings.Contains(joined, want) {
		t.Errorf("expected option ID %q in args; got: %s", want, joined)
	}
	if !strings.Contains(joined, "ITEM-X") {
		t.Errorf("expected ITEM-X in args; got: %s", joined)
	}
}

// captureRunnerFn is a one-line Runner implementation for capture tests.
type captureRunnerFn func(name string, args ...string) ([]byte, error)

func (f captureRunnerFn) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return f(name, args...)
}
