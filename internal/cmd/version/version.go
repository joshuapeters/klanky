package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdVersion(v, c, d string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the build version, commit, and date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "klanky %s\n", v)
			fmt.Fprintf(out, "commit: %s\n", c)
			fmt.Fprintf(out, "built:  %s\n", d)
			return nil
		},
	}
}
