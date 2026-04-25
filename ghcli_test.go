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
	// Variables are emitted in sorted key order for deterministic argv:
	// "name" < "num" alphabetically, so name comes first.
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=Q", "-f", "name=alice", "-F", "num=117"},
		[]byte(`{"data":{}}`), nil,
	)

	var dest struct{}
	err := RunGraphQL(context.Background(), fake, "Q",
		map[string]any{"num": 117, "name": "alice"}, &dest)
	if err != nil {
		t.Fatalf("RunGraphQL: %v", err)
	}
}
