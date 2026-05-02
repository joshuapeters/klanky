// Package root assembles the top-level cobra tree for `klanky`.
package root

import (
	"fmt"

	"github.com/spf13/cobra"

	initcmd "github.com/joshuapeters/klanky/internal/cmd/init"
	"github.com/joshuapeters/klanky/internal/cmd/issue"
	"github.com/joshuapeters/klanky/internal/cmd/project"
	runcmd "github.com/joshuapeters/klanky/internal/cmd/run"
	"github.com/joshuapeters/klanky/internal/cmd/version"
)

// NewCmdRoot returns the root cobra command.
func NewCmdRoot(cfgPath, ver, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:   "klanky",
		Short: "Orchestrate parallel coding agents across GitHub Project v2 boards",
		Long: fmt.Sprintf("Orchestrate parallel coding agents across GitHub Project v2 boards.\n\n"+
			"Version: %s (%s) built %s", ver, commit, date),
		Version:       ver,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(initcmd.NewCmdInit(cfgPath))
	root.AddCommand(issue.NewCmdIssue(cfgPath))
	root.AddCommand(project.NewCmdProject(cfgPath))
	root.AddCommand(runcmd.NewCmdRun(cfgPath))
	root.AddCommand(version.NewCmdVersion(ver, commit, date))

	return root
}
