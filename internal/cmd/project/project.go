// Package project assembles the `klanky project` cobra subtree.
package project

import (
	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/project/link"
	"github.com/joshuapeters/klanky/internal/cmd/project/list"
	newcmd "github.com/joshuapeters/klanky/internal/cmd/project/new"
)

// NewCmdProject returns the parent command. Subcommands attach in the order
// they're implemented; only `link` is wired in v1's first slice.
func NewCmdProject(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage linked GitHub Project v2 boards",
	}
	cmd.AddCommand(link.NewCmdLink(cfgPath))
	cmd.AddCommand(list.NewCmdList(cfgPath))
	cmd.AddCommand(newcmd.NewCmdNew(cfgPath))
	return cmd
}
