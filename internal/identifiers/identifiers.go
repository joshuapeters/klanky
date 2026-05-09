// Package identifiers derives all per-project and per-issue string identifiers
// (branch names, filesystem paths) from a fixed set of base arguments. Construct
// one Identifiers per run; pass varying values (e.g. issue number) as method
// parameters.
package identifiers

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Identifiers derives all string identifiers for a klanky project. Construct
// once and pass throughout the run.
type Identifiers struct {
	stateRoot string
	owner     string
	repo      string
	slug      string
}

// New returns an Identifiers for the given project. owner and repo are
// lowercased to match the directory layout.
func New(stateRoot, owner, repo, slug string) Identifiers {
	return Identifiers{
		stateRoot: stateRoot,
		owner:     strings.ToLower(owner),
		repo:      strings.ToLower(repo),
		slug:      slug,
	}
}

// LockPath is the per-project file lock path.
func (ids Identifiers) LockPath() string {
	return filepath.Join(ids.stateRoot, "locks", ids.owner, ids.repo, ids.slug+".lock")
}

// BranchPrefix is the head-filter prefix used to query PRs for this project.
func (ids Identifiers) BranchPrefix() string {
	return fmt.Sprintf("klanky/%s/", ids.slug)
}

// Branch is the per-issue git branch name.
func (ids Identifiers) Branch(issueNumber int) string {
	return fmt.Sprintf("klanky/%s/issue-%d", ids.slug, issueNumber)
}

// WorktreePath is the per-issue git worktree path.
func (ids Identifiers) WorktreePath(issueNumber int) string {
	return filepath.Join(ids.stateRoot, "worktrees", ids.owner, ids.repo, ids.slug,
		fmt.Sprintf("issue-%d", issueNumber))
}

// LogPath is the per-issue agent log path.
func (ids Identifiers) LogPath(issueNumber int) string {
	return filepath.Join(ids.stateRoot, "logs", ids.owner, ids.repo, ids.slug,
		fmt.Sprintf("issue-%d.log", issueNumber))
}
