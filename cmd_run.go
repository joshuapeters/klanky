package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

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

			fmt.Fprintf(cmd.OutOrStdout(),
				"TODO: not implemented (would run feature #%d in repo %s/%s)\n",
				featureID, cfg.Repo.Owner, cfg.Repo.Name)
			return nil
		},
	}
	return cmd
}
