package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/agent"
	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
	"github.com/joshuapeters/klanky/internal/progress"
	runorch "github.com/joshuapeters/klanky/internal/run"
	"github.com/joshuapeters/klanky/internal/worktree"
)

const defaultTaskTimeout = 20 * time.Minute

func NewCmdRun(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <feature-id>",
		Short: "Execute the current phase of a feature: spawn parallel agents, open PRs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			featureID, err := strconv.Atoi(args[0])
			if err != nil || featureID < 1 {
				return fmt.Errorf("feature-id must be a positive integer, got %q", args[0])
			}

			cfg, err := config.LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			repoRoot, err := filepath.Abs(filepath.Dir(cfgPath))
			if err != nil {
				return fmt.Errorf("resolve repo root: %w", err)
			}

			wtRoot, err := worktree.DefaultWorktreeRoot()
			if err != nil {
				return err
			}

			prog := progress.NewProgress(os.Stderr, time.Now)

			return runorch.Feature(cmd.Context(), runorch.Deps{
				Runner:       gh.RealRunner{},
				Spawner:      agent.RealSpawner{},
				Config:       cfg,
				RepoRoot:     repoRoot,
				FeatureID:    featureID,
				WorktreeRoot: wtRoot,
				Progress:     prog,
				SummaryOut:   cmd.OutOrStdout(),
				Timeout:      defaultTaskTimeout,
			})
		},
	}
	return cmd
}
