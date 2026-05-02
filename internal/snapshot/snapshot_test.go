package snapshot

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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

	got, err := Fetch(context.Background(), fake, cfg, "auth", nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	want := &Snapshot{
		ProjectSlug: "auth",
		Issues: []Issue{{
			Number: 42, Title: "Login UI", Body: "do the thing",
			State: "OPEN", ItemID: "PVTI_1", Status: "Todo",
			BlockedBy: []Blocker{
				{Number: 7, State: "CLOSED", Repo: "joshuapeters/klanky"},
				{Number: 9, State: "OPEN", Repo: "someone/else"},
			},
		}},
		PRsByBranch: map[string]PR{
			BranchForIssue("auth", 42): {
				Number: 99, URL: "https://x", State: "OPEN",
				HeadRefName: "klanky/auth/issue-42",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("snapshot mismatch (-want +got):\n%s", diff)
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
