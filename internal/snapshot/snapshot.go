// Package snapshot fetches a one-shot read of a project's tracked issues, their
// dependencies, and the open/closed PRs on klanky-named branches. The runner
// uses this snapshot for the entire pass; status writes during the run mutate
// GitHub directly and the snapshot is not refreshed.
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

// MaxIssuesPerProject is the hard cap on tracked issues in a project. Beyond
// this, the runner errors out — paginating GraphQL is deferred for v1.
const MaxIssuesPerProject = 100

// MaxBlockedByPerIssue is the hard cap on blockedBy edges per issue. Spec calls
// it out at 50 with a hard error if exceeded.
const MaxBlockedByPerIssue = 50

// Snapshot is the read-only view a single `klanky run` operates on.
type Snapshot struct {
	ProjectSlug string
	Issues      []Issue
	PRsByBranch map[string]PR
}

// Issue is one tracked issue with its dependency edges and current Status.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string // "OPEN" / "CLOSED"
	ItemID    string // ProjectV2 item ID
	Status    string // Status field option name; "" if unset
	BlockedBy []Blocker
}

// Blocker is one BLOCKED_BY edge. Repo is the predecessor's "owner/name" — may
// differ from this repo (cross-repo blockers count for eligibility).
type Blocker struct {
	Number int
	State  string // "OPEN" / "CLOSED"
	Repo   string
}

// PR is the subset of PR fields the runner consults.
type PR struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"` // OPEN/CLOSED/MERGED
	HeadRefName string `json:"headRefName"`
}

// SnapshotQuery is the locked GraphQL document. Listed verbatim in
// project_runner_design.md (with `body` added so the envelope can include it).
const SnapshotQuery = `query($pid: ID!) {
  node(id: $pid) {
    ... on ProjectV2 {
      items(first: 100, query: "label:klanky:tracked") {
        nodes {
          id
          content {
            ... on Issue {
              number title state body
              labels(first: 10) { nodes { name } }
              blockedBy(first: 50) { nodes { number state repository { nameWithOwner } } }
            }
          }
          fieldValues(first: 20) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field { ... on ProjectV2SingleSelectField { name } }
              }
            }
          }
        }
      }
    }
  }
}`

// Fetch performs the GraphQL items query and the PR list, returning a populated
// Snapshot. Issues that come back without the klanky:tracked label (a parser
// quirk in the GraphQL filter) are dropped with no error. Issues with deleted
// repository fields on a blocker are reported on stderrLog.
func Fetch(ctx context.Context, r gh.Runner, cfg *config.Config, slug string, stderrLog func(format string, a ...any)) (*Snapshot, error) {
	p, ok := cfg.Projects[slug]
	if !ok {
		return nil, fmt.Errorf("no project with slug %q in config", slug)
	}

	type rawIssue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Body   string `json:"body"`
		Labels struct {
			Nodes []struct {
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"labels"`
		BlockedBy struct {
			Nodes []struct {
				Number     int    `json:"number"`
				State      string `json:"state"`
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
			} `json:"nodes"`
		} `json:"blockedBy"`
	}
	type rawFieldValue struct {
		Name  string `json:"name"`
		Field struct {
			Name string `json:"name"`
		} `json:"field"`
	}
	var resp struct {
		Node struct {
			Items struct {
				Nodes []struct {
					ID          string   `json:"id"`
					Content     rawIssue `json:"content"`
					FieldValues struct {
						Nodes []rawFieldValue `json:"nodes"`
					} `json:"fieldValues"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"node"`
	}
	if err := gh.RunGraphQL(ctx, r, SnapshotQuery, map[string]any{"pid": p.NodeID}, &resp); err != nil {
		return nil, fmt.Errorf("fetch project items: %w", err)
	}
	if len(resp.Node.Items.Nodes) >= MaxIssuesPerProject {
		return nil, fmt.Errorf("project %q has %d+ tracked issues; v1 hard-caps at %d (paginate later)",
			slug, len(resp.Node.Items.Nodes), MaxIssuesPerProject)
	}

	issues := make([]Issue, 0, len(resp.Node.Items.Nodes))
	for _, n := range resp.Node.Items.Nodes {
		// Defensive label re-check; the GraphQL filter is opaque.
		tracked := false
		for _, l := range n.Content.Labels.Nodes {
			if l.Name == config.LabelTracked {
				tracked = true
				break
			}
		}
		if !tracked {
			continue
		}
		if len(n.Content.BlockedBy.Nodes) >= MaxBlockedByPerIssue {
			return nil, fmt.Errorf("issue #%d has %d+ blockedBy edges; v1 hard-caps at %d",
				n.Content.Number, len(n.Content.BlockedBy.Nodes), MaxBlockedByPerIssue)
		}
		blockers := make([]Blocker, 0, len(n.Content.BlockedBy.Nodes))
		for _, b := range n.Content.BlockedBy.Nodes {
			if b.Repository.NameWithOwner == "" && stderrLog != nil {
				stderrLog("warn: issue #%d references a blocker (#%d) whose repository is missing — treating as permanently blocked\n", n.Content.Number, b.Number)
			}
			blockers = append(blockers, Blocker{Number: b.Number, State: b.State, Repo: b.Repository.NameWithOwner})
		}
		status := ""
		for _, fv := range n.FieldValues.Nodes {
			if fv.Field.Name == config.StatusFieldName {
				status = fv.Name
				break
			}
		}
		issues = append(issues, Issue{
			Number: n.Content.Number, Title: n.Content.Title,
			Body: n.Content.Body, State: n.Content.State,
			ItemID: n.ID, Status: status, BlockedBy: blockers,
		})
	}

	prs, err := fetchPRs(ctx, r, cfg.Repo.Slug(), slug)
	if err != nil {
		return nil, err
	}

	return &Snapshot{ProjectSlug: slug, Issues: issues, PRsByBranch: prs}, nil
}

func fetchPRs(ctx context.Context, r gh.Runner, repoSlug, projectSlug string) (map[string]PR, error) {
	out, err := r.Run(ctx, "gh", "pr", "list",
		"--repo", repoSlug,
		"--state", "all",
		"--search", "head:klanky/"+projectSlug+"/",
		"--json", "headRefName,number,url,state",
		"--limit", "200",
	)
	if err != nil {
		return nil, fmt.Errorf("fetch PR list: %w", err)
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	idx := make(map[string]PR, len(prs))
	for _, pr := range prs {
		idx[pr.HeadRefName] = pr
	}
	return idx, nil
}

// BranchForIssue returns the klanky branch name for a (project slug, issue
// number) pair.
func BranchForIssue(projectSlug string, issueNumber int) string {
	return fmt.Sprintf("klanky/%s/issue-%d", projectSlug, issueNumber)
}
