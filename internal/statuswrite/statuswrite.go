// Package statuswrite mutates a project item's Status single-select field via
// updateProjectV2ItemFieldValue. Includes simple retry semantics — Status is a
// best-effort mirror; reconcile catches any drift on the next run.
package statuswrite

import (
	"context"
	"fmt"
	"time"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

// Mutation is the locked GraphQL mutation. Exported for test stub matching.
const Mutation = `mutation($pid: ID!, $iid: ID!, $fid: ID!, $oid: String!) {
  updateProjectV2ItemFieldValue(input: {projectId: $pid, itemId: $iid, fieldId: $fid, value: {singleSelectOptionId: $oid}}) {
    projectV2Item { id }
  }
}`

// Write sets the named Status option on a project item. Retries up to 3 times
// with exponential backoff (1×, 2×, 4× of baseDelay; baseDelay=0 → 1s).
// Returns an error after exhaustion; callers log and continue.
func Write(ctx context.Context, r gh.Runner, p config.Project, itemID, statusName string, baseDelay time.Duration) error {
	optID, ok := p.Fields.Status.Options[statusName]
	if !ok {
		return fmt.Errorf("unknown Status option %q (config has %d options; re-run `klanky project link`)",
			statusName, len(p.Fields.Status.Options))
	}
	if baseDelay == 0 {
		baseDelay = time.Second
	}
	vars := map[string]any{
		"pid": p.NodeID, "iid": itemID, "fid": p.Fields.Status.ID, "oid": optID,
	}

	delay := baseDelay
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		err := gh.RunGraphQL(ctx, r, Mutation, vars, nil)
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
