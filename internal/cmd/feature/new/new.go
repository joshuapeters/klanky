package new

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cliutil"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

type Options struct {
	Title    string
	BodyFile string
}

func NewCmdNew(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new Feature issue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadConfig(cfgPath)
			if err != nil {
				return err
			}
			return RunFeatureNew(cmd.Context(), gh.RealRunner{}, cfg, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Title, "title", "", "Title of the feature (required)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Path to a markdown file for the issue body")
	return cmd
}

// RunFeatureNew creates a Feature issue, adds it to the configured project,
// and writes a single-line JSON {"feature_id": N, "url": "..."} to out.
func RunFeatureNew(ctx context.Context, r gh.Runner, cfg *config.Config, opts Options, out io.Writer) error {
	if opts.Title == "" {
		return fmt.Errorf("--title is required")
	}

	body := ""
	if opts.BodyFile != "" {
		data, err := os.ReadFile(opts.BodyFile)
		if err != nil {
			return fmt.Errorf("read --body-file %s: %w", opts.BodyFile, err)
		}
		body = string(data)
	}

	repoSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name

	createOut, err := r.Run(ctx, "gh", "issue", "create",
		"--repo", repoSlug,
		"--title", opts.Title,
		"--label", cfg.FeatureLabel.Name,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	number := gh.LastIssueNumberFromURL(string(createOut))
	if number == 0 {
		return fmt.Errorf("could not parse issue number from gh output: %q", string(createOut))
	}

	viewOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(number),
		"--repo", repoSlug,
		"--json", "number,id,url",
	)
	if err != nil {
		return fmt.Errorf("gh issue view: %w", err)
	}
	var issue struct {
		Number int    `json:"number"`
		ID     string `json:"id"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(viewOut, &issue); err != nil {
		return fmt.Errorf("parse issue view: %w", err)
	}

	if _, err := r.Run(ctx, "gh", "project", "item-add", strconv.Itoa(cfg.Project.Number),
		"--owner", cfg.Project.OwnerLogin,
		"--url", issue.URL,
		"--format", "json",
	); err != nil {
		return fmt.Errorf("gh project item-add: %w", err)
	}

	return cliutil.PrintJSONLine(out, map[string]any{
		"feature_id": issue.Number,
		"url":        issue.URL,
	})
}
