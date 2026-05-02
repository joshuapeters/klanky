package initcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func TestRunInit_WithRepoFlag_WritesEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	var out bytes.Buffer
	err := RunInit(context.Background(), gh.NewFakeRunner(), Options{
		RepoSlug:   "joshuapeters/klanky",
		ConfigPath: cfgPath,
	}, &out)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SchemaVersion != config.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, config.SchemaVersion)
	}
	if cfg.Repo.Owner != "joshuapeters" || cfg.Repo.Name != "klanky" {
		t.Errorf("Repo = %+v", cfg.Repo)
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("Projects = %v, want empty", cfg.Projects)
	}
	if !strings.Contains(out.String(), "Wrote ") {
		t.Errorf("expected confirmation line in stdout; got %q", out.String())
	}
}

func TestRunInit_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := RunInit(context.Background(), gh.NewFakeRunner(), Options{
		RepoSlug: "o/r", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want error when config already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error %q should mention already exists", err)
	}
}

func TestRunInit_BadRepoSlug(t *testing.T) {
	err := RunInit(context.Background(), gh.NewFakeRunner(), Options{
		RepoSlug: "no-slash", ConfigPath: filepath.Join(t.TempDir(), ".klankyrc.json"),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Errorf("want owner/name error, got %v", err)
	}
}

func TestRunInit_AutoDetectFromGitRemote(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")

	fake := gh.NewFakeRunner()
	fake.Stub(
		[]string{"git", "remote", "get-url", "origin"},
		[]byte("https://github.com/joshuapeters/klanky.git\n"),
		nil,
	)
	err := RunInit(context.Background(), fake, Options{ConfigPath: cfgPath}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	cfg, _ := config.LoadConfig(cfgPath)
	if cfg.Repo.Owner != "joshuapeters" || cfg.Repo.Name != "klanky" {
		t.Errorf("Repo = %+v", cfg.Repo)
	}
}

func TestRunInit_AutoDetectFails(t *testing.T) {
	fake := gh.NewFakeRunner()
	fake.Stub(
		[]string{"git", "remote", "get-url", "origin"},
		nil,
		errors.New("fatal: No such remote 'origin'"),
	)
	err := RunInit(context.Background(), fake, Options{
		ConfigPath: filepath.Join(t.TempDir(), ".klankyrc.json"),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf("want --repo guidance in error, got %v", err)
	}
}

func TestParseGitRemote(t *testing.T) {
	cases := map[string][2]string{
		"https://github.com/joshuapeters/klanky.git":  {"joshuapeters", "klanky"},
		"https://github.com/joshuapeters/klanky":      {"joshuapeters", "klanky"},
		"git@github.com:joshuapeters/klanky.git":      {"joshuapeters", "klanky"},
		"git@github.com:joshuapeters/klanky":          {"joshuapeters", "klanky"},
		"ssh://git@github.com/joshuapeters/klanky.git": {"joshuapeters", "klanky"},
	}
	for in, want := range cases {
		o, n, err := parseGitRemote(in)
		if err != nil {
			t.Errorf("parseGitRemote(%q) error: %v", in, err)
			continue
		}
		if o != want[0] || n != want[1] {
			t.Errorf("parseGitRemote(%q) = (%q, %q), want %v", in, o, n, want)
		}
	}
}

func TestParseGitRemote_Unknown(t *testing.T) {
	if _, _, err := parseGitRemote("https://gitlab.com/x/y.git"); err == nil {
		t.Error("expected error for non-github URL")
	}
}
