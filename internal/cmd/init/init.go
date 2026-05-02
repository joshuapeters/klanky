// Package initcmd implements `klanky init`. It writes a minimal .klankyrc.json
// for this repo and does not touch GitHub. Subsequent `klanky project link` /
// `project new` calls populate the projects map.
package initcmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

// Options is the parsed flag set for `klanky init`.
type Options struct {
	RepoSlug   string // owner/name; falls back to git remote origin if empty
	ConfigPath string
}

// NewCmdInit returns the cobra command. cfgPath is the absolute config path
// resolved by main.
func NewCmdInit(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write an empty .klankyrc.json for this repo",
		Long: "Write a minimal .klankyrc.json with no projects linked. " +
			"Does not contact GitHub. Run `klanky project link <url>` or " +
			"`klanky project new` to register projects.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunInit(cmd.Context(), gh.RealRunner{}, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.RepoSlug, "repo", "",
		"owner/name of the repo (auto-detected from `git remote get-url origin` if omitted)")
	return cmd
}

// RunInit is the testable entry point. The Runner is used only for the optional
// `git remote` lookup; when --repo is provided no commands are run.
func RunInit(ctx context.Context, r gh.Runner, opts Options, out io.Writer) error {
	if config.Exists(opts.ConfigPath) {
		return fmt.Errorf("%s already exists; delete it manually if you want to start over", opts.ConfigPath)
	}

	owner, name, err := resolveRepo(ctx, r, opts.RepoSlug)
	if err != nil {
		return err
	}

	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: owner, Name: name},
		Projects:      map[string]config.Project{},
	}
	if err := config.SaveConfig(opts.ConfigPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s for %s/%s.\n", opts.ConfigPath, owner, name)
	fmt.Fprintln(out, "Next: `klanky project link <project-url>` or `klanky project new --slug <slug> --title <title>`.")
	return nil
}

// resolveRepo returns (owner, name) from --repo if set, else by parsing
// `git remote get-url origin`. Errors when neither path yields a github.com
// owner/name pair.
func resolveRepo(ctx context.Context, r gh.Runner, slug string) (string, string, error) {
	if slug != "" {
		return parseSlug(slug)
	}
	out, err := r.Run(ctx, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("auto-detect repo: %w (pass --repo owner/name)", err)
	}
	owner, name, err := parseGitRemote(strings.TrimSpace(string(out)))
	if err != nil {
		return "", "", fmt.Errorf("parse `git remote get-url origin`: %w (pass --repo owner/name)", err)
	}
	return owner, name, nil
}

func parseSlug(s string) (string, string, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("--repo must be owner/name, got %q", s)
	}
	return parts[0], parts[1], nil
}

// parseGitRemote understands the three GitHub remote URL forms:
//
//	https://github.com/owner/repo(.git)?
//	git@github.com:owner/repo(.git)?
//	ssh://git@github.com/owner/repo(.git)?
func parseGitRemote(url string) (string, string, error) {
	const httpsPrefix = "https://github.com/"
	const sshShort = "git@github.com:"
	const sshLong = "ssh://git@github.com/"

	var rest string
	switch {
	case strings.HasPrefix(url, httpsPrefix):
		rest = strings.TrimPrefix(url, httpsPrefix)
	case strings.HasPrefix(url, sshShort):
		rest = strings.TrimPrefix(url, sshShort)
	case strings.HasPrefix(url, sshLong):
		rest = strings.TrimPrefix(url, sshLong)
	default:
		return "", "", fmt.Errorf("unrecognized github URL: %q", url)
	}
	rest = strings.TrimSuffix(rest, ".git")
	rest = strings.TrimSuffix(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/repo in %q", url)
	}
	return parts[0], parts[1], nil
}
