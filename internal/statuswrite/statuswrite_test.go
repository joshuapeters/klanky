package statuswrite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func proj() config.Project {
	return config.Project{
		NodeID: "PVT_p",
		Fields: config.ProjectFields{Status: config.StatusField{
			ID: "PVTSSF_s", Name: "Status",
			Options: map[string]string{"Todo": "opt_todo", "Done": "opt_done"},
		}},
	}
}

func TestWrite_Success(t *testing.T) {
	fake := gh.NewFakeRunner()
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + Mutation,
			"-f", "fid=PVTSSF_s",
			"-f", "iid=PVTI_x",
			"-f", "oid=opt_todo",
			"-f", "pid=PVT_p"},
		[]byte(`{"data":{"updateProjectV2ItemFieldValue":{"projectV2Item":{"id":"PVTI_x"}}}}`),
		nil,
	)
	if err := Write(context.Background(), fake, proj(), "PVTI_x", "Todo", time.Millisecond); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestWrite_UnknownStatus(t *testing.T) {
	err := Write(context.Background(), gh.NewFakeRunner(), proj(), "PVTI_x", "Bogus", time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "unknown Status") {
		t.Errorf("expected unknown-status error, got %v", err)
	}
}

func TestWrite_RetriesThenFails(t *testing.T) {
	fake := gh.NewFakeRunner()
	args := []string{"gh", "api", "graphql",
		"-f", "query=" + Mutation,
		"-f", "fid=PVTSSF_s",
		"-f", "iid=PVTI_x",
		"-f", "oid=opt_todo",
		"-f", "pid=PVT_p"}
	bad := errors.New("nope")
	for i := 0; i < 3; i++ {
		fake.Stub(args, nil, bad)
	}
	err := Write(context.Background(), fake, proj(), "PVTI_x", "Todo", time.Millisecond)
	if err == nil {
		t.Fatal("expected final error")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("error should mention attempts: %v", err)
	}
	if len(fake.Calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(fake.Calls))
	}
}
