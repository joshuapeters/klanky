// Package link implements `klanky project link <project-url>`. It is the
// dominant adoption path: the user (or planning agent) creates a conformant
// GitHub Project v2 in the UI, then runs link to register it under a slug.
package link

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/ghx"
)

// Options holds the parsed flag set.
type Options struct {
	ProjectURL string
	Slug       string // override; auto-derived from title when empty
	ConfigPath string
}

// NewCmdLink returns the cobra command.
func NewCmdLink(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "link <project-url>",
		Short: "Validate and register an existing GitHub Project v2 under a klanky slug",
		Long: "Validates that the project conforms to klanky's schema (Status field with the " +
			"five required options), generates a slug (or uses --slug), creates the klanky:tracked " +
			"label on the repo if absent, and records the project's resolved IDs in .klankyrc.json. " +
			"Re-running is idempotent — it refreshes IDs without changing the slug.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ProjectURL = args[0]
			opts.ConfigPath = cfgPath
			return RunProjectLink(cmd.Context(), gh.RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Slug, "slug", "",
		"slug override (default: derived from project title)")
	return cmd
}

// ParseProjectURL splits a Projects v2 URL of the form
// https://github.com/{users|orgs}/<owner>/projects/<n>(/...)? into its parts.
func ParseProjectURL(s string) (owner string, number int, ownerType string, err error) {
	u, perr := url.Parse(s)
	if perr != nil || u.Host == "" {
		return "", 0, "", fmt.Errorf("invalid URL: %q", s)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "projects" {
		return "", 0, "", fmt.Errorf("not a Projects v2 URL: %q", s)
	}
	switch parts[0] {
	case "users":
		ownerType = "User"
	case "orgs":
		ownerType = "Organization"
	default:
		return "", 0, "", fmt.Errorf("URL must contain /users/ or /orgs/: %q", s)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, "", fmt.Errorf("project number %q is not an integer", parts[3])
	}
	return parts[1], n, ownerType, nil
}

// RunProjectLink is the testable entry point.
func RunProjectLink(ctx context.Context, r gh.Runner, opts Options, out io.Writer) error {
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	owner, number, ownerType, err := ParseProjectURL(opts.ProjectURL)
	if err != nil {
		return err
	}

	header, err := fetchProjectHeader(ctx, r, owner, number)
	if err != nil {
		return err
	}

	rawFields, err := fetchProjectFields(ctx, r, owner, number)
	if err != nil {
		return err
	}

	if errs := config.ValidateProjectSchema(rawFields); len(errs) > 0 {
		return fmt.Errorf("project does not conform to klanky's schema (see SCHEMA.md):\n  - %s",
			strings.Join(errs, "\n  - "))
	}

	if err := ghx.EnsureTrackedLabel(ctx, r, cfg.Repo.Slug()); err != nil {
		return err
	}

	inventory, err := fetchInventoryCount(ctx, r, header.ID)
	if err != nil {
		return fmt.Errorf("count tracked open issues: %w", err)
	}

	slug, err := chooseSlug(cfg, header.ID, header.Title, opts.Slug)
	if err != nil {
		return err
	}

	statusRaw := config.FindField(rawFields, config.StatusFieldName)
	options := make(map[string]string, len(statusRaw.Options))
	for _, o := range statusRaw.Options {
		options[o.Name] = o.ID
	}

	cfg.Projects[slug] = config.Project{
		URL:        header.URL,
		Number:     header.Number,
		NodeID:     header.ID,
		Title:      header.Title,
		OwnerLogin: owner,
		OwnerType:  ownerType,
		Fields: config.ProjectFields{
			Status: config.StatusField{ID: statusRaw.ID, Name: statusRaw.Name, Options: options},
		},
	}
	if err := config.SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "Linked project %q as slug %q. Inventory: %d tracked open issue(s).\n",
		header.Title, slug, inventory)
	return nil
}

// projectHeader is the subset of project metadata persisted in config.
type projectHeader struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

func fetchProjectHeader(ctx context.Context, r gh.Runner, owner string, number int) (*projectHeader, error) {
	out, err := r.Run(ctx, "gh", "project", "view", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("gh project view: %w", err)
	}
	var h projectHeader
	if err := json.Unmarshal(out, &h); err != nil {
		return nil, fmt.Errorf("parse project view: %w", err)
	}
	return &h, nil
}

func fetchProjectFields(ctx context.Context, r gh.Runner, owner string, number int) ([]config.RawField, error) {
	out, err := r.Run(ctx, "gh", "project", "field-list", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("gh project field-list: %w", err)
	}
	var pf config.ProjectFieldsRaw
	if err := json.Unmarshal(out, &pf); err != nil {
		return nil, fmt.Errorf("parse field-list: %w", err)
	}
	return pf.Fields, nil
}

// fetchInventoryCount returns the number of open, klanky:tracked items on the
// project. Used solely for the link-time message.
func fetchInventoryCount(ctx context.Context, r gh.Runner, projectNodeID string) (int, error) {
	const q = `query($pid: ID!) {
  node(id: $pid) {
    ... on ProjectV2 {
      items(first: 1, query: "label:klanky:tracked is:open") { totalCount }
    }
  }
}`
	var resp struct {
		Node struct {
			Items struct {
				TotalCount int `json:"totalCount"`
			} `json:"items"`
		} `json:"node"`
	}
	if err := gh.RunGraphQL(ctx, r, q, map[string]any{"pid": projectNodeID}, &resp); err != nil {
		return 0, err
	}
	return resp.Node.Items.TotalCount, nil
}

// chooseSlug picks the slug to use for this project. Behavior:
//   - If config already maps some slug to this nodeID, reuse it (idempotent re-link).
//   - Else if user passed --slug, validate it; refuse if it points at a different
//     project's nodeID.
//   - Else derive from title and append -2/-3/... until the slug is unused.
func chooseSlug(cfg *config.Config, nodeID, title, override string) (string, error) {
	for slug, p := range cfg.Projects {
		if p.NodeID == nodeID {
			if override != "" && override != slug {
				return "", fmt.Errorf("project already linked as slug %q; --slug %q would create a duplicate. Re-run without --slug to refresh in place, or unlink first.",
					slug, override)
			}
			return slug, nil
		}
	}
	if override != "" {
		if err := config.ValidateSlug(override); err != nil {
			return "", err
		}
		if _, taken := cfg.Projects[override]; taken {
			return "", fmt.Errorf("slug %q is already in use by another project; pick a different --slug or unlink first", override)
		}
		return override, nil
	}
	base := config.DeriveSlug(title)
	if base == "" {
		return "", fmt.Errorf("could not derive slug from title %q; pass --slug explicitly", title)
	}
	if err := config.ValidateSlug(base); err != nil {
		return "", fmt.Errorf("derived slug %q from title %q is invalid: %w", base, title, err)
	}
	if _, taken := cfg.Projects[base]; !taken {
		return base, nil
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, taken := cfg.Projects[candidate]; !taken {
			return candidate, nil
		}
	}
}
