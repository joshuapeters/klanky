package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

const defaultConfigPath = ".klankyrc.json"

// Build-time metadata. Overwritten at link time by goreleaser via -ldflags.
// Local `go build` leaves these at their default sentinel values.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "klanky",
		Short:        "Orchestrate parallel coding agents against a GitHub-issue task graph",
		Long:         fmt.Sprintf("Orchestrate parallel coding agents against a GitHub-issue task graph.\n\nVersion: %s (%s) built %s", version, commit, date),
		Version:      version,
		SilenceUsage: true,
	}

	cfgPath := defaultConfigPath
	if abs, err := filepath.Abs(defaultConfigPath); err == nil {
		cfgPath = abs
	}

	root.AddCommand(newInitCmd(cfgPath))
	root.AddCommand(newProjectCmd(cfgPath))
	root.AddCommand(newFeatureCmd(cfgPath))
	root.AddCommand(newTaskCmd(cfgPath))
	root.AddCommand(newRunCmd(cfgPath))
	root.AddCommand(newVersionCmd(version, commit, date))

	return root
}

func main() {
	// Signal-aware context so Ctrl-C cancels the runner cleanly: errgroup
	// cancellation propagates into RealSpawner.Cancel which SIGKILLs the
	// agent process group, defer-released lock file, and a non-zero exit
	// code (130 = POSIX SIGINT).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := newRootCmd().ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(ctx.Err(), context.Canceled) {
			os.Exit(130)
		}
		os.Exit(1)
	}
}
