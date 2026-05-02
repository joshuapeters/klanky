// Package issue assembles the `klanky issue` cobra subtree.
package issue

import (
	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/issue/add"
)

func NewCmdIssue(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage klanky-tracked issues on a project",
	}
	cmd.AddCommand(add.NewCmdAdd(cfgPath))
	return cmd
}
