package project

import (
	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/project/link"
)

func NewCmdProject(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage project linkage",
	}
	cmd.AddCommand(link.NewCmdLink(cfgPath))
	return cmd
}
