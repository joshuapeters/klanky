// Package add implements `klanky issue add`. It creates a tracked issue on the
// repo, adds it to the named project, sets Status=Todo, and (optionally) wires
// BLOCKED_BY edges to existing issues. Best-effort: failures partway through
// are reported, not rolled back.
package add

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

// Options holds the parsed flag set.
type Options struct {
	ProjectSlug string
	Title       string
	Body        string
	BodyFile    string
	DependsOn   []int
	Output      string
	ConfigPath  string
}

// NewCmdAdd returns the cobra command.
func NewCmdAdd(cfgPath string) *cobra.Command {
	var opts Options
	var dependsOnRaw string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a klanky-tracked issue and add it to a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			deps, err := parseDependsOn(dependsOnRaw)
			if err != nil {
				return err
			}
			opts.DependsOn = deps
			return RunIssueAdd(cmd.Context(), gh.RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ProjectSlug, "project", "", "project slug (required)")
	cmd.Flags().StringVar(&opts.Title, "title", "", "issue title (required)")
	cmd.Flags().StringVar(&opts.Body, "body", "", "issue body")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "read issue body from this path (use - for stdin)")
	cmd.Flags().StringVar(&dependsOnRaw, "depends-on", "",
		"comma-separated list of issue numbers in this repo that block this one")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output mode: text|json")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("title")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
	return cmd
}

// parseDependsOn turns "12, 7, 9" into []int{12, 7, 9}. Returns an error on
// any non-integer token. Empty input → empty slice.
func parseDependsOn(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.TrimPrefix(p, "#")
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("--depends-on: %q is not an integer", p)
		}
		out = append(out, n)
	}
	return out, nil
}

// Result is the value returned to the caller (used as JSON output and to drive
// the text-mode message).
type Result struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	NodeID string `json:"node_id"`
	ItemID string `json:"item_id"`
	Status string `json:"status"`
	Deps   []int  `json:"deps"`
}

// RunIssueAdd is the testable entry point.
func RunIssueAdd(ctx context.Context, r gh.Runner, opts Options, out io.Writer) error {
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	mode, err := config.ResolveOutput(cfg, opts.Output)
	if err != nil {
		return err
	}
	project, ok := cfg.Projects[opts.ProjectSlug]
	if !ok {
		return fmt.Errorf("no project with slug %q (try `klanky project list`)", opts.ProjectSlug)
	}

	body := opts.Body
	if opts.BodyFile != "" {
		body, err = readBody(opts.BodyFile)
		if err != nil {
			return err
		}
	}

	repoSlug := cfg.Repo.Slug()

	// 1. Validate deps before creating anything.
	depNodeIDs, err := validateDeps(ctx, r, repoSlug, opts.DependsOn)
	if err != nil {
		return err
	}

	// 2. Create the issue (with klanky:tracked label).
	issue, err := createIssue(ctx, r, repoSlug, opts.Title, body)
	if err != nil {
		return err
	}

	// 3. Add to project.
	itemID, err := addToProject(ctx, r, project.OwnerLogin, project.Number, issue.URL)
	if err != nil {
		return reportPartial(out, mode, issue, err, "add to project")
	}

	// 4. Set Status=Todo.
	if err := setStatusTodo(ctx, r, project, itemID); err != nil {
		return reportPartial(out, mode, issue, err, "set Status=Todo")
	}

	// 5. Wire deps.
	if err := wireBlockedBy(ctx, r, issue.NodeID, depNodeIDs); err != nil {
		return reportPartial(out, mode, issue, err, "wire dependencies")
	}

	res := Result{
		Number: issue.Number, URL: issue.URL, NodeID: issue.NodeID,
		ItemID: itemID, Status: "Todo", Deps: opts.DependsOn,
	}
	return printResult(out, mode, res)
}

// readBody reads --body-file. "-" means stdin.
func readBody(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read body from stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read body file %s: %w", path, err)
	}
	return string(data), nil
}

// issueRef is the subset of issue metadata we carry through the create flow.
type issueRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	NodeID string `json:"id"`
}

// validateDeps fetches each dep's state and node ID. Refuses if any are not
// OPEN or don't exist. Returns deps in input order (for predictable mutation
// ordering later).
func validateDeps(ctx context.Context, r gh.Runner, repoSlug string, nums []int) (map[int]string, error) {
	if len(nums) == 0 {
		return nil, nil
	}
	out := make(map[int]string, len(nums))
	var problems []string
	for _, n := range nums {
		raw, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(n),
			"--repo", repoSlug, "--json", "id,state,number")
		if err != nil {
			problems = append(problems, fmt.Sprintf("#%d: %v", n, err))
			continue
		}
		var info struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Number int    `json:"number"`
		}
		if jerr := json.Unmarshal(raw, &info); jerr != nil {
			problems = append(problems, fmt.Sprintf("#%d: parse: %v", n, jerr))
			continue
		}
		if info.State != "OPEN" {
			problems = append(problems, fmt.Sprintf("#%d is %s; only OPEN issues can be deps", n, strings.ToLower(info.State)))
			continue
		}
		out[n] = info.ID
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("dep validation failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return out, nil
}

// createIssue runs `gh issue create` with the klanky:tracked label, parses
// the resulting URL, then fetches the node ID. Two calls; could be one
// GraphQL but the gh CLI handles label resolution for us.
func createIssue(ctx context.Context, r gh.Runner, repoSlug, title, body string) (*issueRef, error) {
	args := []string{"issue", "create",
		"--repo", repoSlug,
		"--title", title,
		"--body", body,
		"--label", config.LabelTracked,
	}
	out, err := r.Run(ctx, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("gh issue create: %w", err)
	}
	url := strings.TrimSpace(string(out))
	number, err := numberFromIssueURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse issue URL %q: %w", url, err)
	}
	viewOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(number),
		"--repo", repoSlug, "--json", "id")
	if err != nil {
		return nil, fmt.Errorf("gh issue view #%d (resolve node ID): %w", number, err)
	}
	var info struct {
		ID string `json:"id"`
	}
	if jerr := json.Unmarshal(viewOut, &info); jerr != nil {
		return nil, fmt.Errorf("parse issue view: %w", jerr)
	}
	return &issueRef{Number: number, URL: url, NodeID: info.ID}, nil
}

// numberFromIssueURL returns the trailing /issues/<n> integer.
func numberFromIssueURL(url string) (int, error) {
	const marker = "/issues/"
	i := strings.LastIndex(url, marker)
	if i < 0 {
		return 0, fmt.Errorf("no /issues/<n> in URL")
	}
	rest := url[i+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("no digits after /issues/")
	}
	return strconv.Atoi(rest[:end])
}

// addToProject calls `gh project item-add` (which under the hood is the
// addProjectV2ItemById mutation). Returns the new item's node ID.
func addToProject(ctx context.Context, r gh.Runner, ownerLogin string, projectNumber int, issueURL string) (string, error) {
	out, err := r.Run(ctx, "gh", "project", "item-add", strconv.Itoa(projectNumber),
		"--owner", ownerLogin,
		"--url", issueURL,
		"--format", "json",
	)
	if err != nil {
		return "", fmt.Errorf("gh project item-add: %w", err)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse project item-add: %w", err)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("project item-add returned no item id")
	}
	return resp.ID, nil
}

// setStatusTodo writes Status=Todo via the singleSelectOptionId mutation.
func setStatusTodo(ctx context.Context, r gh.Runner, p config.Project, itemID string) error {
	optID, ok := p.Fields.Status.Options["Todo"]
	if !ok {
		return fmt.Errorf("project config has no Status option 'Todo'; re-run `klanky project link` to refresh")
	}
	const q = `mutation($pid: ID!, $iid: ID!, $fid: ID!, $oid: String!) {
  updateProjectV2ItemFieldValue(input: {projectId: $pid, itemId: $iid, fieldId: $fid, value: {singleSelectOptionId: $oid}}) {
    projectV2Item { id }
  }
}`
	vars := map[string]any{"pid": p.NodeID, "iid": itemID, "fid": p.Fields.Status.ID, "oid": optID}
	if err := gh.RunGraphQL(ctx, r, q, vars, nil); err != nil {
		return fmt.Errorf("set Status=Todo: %w", err)
	}
	return nil
}

// wireBlockedBy adds BLOCKED_BY edges from this issue to each dep, in a stable
// order (sorted dep number) so test stubs are deterministic.
func wireBlockedBy(ctx context.Context, r gh.Runner, issueID string, deps map[int]string) error {
	if len(deps) == 0 {
		return nil
	}
	nums := make([]int, 0, len(deps))
	for n := range deps {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	const q = `mutation($issueId: ID!, $blockingIssueId: ID!) {
  addBlockedBy(input: {issueId: $issueId, blockingIssueId: $blockingIssueId}) {
    issue { id }
  }
}`
	for _, n := range nums {
		vars := map[string]any{"issueId": issueID, "blockingIssueId": deps[n]}
		if err := gh.RunGraphQL(ctx, r, q, vars, nil); err != nil {
			return fmt.Errorf("addBlockedBy #%d: %w", n, err)
		}
	}
	return nil
}

// reportPartial emits a partial-success message and returns the underlying
// error so the caller exits non-zero.
func reportPartial(out io.Writer, mode string, issue *issueRef, err error, step string) error {
	if mode == config.OutputJSON {
		_ = json.NewEncoder(out).Encode(map[string]any{
			"number": issue.Number, "url": issue.URL, "node_id": issue.NodeID,
			"failed_step": step, "error": err.Error(),
		})
	} else {
		fmt.Fprintf(out, "Issue #%d created at %s, but step %q failed.\n", issue.Number, issue.URL, step)
	}
	return err
}

func printResult(out io.Writer, mode string, res Result) error {
	if mode == config.OutputJSON {
		return json.NewEncoder(out).Encode(res)
	}
	fmt.Fprintf(out, "Created issue #%d %s\n", res.Number, res.URL)
	if len(res.Deps) > 0 {
		labels := make([]string, len(res.Deps))
		for i, d := range res.Deps {
			labels[i] = fmt.Sprintf("#%d", d)
		}
		fmt.Fprintf(out, "Linked dependencies: %s\n", strings.Join(labels, ", "))
	}
	return nil
}
