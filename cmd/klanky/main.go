package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joshuapeters/klanky/internal/cmd/root"
)

const defaultConfigPath = ".klankyrc.json"

// Build-time metadata. Overwritten at link time by goreleaser via -ldflags.
// Local `go build` leaves these at their default sentinel values.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Signal-aware context so Ctrl-C cancels the runner cleanly: errgroup
	// cancellation propagates into RealSpawner.Cancel which SIGKILLs the
	// agent process group, defer-released lock file, and a non-zero exit
	// code (130 = POSIX SIGINT).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath := defaultConfigPath
	if abs, err := filepath.Abs(defaultConfigPath); err == nil {
		cfgPath = abs
	}

	err := root.NewCmdRoot(cfgPath, version, commit, date).ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(ctx.Err(), context.Canceled) {
			os.Exit(130)
		}
		os.Exit(1)
	}
}
