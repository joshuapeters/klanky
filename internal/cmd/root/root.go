package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joshuapeters/klanky/internal/cmd/feature"
	initcmd "github.com/joshuapeters/klanky/internal/cmd/init"
	"github.com/joshuapeters/klanky/internal/cmd/project"
	runcmd "github.com/joshuapeters/klanky/internal/cmd/run"
	"github.com/joshuapeters/klanky/internal/cmd/task"
	"github.com/joshuapeters/klanky/internal/cmd/version"
)

func NewCmdRoot(cfgPath, ver, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:          "klanky",
		Short:        "Orchestrate parallel coding agents against a GitHub-issue task graph",
		Long:         fmt.Sprintf("Orchestrate parallel coding agents against a GitHub-issue task graph.\n\nVersion: %s (%s) built %s", ver, commit, date),
		Version:      ver,
		SilenceUsage: true,
	}

	root.AddCommand(initcmd.NewCmdInit(cfgPath))
	root.AddCommand(project.NewCmdProject(cfgPath))
	root.AddCommand(feature.NewCmdFeature(cfgPath))
	root.AddCommand(task.NewCmdTask(cfgPath))
	root.AddCommand(runcmd.NewCmdRun(cfgPath))
	root.AddCommand(version.NewCmdVersion(ver, commit, date))

	return root
}
