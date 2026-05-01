package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the build version, commit, and date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "klanky %s\n", version); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "commit: %s\n", commit); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "built:  %s\n", date); err != nil {
				return err
			}
			return nil
		},
	}
}
