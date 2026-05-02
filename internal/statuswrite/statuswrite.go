package main

import (
	"context"
	"fmt"
	"time"
)

// WriteStatus sets the Status single-select field on a project item to the
// option named statusName. Retries up to 3 times with exponential backoff
// (1×, 2×, 4× of baseDelay). When baseDelay is 0 it defaults to 1s.
//
// Returns an error after exhausting retries; caller logs and continues
// (status writes are best-effort by design — reconcile fixes any drift).
func WriteStatus(ctx context.Context, r Runner, cfg *Config, itemID, statusName string, baseDelay time.Duration) error {
	optionID, ok := cfg.Project.Fields.Status.Options[statusName]
	if !ok {
		return fmt.Errorf("unknown Status option %q (config has %d options)",
			statusName, len(cfg.Project.Fields.Status.Options))
	}
	if baseDelay == 0 {
		baseDelay = time.Second
	}

	var lastErr error
	delay := baseDelay
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := r.Run(ctx, "gh", "project", "item-edit",
			"--id", itemID,
			"--field-id", cfg.Project.Fields.Status.ID,
			"--project-id", cfg.Project.NodeID,
			"--single-select-option-id", optionID,
		)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}
	return fmt.Errorf("set Status to %q on item %s after 3 attempts: %w", statusName, itemID, lastErr)
}
