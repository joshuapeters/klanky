// Package run wires `klanky run` to the runner orchestration package.
package run

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/runner"
)

// Options holds the parsed flag set.
type Options struct {
	ProjectSlug string
	Output      string
	Concurrency int
	Timeout     time.Duration
	ConfigPath  string
}

// NewCmdRun returns the cobra command.
func NewCmdRun(cfgPath string) *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run agents against eligible issues in a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.ConfigPath = cfgPath
			return RunRun(cmd.Context(), opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&opts.ProjectSlug, "project", "", "project slug (required)")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "output mode: text|json")
	cmd.Flags().IntVar(&opts.Concurrency, "concurrency", runner.DefaultConcurrency, "max parallel agents")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", runner.DefaultTimeout, "per-issue agent timeout")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

// RunRun is the testable entry point. It uses the real subprocess runner and
// real spawner; tests for the orchestration logic live in internal/runner.
func RunRun(ctx context.Context, opts Options, stdout, stderr interface {
	Write(p []byte) (int, error)
}) error {
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	repoRoot, err := repoRoot(ctx)
	if err != nil {
		return err
	}
	stateRoot, err := runner.DefaultStateRoot()
	if err != nil {
		return err
	}

	d := runner.Deps{
		Runner:      gh.RealRunner{},
		Spawner:     agent.RealSpawner{},
		Config:      cfg,
		ProjectSlug: opts.ProjectSlug,
		RepoRoot:    repoRoot,
		StateRoot:   stateRoot,
		Output:      opts.Output,
		Concurrency: opts.Concurrency,
		Timeout:     opts.Timeout,
		Stdout:      stdout,
		Stderr:      stderr,
	}
	return runner.Run(ctx, d)
}

// repoRoot returns `git rev-parse --show-toplevel`. We require `klanky run`
// to be invoked from inside the repo so the git worktree commands have a
// canonical anchor.
func repoRoot(ctx context.Context) (string, error) {
	out, err := gh.RealRunner{}.Run(ctx, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("locate repo root (run klanky inside the repo): %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("git rev-parse returned empty repo root")
	}
	if _, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("repo root %s missing: %w", root, err)
	}
	return root, nil
}
