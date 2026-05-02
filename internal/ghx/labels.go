// Package ghx holds gh-CLI helpers shared across klanky commands. Anything in
// here should be a thin, project-domain wrapper around `gh` — heavy lifting
// belongs in the calling command.
package ghx

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

// EnsureTrackedLabel idempotently creates the klanky:tracked label on
// repoSlug (owner/name). The check uses `gh label list --search`, whose match
// is a substring; we re-verify the exact name client-side before deciding to
// skip creation.
func EnsureTrackedLabel(ctx context.Context, r gh.Runner, repoSlug string) error {
	listOut, err := r.Run(ctx, "gh", "label", "list",
		"--repo", repoSlug, "--search", config.LabelTracked, "--json", "name")
	if err != nil {
		return fmt.Errorf("gh label list: %w", err)
	}
	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(listOut, &labels); err != nil {
		return fmt.Errorf("parse label list: %w", err)
	}
	for _, l := range labels {
		if l.Name == config.LabelTracked {
			return nil
		}
	}
	if _, err := r.Run(ctx, "gh", "label", "create", config.LabelTracked,
		"--repo", repoSlug,
		"--description", "Tracked by klanky — runner reads/writes this issue.",
		"--color", "0E8A16",
	); err != nil {
		return fmt.Errorf("gh label create %s on %s: %w", config.LabelTracked, repoSlug, err)
	}
	return nil
}
