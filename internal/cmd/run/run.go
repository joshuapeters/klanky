package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

const defaultTaskTimeout = 20 * time.Minute

func newRunCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <feature-id>",
		Short: "Execute the current phase of a feature: spawn parallel agents, open PRs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			featureID, err := strconv.Atoi(args[0])
			if err != nil || featureID < 1 {
				return fmt.Errorf("feature-id must be a positive integer, got %q", args[0])
			}

			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			repoRoot, err := filepath.Abs(filepath.Dir(cfgPath))
			if err != nil {
				return fmt.Errorf("resolve repo root: %w", err)
			}

			wtRoot, err := DefaultWorktreeRoot()
			if err != nil {
				return err
			}

			progress := NewProgress(os.Stderr, time.Now)

			return RunFeature(cmd.Context(), RunFeatureDeps{
				Runner:       RealRunner{},
				Spawner:      RealSpawner{},
				Config:       cfg,
				RepoRoot:     repoRoot,
				FeatureID:    featureID,
				WorktreeRoot: wtRoot,
				Progress:     progress,
				SummaryOut:   cmd.OutOrStdout(),
				Timeout:      defaultTaskTimeout,
			})
		},
	}
	return cmd
}
