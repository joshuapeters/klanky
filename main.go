package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "klanky",
		Short:        "Orchestrate parallel coding agents against a GitHub-issue task graph",
		SilenceUsage: true,
	}

	root.AddCommand(&cobra.Command{Use: "init", Short: "Bootstrap a new project for this repo"})
	root.AddCommand(&cobra.Command{Use: "project", Short: "Manage project linkage"})
	root.AddCommand(&cobra.Command{Use: "feature", Short: "Manage features"})
	root.AddCommand(&cobra.Command{Use: "task", Short: "Manage tasks"})

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
