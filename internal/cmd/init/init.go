package initcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

type Options struct {
	Owner       string
	Title       string
	Description string
	RepoSlug    string
	ConfigPath  string
}

// statusColors maps each Status option to a Projects v2 color enum.
var statusColors = map[string]string{
	"Todo":            "GRAY",
	"In Progress":     "YELLOW",
	"In Review":       "BLUE",
	"Needs Attention": "RED",
	"Done":            "GREEN",
}

// buildUpdateStatusOptionsMutation returns a fully-substituted GraphQL
// mutation string that updates the Status single-select field's options
// to klanky's required 5-option set. Existing option IDs (passed via
// `existing`) are preserved so already-assigned items keep their status;
// missing options are created.
func buildUpdateStatusOptionsMutation(existing map[string]string) string {
	var b strings.Builder
	b.WriteString(`mutation($fieldId: ID!) { updateProjectV2Field(input: {fieldId: $fieldId, singleSelectOptions: [`)
	for i, name := range config.StatusOptions {
		if i > 0 {
			b.WriteString(",")
		}
		color := statusColors[name]
		if id, ok := existing[name]; ok {
			fmt.Fprintf(&b, `{id: "%s", name: "%s", color: %s, description: ""}`, id, name, color)
		} else {
			fmt.Fprintf(&b, `{name: "%s", color: %s, description: ""}`, name, color)
		}
	}
	b.WriteString(`]}) { projectV2Field { ... on ProjectV2SingleSelectField { id options { id name } } } } }`)
	return b.String()
}

func NewCmdInit(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new project for this repo (creates project, fields, label)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunInit(cmd.Context(), gh.RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Owner, "owner", "@me",
		"Project owner (@me for current user, or an org login)")
	cmd.Flags().StringVar(&opts.Title, "title", "Klanky", "Project title")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Project description (optional)")
	cmd.Flags().StringVar(&opts.RepoSlug, "repo", "",
		"owner/name of the repo to link (required)")
	return cmd
}

func RunInit(ctx context.Context, r gh.Runner, opts Options, out io.Writer) error {
	if opts.RepoSlug == "" {
		return fmt.Errorf("--repo owner/name is required")
	}

	// 1. Create project.
	createOut, err := r.Run(ctx, "gh", "project", "create",
		"--owner", opts.Owner,
		"--title", opts.Title,
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project create: %w", err)
	}
	var created struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		URL    string `json:"url"`
		Owner  struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(createOut, &created); err != nil {
		return fmt.Errorf("parse project create: %w", err)
	}

	// 2. Create Phase field.
	phaseOut, err := r.Run(ctx, "gh", "project", "field-create", strconv.Itoa(created.Number),
		"--owner", opts.Owner,
		"--name", config.FieldNamePhase,
		"--data-type", "NUMBER",
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project field-create Phase: %w", err)
	}
	var phaseField config.ConfigField
	if err := json.Unmarshal(phaseOut, &phaseField); err != nil {
		return fmt.Errorf("parse Phase field-create: %w", err)
	}

	// 3. Discover the default Status field and its existing options.
	flOut, err := r.Run(ctx, "gh", "project", "field-list", strconv.Itoa(created.Number),
		"--owner", opts.Owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project field-list: %w", err)
	}
	var fl config.ProjectFields
	if err := json.Unmarshal(flOut, &fl); err != nil {
		return fmt.Errorf("parse field-list: %w", err)
	}
	status := config.FindField(fl.Fields, config.FieldNameStatus)
	if status == nil {
		return fmt.Errorf("Projects v2 didn't create the default Status field — re-run init or contact GitHub support")
	}
	existingOptIDs := map[string]string{}
	for _, o := range status.Options {
		existingOptIDs[o.Name] = o.ID
	}

	// 4. Build the full mutation string (preserving existing option IDs) and send.
	mutation := buildUpdateStatusOptionsMutation(existingOptIDs)

	var updateResult struct {
		UpdateProjectV2Field struct {
			ProjectV2Field struct {
				ID      string                      `json:"id"`
				Options []config.ProjectFieldOption `json:"options"`
			} `json:"projectV2Field"`
		} `json:"updateProjectV2Field"`
	}
	if err := gh.RunGraphQL(ctx, r, mutation,
		map[string]any{"fieldId": status.ID},
		&updateResult,
	); err != nil {
		return fmt.Errorf("update Status options: %w", err)
	}

	// 5. Create the label on the repo.
	if _, err := r.Run(ctx, "gh", "label", "create", config.LabelFeatureName,
		"--repo", opts.RepoSlug,
		"--description", "Marks an issue as a Klanky feature (parent of task sub-issues)",
		"--color", "0E8A16",
	); err != nil {
		return fmt.Errorf("gh label create: %w", err)
	}

	// 6. Build resolved options map from the GraphQL response.
	options := make(map[string]string, len(updateResult.UpdateProjectV2Field.ProjectV2Field.Options))
	for _, o := range updateResult.UpdateProjectV2Field.ProjectV2Field.Options {
		options[o.Name] = o.ID
	}

	// 7. Write config.
	repoParts := strings.SplitN(opts.RepoSlug, "/", 2)
	if len(repoParts) != 2 {
		return fmt.Errorf("--repo must be owner/name")
	}
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.ConfigRepo{Owner: repoParts[0], Name: repoParts[1]},
		Project: config.ConfigProject{
			URL:        created.URL,
			Number:     created.Number,
			NodeID:     created.ID,
			OwnerLogin: created.Owner.Login,
			OwnerType:  created.Owner.Type,
			Fields: config.ConfigFields{
				Phase:  config.ConfigField{ID: phaseField.ID, Name: phaseField.Name},
				Status: config.ConfigStatusField{ID: status.ID, Name: config.FieldNameStatus, Options: options},
			},
		},
		FeatureLabel: config.ConfigLabel{Name: config.LabelFeatureName},
	}
	if err := config.SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\nProject: %s\n", opts.ConfigPath, created.URL)
	return nil
}
