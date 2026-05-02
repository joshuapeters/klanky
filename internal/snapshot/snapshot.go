package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Snapshot is the read-only view of a feature's state at the start of a run.
// Status writes during the run mutate GitHub directly; Snapshot is not updated.
type Snapshot struct {
	Feature     FeatureInfo
	Tasks       []TaskInfo
	PRsByBranch map[string]PRInfo
}

type FeatureInfo struct {
	Number int
	Title  string
}

type TaskInfo struct {
	Number int
	Title  string
	Body   string
	State  string // "OPEN" or "CLOSED"
	NodeID string
	ItemID string // project item ID (filtered to our project node)
	Phase  *int   // nil when not set on the project item
	Status string // "Todo" / "In Progress" / "In Review" / "Needs Attention" / "Done" / "" if unset
}

// PRInfo is the subset of PR fields the runner consults. State carries the
// merge/close information ("OPEN" / "CLOSED" / "MERGED"); explicit Closed and
// Merged booleans were removed because (a) State subsumes them and (b) recent
// `gh pr list --json` doesn't expose `merged`, only `mergedAt`.
type PRInfo struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
}

const snapshotQuery = `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      number
      title
      subIssues(first: 100) {
        nodes {
          number
          title
          body
          state
          id
          projectItems(first: 5) {
            nodes {
              id
              project { id }
              fieldValues(first: 20) {
                nodes {
                  ... on ProjectV2ItemFieldNumberValue {
                    field { ... on ProjectV2Field { name } }
                    number
                  }
                  ... on ProjectV2ItemFieldSingleSelectValue {
                    field { ... on ProjectV2SingleSelectField { name } }
                    name
                    optionId
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// FetchSnapshot makes one GraphQL call for tasks+fields and one PR list call,
// then returns a populated Snapshot. The PR list filters to klanky-pattern
// branches under this feature.
func FetchSnapshot(ctx context.Context, r Runner, cfg *Config, featureID int) (*Snapshot, error) {
	var gqlResp struct {
		Repository struct {
			Issue struct {
				Number    int    `json:"number"`
				Title     string `json:"title"`
				SubIssues struct {
					Nodes []struct {
						Number       int    `json:"number"`
						Title        string `json:"title"`
						Body         string `json:"body"`
						State        string `json:"state"`
						ID           string `json:"id"`
						ProjectItems struct {
							Nodes []struct {
								ID      string `json:"id"`
								Project struct {
									ID string `json:"id"`
								} `json:"project"`
								FieldValues struct {
									Nodes []struct {
										Field struct {
											Name string `json:"name"`
										} `json:"field"`
										Number   *float64 `json:"number,omitempty"`
										Name     string   `json:"name,omitempty"`
										OptionID string   `json:"optionId,omitempty"`
									} `json:"nodes"`
								} `json:"fieldValues"`
							} `json:"nodes"`
						} `json:"projectItems"`
					} `json:"nodes"`
				} `json:"subIssues"`
			} `json:"issue"`
		} `json:"repository"`
	}

	if err := RunGraphQL(ctx, r, snapshotQuery, map[string]any{
		"owner":  cfg.Repo.Owner,
		"repo":   cfg.Repo.Name,
		"number": featureID,
	}, &gqlResp); err != nil {
		return nil, fmt.Errorf("fetch feature snapshot: %w", err)
	}

	feature := FeatureInfo{
		Number: gqlResp.Repository.Issue.Number,
		Title:  gqlResp.Repository.Issue.Title,
	}
	if feature.Number == 0 {
		return nil, fmt.Errorf("feature #%d not found in %s/%s", featureID, cfg.Repo.Owner, cfg.Repo.Name)
	}

	tasks := make([]TaskInfo, 0, len(gqlResp.Repository.Issue.SubIssues.Nodes))
	for _, n := range gqlResp.Repository.Issue.SubIssues.Nodes {
		ti := TaskInfo{
			Number: n.Number,
			Title:  n.Title,
			Body:   n.Body,
			State:  n.State,
			NodeID: n.ID,
		}
		// Find the project item belonging to OUR project (filter by node ID).
		for _, pi := range n.ProjectItems.Nodes {
			if pi.Project.ID != cfg.Project.NodeID {
				continue
			}
			ti.ItemID = pi.ID
			for _, fv := range pi.FieldValues.Nodes {
				switch fv.Field.Name {
				case FieldNamePhase:
					if fv.Number != nil {
						p := int(*fv.Number)
						ti.Phase = &p
					}
				case FieldNameStatus:
					ti.Status = fv.Name
				}
			}
			break
		}
		tasks = append(tasks, ti)
	}

	prSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name
	prSearch := fmt.Sprintf("head:klanky/feat-%d/", featureID)
	prOut, err := r.Run(ctx, "gh", "pr", "list",
		"--repo", prSlug,
		"--state", "all",
		"--search", prSearch,
		"--json", "headRefName,number,url,state",
		"--limit", "200",
	)
	if err != nil {
		return nil, fmt.Errorf("fetch PR list: %w", err)
	}
	var prs []PRInfo
	if err := json.Unmarshal(prOut, &prs); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	prsByBranch := make(map[string]PRInfo, len(prs))
	for _, pr := range prs {
		prsByBranch[pr.HeadRefName] = pr
	}

	return &Snapshot{
		Feature:     feature,
		Tasks:       tasks,
		PRsByBranch: prsByBranch,
	}, nil
}

// BranchForTask returns the branch name for a (feature, task) pair.
func BranchForTask(featureID, taskNumber int) string {
	return fmt.Sprintf("klanky/feat-%d/task-%d", featureID, taskNumber)
}

// itoa is a tiny helper so callers don't need strconv just to format ints in a single place.
func itoa(n int) string { return strconv.Itoa(n) }
