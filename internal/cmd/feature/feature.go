package feature

import (
	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/feature/new"
)

func NewCmdFeature(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feature",
		Short: "Manage features",
	}
	cmd.AddCommand(new.NewCmdNew(cfgPath))
	return cmd
}
