package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultConfigPath = ".klankyrc.json"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "klanky",
		Short:        "Orchestrate parallel coding agents against a GitHub-issue task graph",
		SilenceUsage: true,
	}

	cfgPath := defaultConfigPath
	if abs, err := filepath.Abs(defaultConfigPath); err == nil {
		cfgPath = abs
	}

	root.AddCommand(&cobra.Command{Use: "init", Short: "Bootstrap a new project for this repo"})
	root.AddCommand(&cobra.Command{Use: "project", Short: "Manage project linkage"})
	root.AddCommand(newFeatureCmd(cfgPath))
	root.AddCommand(&cobra.Command{Use: "task", Short: "Manage tasks"})

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
