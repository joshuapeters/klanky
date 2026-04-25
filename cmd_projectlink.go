package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type ProjectLinkOptions struct {
	ProjectURL string
	RepoSlug   string
	ConfigPath string
}

func newProjectCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage project linkage",
	}
	cmd.AddCommand(newProjectLinkCmd(cfgPath))
	return cmd
}

func newProjectLinkCmd(cfgPath string) *cobra.Command {
	var opts ProjectLinkOptions
	cmd := &cobra.Command{
		Use:   "link <project-url>",
		Short: "Validate and link an existing conformant Projects v2 project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ProjectURL = args[0]
			opts.ConfigPath = cfgPath
			return RunProjectLink(cmd.Context(), RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.RepoSlug, "repo", "", "owner/name of the repo (required if not in a git checkout)")
	return cmd
}

// ParseProjectURL extracts owner, project number, and owner type from a
// Projects v2 URL like:
//
//	https://github.com/users/alice/projects/4
//	https://github.com/orgs/wistia/projects/12
//
// Trailing path segments (e.g., /views/1) are ignored.
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

func RunProjectLink(ctx context.Context, r Runner, opts ProjectLinkOptions, out io.Writer) error {
	if opts.RepoSlug == "" {
		return fmt.Errorf("--repo is required (owner/name)")
	}
	repoParts := strings.SplitN(opts.RepoSlug, "/", 2)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return fmt.Errorf("--repo must be owner/name")
	}

	owner, number, ownerType, err := ParseProjectURL(opts.ProjectURL)
	if err != nil {
		return err
	}

	headerOut, err := r.Run(ctx, "gh", "project", "view", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project view: %w", err)
	}
	var header struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(headerOut, &header); err != nil {
		return fmt.Errorf("parse project view: %w", err)
	}

	fieldOut, err := r.Run(ctx, "gh", "project", "field-list", strconv.Itoa(number),
		"--owner", owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("gh project field-list: %w", err)
	}
	var pf ProjectFields
	if err := json.Unmarshal(fieldOut, &pf); err != nil {
		return fmt.Errorf("parse field-list: %w", err)
	}

	labelOut, err := r.Run(ctx, "gh", "label", "list",
		"--repo", opts.RepoSlug,
		"--search", LabelFeatureName,
		"--json", "name")
	if err != nil {
		return fmt.Errorf("gh label list: %w", err)
	}
	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(labelOut, &labels); err != nil {
		return fmt.Errorf("parse label list: %w", err)
	}

	var validationErrs []string
	validationErrs = append(validationErrs, ValidateProject(pf)...)

	labelFound := false
	for _, l := range labels {
		if l.Name == LabelFeatureName {
			labelFound = true
			break
		}
	}
	if !labelFound {
		validationErrs = append(validationErrs,
			fmt.Sprintf("repo %s missing label %q", opts.RepoSlug, LabelFeatureName))
	}

	if len(validationErrs) > 0 {
		return fmt.Errorf("project not conformant:\n  - %s", strings.Join(validationErrs, "\n  - "))
	}

	phase := findField(pf.Fields, FieldNamePhase)
	status := findField(pf.Fields, FieldNameStatus)
	options := make(map[string]string, len(status.Options))
	for _, o := range status.Options {
		options[o.Name] = o.ID
	}

	cfg := &Config{
		SchemaVersion: SchemaVersion,
		Repo:          ConfigRepo{Owner: repoParts[0], Name: repoParts[1]},
		Project: ConfigProject{
			URL:        header.URL,
			Number:     header.Number,
			NodeID:     header.ID,
			OwnerLogin: owner,
			OwnerType:  ownerType,
			Fields: ConfigFields{
				Phase:  ConfigField{ID: phase.ID, Name: phase.Name},
				Status: ConfigStatusField{ID: status.ID, Name: status.Name, Options: options},
			},
		},
		FeatureLabel: ConfigLabel{Name: LabelFeatureName},
	}
	if err := SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\n", opts.ConfigPath)
	return nil
}
