package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

type TaskAddOptions struct {
	FeatureID int
	Phase     int
	Title     string
	SpecFile  string
}

// addSubIssueMutation links a child issue as a sub-issue of a parent.
const addSubIssueMutation = `mutation($issueId: ID!, $subIssueId: ID!) { addSubIssue(input: {issueId: $issueId, subIssueId: $subIssueId}) { issue { number } } }`

func newTaskCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(newTaskAddCmd(cfgPath))
	return cmd
}

func newTaskAddCmd(cfgPath string) *cobra.Command {
	var opts TaskAddOptions
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new Task sub-issue under a Feature",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}
			return RunTaskAdd(cmd.Context(), RealRunner{}, cfg, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&opts.FeatureID, "feature", 0, "Parent feature issue number (required)")
	cmd.Flags().IntVar(&opts.Phase, "phase", 0, "Phase number (required, >= 1)")
	cmd.Flags().StringVar(&opts.Title, "title", "", "Title of the task (required)")
	cmd.Flags().StringVar(&opts.SpecFile, "spec-file", "", "Path to a markdown spec file (required)")
	return cmd
}

func RunTaskAdd(ctx context.Context, r Runner, cfg *Config, opts TaskAddOptions, out io.Writer) error {
	if opts.FeatureID == 0 {
		return fmt.Errorf("--feature is required")
	}
	if opts.Phase < 1 {
		return fmt.Errorf("--phase is required (>= 1)")
	}
	if opts.Title == "" {
		return fmt.Errorf("--title is required")
	}
	if opts.SpecFile == "" {
		return fmt.Errorf("--spec-file is required")
	}

	specBytes, err := os.ReadFile(opts.SpecFile)
	if err != nil {
		return fmt.Errorf("read --spec-file %s: %w", opts.SpecFile, err)
	}
	body := string(specBytes)

	repoSlug := cfg.Repo.Owner + "/" + cfg.Repo.Name

	// 1. Look up the Feature parent's node ID.
	parentOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(opts.FeatureID),
		"--repo", repoSlug, "--json", "id")
	if err != nil {
		return fmt.Errorf("look up parent feature #%d: %w", opts.FeatureID, err)
	}
	var parent struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(parentOut, &parent); err != nil {
		return fmt.Errorf("parse parent issue view: %w", err)
	}

	// 2. Create the task issue.
	createOut, err := r.Run(ctx, "gh", "issue", "create",
		"--repo", repoSlug,
		"--title", opts.Title,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	number := lastIssueNumberFromURL(string(createOut))
	if number == 0 {
		return fmt.Errorf("could not parse issue number: %q", string(createOut))
	}

	// 3. Get task issue's node ID and URL.
	viewOut, err := r.Run(ctx, "gh", "issue", "view", strconv.Itoa(number),
		"--repo", repoSlug, "--json", "number,id,url")
	if err != nil {
		return fmt.Errorf("gh issue view (task): %w", err)
	}
	var task struct {
		Number int    `json:"number"`
		ID     string `json:"id"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(viewOut, &task); err != nil {
		return fmt.Errorf("parse task issue view: %w", err)
	}

	// 4. Link as sub-issue via GraphQL.
	var subResult struct {
		AddSubIssue struct {
			Issue struct {
				Number int `json:"number"`
			} `json:"issue"`
		} `json:"addSubIssue"`
	}
	if err := RunGraphQL(ctx, r, addSubIssueMutation,
		map[string]any{"issueId": parent.ID, "subIssueId": task.ID},
		&subResult,
	); err != nil {
		return fmt.Errorf("link sub-issue: %w", err)
	}

	// 5. Add to project; capture item ID.
	addOut, err := r.Run(ctx, "gh", "project", "item-add", strconv.Itoa(cfg.Project.Number),
		"--owner", cfg.Project.OwnerLogin,
		"--url", task.URL,
		"--format", "json",
	)
	if err != nil {
		return fmt.Errorf("gh project item-add: %w", err)
	}
	var added struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(addOut, &added); err != nil {
		return fmt.Errorf("parse item-add output: %w", err)
	}

	// 6. Set Phase.
	if _, err := r.Run(ctx, "gh", "project", "item-edit",
		"--id", added.ID,
		"--field-id", cfg.Project.Fields.Phase.ID,
		"--project-id", cfg.Project.NodeID,
		"--number", strconv.Itoa(opts.Phase),
	); err != nil {
		return fmt.Errorf("set Phase: %w", err)
	}

	// 7. Set Status = Todo.
	todoOptionID, ok := cfg.Project.Fields.Status.Options["Todo"]
	if !ok {
		return fmt.Errorf("config missing Status option Todo; re-run `klanky project link`")
	}
	if _, err := r.Run(ctx, "gh", "project", "item-edit",
		"--id", added.ID,
		"--field-id", cfg.Project.Fields.Status.ID,
		"--project-id", cfg.Project.NodeID,
		"--single-select-option-id", todoOptionID,
	); err != nil {
		return fmt.Errorf("set Status: %w", err)
	}

	return PrintJSONLine(out, map[string]any{
		"task_id": task.Number,
		"url":     task.URL,
	})
}
