package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	SchemaVersion int           `json:"schema_version"`
	Repo          ConfigRepo    `json:"repo"`
	Project       ConfigProject `json:"project"`
	FeatureLabel  ConfigLabel   `json:"feature_label"`
}

type ConfigRepo struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type ConfigProject struct {
	URL        string       `json:"url"`
	Number     int          `json:"number"`
	NodeID     string       `json:"node_id"`
	OwnerLogin string       `json:"owner_login"`
	OwnerType  string       `json:"owner_type"`
	Fields     ConfigFields `json:"fields"`
}

type ConfigFields struct {
	Phase  ConfigField       `json:"phase"`
	Status ConfigStatusField `json:"status"`
}

type ConfigField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ConfigStatusField struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Options map[string]string `json:"options"`
}

type ConfigLabel struct {
	Name string `json:"name"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .klankyrc.json at %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse .klankyrc.json at %s: %w", path, err)
	}
	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
