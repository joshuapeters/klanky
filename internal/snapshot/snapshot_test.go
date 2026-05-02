package snapshot

import (
	"context"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func cfgWith(slug, projectID string) *config.Config {
	return &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects: map[string]config.Project{
			slug: {
				NodeID: projectID, Number: 4, OwnerLogin: "joshuapeters",
				Fields: config.ProjectFields{
					Status: config.StatusField{Name: "Status"},
				},
			},
		},
	}
}

const sampleResponse = `{"data":{"node":{"items":{"nodes":[
{
  "id":"PVTI_1",
  "content":{
    "number":42,"title":"Login UI","state":"OPEN","body":"do the thing",
    "labels":{"nodes":[{"name":"klanky:tracked"}]},
    "blockedBy":{"nodes":[
      {"number":7,"state":"CLOSED","repository":{"nameWithOwner":"joshuapeters/klanky"}},
      {"number":9,"state":"OPEN","repository":{"nameWithOwner":"someone/else"}}
    ]}
  },
  "fieldValues":{"nodes":[
    {"name":"Todo","field":{"name":"Status"}}
  ]}
},
{
  "id":"PVTI_2",
  "content":{
    "number":50,"title":"Forgotten label","state":"OPEN","body":"",
    "labels":{"nodes":[]},
    "blockedBy":{"nodes":[]}
  },
  "fieldValues":{"nodes":[]}
}
]}}}}`

func TestFetch_FiltersUntrackedAndExtractsBlockers(t *testing.T) {
	cfg := cfgWith("auth", "PVT_proj")

	fake := gh.NewFakeRunner()
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + SnapshotQuery,
			"-f", "pid=PVT_proj"},
		[]byte(sampleResponse), nil,
	)
	fake.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "joshuapeters/klanky",
			"--state", "all",
			"--search", "head:klanky/auth/",
			"--json", "headRefName,number,url,state",
			"--limit", "200"},
		[]byte(`[{"number":99,"url":"https://x","state":"OPEN","headRefName":"klanky/auth/issue-42"}]`),
		nil,
	)

	snap, err := Fetch(context.Background(), fake, cfg, "auth", nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(snap.Issues) != 1 || snap.Issues[0].Number != 42 {
		t.Fatalf("expected 1 tracked issue (#42); got %d", len(snap.Issues))
	}
	got := snap.Issues[0]
	if got.Title != "Login UI" || got.Status != "Todo" || got.Body != "do the thing" {
		t.Errorf("got %+v", got)
	}
	if len(got.BlockedBy) != 2 {
		t.Fatalf("blockers = %d, want 2", len(got.BlockedBy))
	}
	if got.BlockedBy[0].Number != 7 || got.BlockedBy[0].State != "CLOSED" {
		t.Errorf("blocker0 = %+v", got.BlockedBy[0])
	}
	if got.BlockedBy[1].Repo != "someone/else" {
		t.Errorf("cross-repo blocker missing repo: %+v", got.BlockedBy[1])
	}
	pr, ok := snap.PRsByBranch[BranchForIssue("auth", 42)]
	if !ok || pr.Number != 99 {
		t.Errorf("expected PR #99 indexed by branch klanky/auth/issue-42; got %v", snap.PRsByBranch)
	}
}

func TestFetch_UnknownSlug(t *testing.T) {
	_, err := Fetch(context.Background(), gh.NewFakeRunner(), cfgWith("auth", "X"), "billing", nil)
	if err == nil || !strings.Contains(err.Error(), "no project") {
		t.Errorf("expected unknown-slug error, got %v", err)
	}
}

func TestBranchForIssue(t *testing.T) {
	if got := BranchForIssue("auth", 42); got != "klanky/auth/issue-42" {
		t.Errorf("BranchForIssue = %q", got)
	}
}
