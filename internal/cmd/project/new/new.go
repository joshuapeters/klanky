// Package new implements `klanky project new --slug X --title Y`. It creates a
// fresh Projects v2 board (linking it to this repo) and registers it in
// .klankyrc.json under the given slug. The Status single-select field is
// auto-created by GitHub with the five required options; we only need to
// fetch the field's resolved IDs and persist the name→optionId map.
package newcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/ghx"
)

// Options holds the parsed flag set.
type Options struct {
	Slug       string
	Title      string
	Owner      string // "@me" or a user/org login; default @me
	ConfigPath string
}

// NewCmdNew returns the cobra command.
func NewCmdNew(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new GitHub Project v2 board, link it to this repo, and register a slug",
		Long: "Creates a fresh Projects v2 board with klanky's required schema, links it to " +
			"the repo from .klankyrc.json, ensures the klanky:tracked label exists, and adds " +
			"the new project to the projects map under --slug.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunProjectNew(cmd.Context(), gh.RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "klanky slug for this project (required)")
	cmd.Flags().StringVar(&opts.Title, "title", "", "project title (required)")
	cmd.Flags().StringVar(&opts.Owner, "owner", "@me",
		"project owner login (@me for current user; or a user/org login)")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

// RunProjectNew is the testable entry point.
func RunProjectNew(ctx context.Context, r gh.Runner, opts Options, out io.Writer) error {
	if err := config.ValidateSlug(opts.Slug); err != nil {
		return err
	}
	if opts.Title == "" {
		return fmt.Errorf("--title is required")
	}
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	if _, taken := cfg.Projects[opts.Slug]; taken {
		return fmt.Errorf("slug %q is already in use; pick a different --slug or unlink first", opts.Slug)
	}

	ownerID, ownerLogin, ownerType, err := resolveOwner(ctx, r, opts.Owner)
	if err != nil {
		return err
	}

	repoID, err := resolveRepoID(ctx, r, cfg.Repo.Owner, cfg.Repo.Name)
	if err != nil {
		return err
	}

	created, err := createProject(ctx, r, ownerID, opts.Title, repoID)
	if err != nil {
		return err
	}

	statusField, err := fetchStatusField(ctx, r, created.ID)
	if err != nil {
		return fmt.Errorf("fetch auto-created Status field: %w", err)
	}
	if missing := missingStatusOptions(statusField.Options); len(missing) > 0 {
		return fmt.Errorf("auto-created Status field is missing options %v — GitHub schema may have changed", missing)
	}

	if err := ghx.EnsureTrackedLabel(ctx, r, cfg.Repo.Slug()); err != nil {
		return err
	}

	options := make(map[string]string, len(statusField.Options))
	for _, o := range statusField.Options {
		options[o.Name] = o.ID
	}
	cfg.Projects[opts.Slug] = config.Project{
		URL:        created.URL,
		Number:     created.Number,
		NodeID:     created.ID,
		Title:      created.Title,
		OwnerLogin: ownerLogin,
		OwnerType:  ownerType,
		Fields: config.ProjectFields{
			Status: config.StatusField{ID: statusField.ID, Name: statusField.Name, Options: options},
		},
	}
	if err := config.SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "Created project %q (slug %q) at %s.\n", created.Title, opts.Slug, created.URL)
	return nil
}

// resolveOwner returns (ownerNodeID, ownerLogin, ownerType) for the given
// --owner flag value. "@me" or "" means the current user. Otherwise looks up
// both user and organization records and uses whichever exists.
func resolveOwner(ctx context.Context, r gh.Runner, owner string) (string, string, string, error) {
	if owner == "" || owner == "@me" {
		var resp struct {
			Viewer struct {
				ID    string `json:"id"`
				Login string `json:"login"`
			} `json:"viewer"`
		}
		if err := gh.RunGraphQL(ctx, r, `query { viewer { id login } }`, nil, &resp); err != nil {
			return "", "", "", fmt.Errorf("resolve current user: %w", err)
		}
		return resp.Viewer.ID, resp.Viewer.Login, "User", nil
	}
	const q = `query($login: String!) {
  user(login: $login) { id }
  organization(login: $login) { id }
}`
	var resp struct {
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
		Organization *struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if err := gh.RunGraphQL(ctx, r, q, map[string]any{"login": owner}, &resp); err != nil {
		return "", "", "", fmt.Errorf("resolve owner %q: %w", owner, err)
	}
	if resp.User != nil && resp.User.ID != "" {
		return resp.User.ID, owner, "User", nil
	}
	if resp.Organization != nil && resp.Organization.ID != "" {
		return resp.Organization.ID, owner, "Organization", nil
	}
	return "", "", "", fmt.Errorf("no GitHub user or organization named %q", owner)
}

func resolveRepoID(ctx context.Context, r gh.Runner, owner, name string) (string, error) {
	const q = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { id }
}`
	var resp struct {
		Repository struct {
			ID string `json:"id"`
		} `json:"repository"`
	}
	if err := gh.RunGraphQL(ctx, r, q, map[string]any{"owner": owner, "name": name}, &resp); err != nil {
		return "", fmt.Errorf("resolve repo %s/%s: %w", owner, name, err)
	}
	if resp.Repository.ID == "" {
		return "", fmt.Errorf("repository %s/%s not found or not accessible", owner, name)
	}
	return resp.Repository.ID, nil
}

// createdProject is the subset of createProjectV2 response we persist.
type createdProject struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

func createProject(ctx context.Context, r gh.Runner, ownerID, title, repoID string) (*createdProject, error) {
	const q = `mutation($ownerId: ID!, $title: String!, $repoId: ID!) {
  createProjectV2(input: {ownerId: $ownerId, title: $title, repositoryId: $repoId}) {
    projectV2 { id number url title }
  }
}`
	var resp struct {
		CreateProjectV2 struct {
			ProjectV2 createdProject `json:"projectV2"`
		} `json:"createProjectV2"`
	}
	vars := map[string]any{"ownerId": ownerID, "title": title, "repoId": repoID}
	if err := gh.RunGraphQL(ctx, r, q, vars, &resp); err != nil {
		return nil, fmt.Errorf("createProjectV2: %w", err)
	}
	if resp.CreateProjectV2.ProjectV2.ID == "" {
		return nil, fmt.Errorf("createProjectV2 returned empty project")
	}
	out := resp.CreateProjectV2.ProjectV2
	return &out, nil
}

// statusField mirrors the inline GraphQL response for the Status field.
type statusField struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Options []statusOption `json:"options"`
}

type statusOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func fetchStatusField(ctx context.Context, r gh.Runner, projectID string) (*statusField, error) {
	const q = `query($pid: ID!) {
  node(id: $pid) {
    ... on ProjectV2 {
      field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          name
          options { id name }
        }
      }
    }
  }
}`
	var resp struct {
		Node struct {
			Field statusField `json:"field"`
		} `json:"node"`
	}
	if err := gh.RunGraphQL(ctx, r, q, map[string]any{"pid": projectID}, &resp); err != nil {
		return nil, err
	}
	if resp.Node.Field.ID == "" {
		return nil, fmt.Errorf("project has no Status single-select field")
	}
	out := resp.Node.Field
	return &out, nil
}

func missingStatusOptions(opts []statusOption) []string {
	present := make(map[string]bool, len(opts))
	for _, o := range opts {
		present[o.Name] = true
	}
	var missing []string
	for _, want := range config.StatusOptions {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	return missing
}
