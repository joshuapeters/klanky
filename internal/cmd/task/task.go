package task

import (
	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/task/add"
)

func NewCmdTask(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(add.NewCmdAdd(cfgPath))
	return cmd
}
