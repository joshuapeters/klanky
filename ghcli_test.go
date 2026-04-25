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
