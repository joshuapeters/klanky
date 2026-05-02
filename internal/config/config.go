// Package config holds the on-disk shape of .klankyrc.json — the per-repo,
// version-controlled bindings between klanky slugs and the GitHub Project v2
// boards they reference. Per-run ephemera (worktrees, logs, locks) live
// elsewhere; see AGENTS.md for the state-location asymmetry.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// SchemaVersion is the current on-disk config schema version. v1 is the first
// post-rewrite shape; no migration code exists because no v0 is in the wild.
const SchemaVersion = 1

// LabelTracked is the hardcoded allowlist label klanky reads/writes against.
// Issues without this label are invisible to klanky.
const LabelTracked = "klanky:tracked"

// StatusFieldName is the canonical name of the per-project Status single-select
// field. GitHub auto-creates it on every new ProjectV2.
const StatusFieldName = "Status"

// StatusOptions are the five Status values, case-sensitive, in the order they
// appear left-to-right on a kanban view. Order is not enforced on disk.
var StatusOptions = []string{"Todo", "In Progress", "In Review", "Needs Attention", "Done"}

// OutputText / OutputJSON are the valid values for --output and default_output.
const (
	OutputText = "text"
	OutputJSON = "json"
)

// ResolveOutput picks the effective output mode for one command invocation.
// Flag value wins, then config's default_output, then text. Returns an error
// for unknown values.
func ResolveOutput(c *Config, flag string) (string, error) {
	pick := flag
	if pick == "" && c != nil {
		pick = c.DefaultOutput
	}
	if pick == "" {
		pick = OutputText
	}
	switch pick {
	case OutputText, OutputJSON:
		return pick, nil
	default:
		return "", fmt.Errorf("unknown output mode %q (want %q or %q)", pick, OutputText, OutputJSON)
	}
}

// Config is the .klankyrc.json document.
type Config struct {
	SchemaVersion int                `json:"schema_version"`
	Repo          Repo               `json:"repo"`
	DefaultOutput string             `json:"default_output,omitempty"`
	Projects      map[string]Project `json:"projects"`
}

// Repo is the GitHub repository this config binds to.
type Repo struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// Slug returns "owner/name" for use with `gh --repo` and similar.
func (r Repo) Slug() string { return r.Owner + "/" + r.Name }

// Project is one linked GitHub Project v2 board, identified by klanky slug.
type Project struct {
	URL        string        `json:"url"`
	Number     int           `json:"number"`
	NodeID     string        `json:"node_id"`
	Title      string        `json:"title"`
	OwnerLogin string        `json:"owner_login"`
	OwnerType  string        `json:"owner_type"` // "User" or "Organization"
	Fields     ProjectFields `json:"fields"`
}

// ProjectFields holds the resolved field IDs for the Status field. New
// klanky-managed fields would go here too, but v1 has only one.
type ProjectFields struct {
	Status StatusField `json:"status"`
}

// StatusField is the per-project Status single-select field's resolved IDs and
// option name→optionId map. Option IDs (not names) are required by
// updateProjectV2ItemFieldValue.
type StatusField struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Options map[string]string `json:"options"`
}

// LoadConfig reads and parses .klankyrc.json. Returns ErrNoConfig (wrapped)
// when the file does not exist so callers can prompt the user to run
// `klanky init`.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found — run `klanky init` first: %w", path, err)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]Project{}
	}
	return &cfg, nil
}

// SaveConfig writes the config to path with stable two-space indentation and a
// trailing newline. Atomic-ish (write + rename) so a crashed run doesn't leave
// a half-written config.
func SaveConfig(path string, cfg *Config) error {
	if cfg.Projects == nil {
		cfg.Projects = map[string]Project{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Exists reports whether a config file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
