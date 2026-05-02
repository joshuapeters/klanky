package link

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func TestParseProjectURL(t *testing.T) {
	cases := []struct {
		in        string
		owner     string
		number    int
		ownerType string
		wantErr   bool
	}{
		{"https://github.com/users/joshuapeters/projects/4", "joshuapeters", 4, "User", false},
		{"https://github.com/orgs/wistia/projects/12/views/3", "wistia", 12, "Organization", false},
		{"https://github.com/repos/x/y/issues/1", "", 0, "", true},
		{"https://github.com/orgs/wistia/projects/abc", "", 0, "", true},
		{"not-a-url", "", 0, "", true},
	}
	for _, c := range cases {
		owner, number, ownerType, err := ParseProjectURL(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseProjectURL(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if owner != c.owner || number != c.number || ownerType != c.ownerType {
			t.Errorf("ParseProjectURL(%q) = (%q, %d, %q), want (%q, %d, %q)",
				c.in, owner, number, ownerType, c.owner, c.number, c.ownerType)
		}
	}
}

// seedConfig writes a minimal config and returns its path.
func seedConfig(t *testing.T, projects map[string]config.Project) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects:      projects,
	}
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatalf("seedConfig: %v", err)
	}
	return path
}

// stubConformantProject populates the gh runner with a happy-path snapshot of
// gh CLI calls for a project that conforms to klanky's schema.
func stubConformantProject(fake *gh.FakeRunner, owner string, number int, projectID, title string) {
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", owner, "--format", "json"},
		[]byte(`{"id":"`+projectID+`","number":4,"url":"https://github.com/users/`+owner+`/projects/4","title":"`+title+`"}`),
		nil,
	)
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", owner, "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTSSF_status","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"opt_todo","name":"Todo"},
				{"id":"opt_inp","name":"In Progress"},
				{"id":"opt_inr","name":"In Review"},
				{"id":"opt_na","name":"Needs Attention"},
				{"id":"opt_done","name":"Done"}
			]}
		]}`), nil,
	)
	_ = number // kept for future stubs that need it
}

func stubLabelMissing(fake *gh.FakeRunner, repo string) {
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", repo, "--search", "klanky:tracked", "--json", "name"},
		[]byte("[]"), nil,
	)
	fake.Stub(
		[]string{"gh", "label", "create", "klanky:tracked",
			"--repo", repo,
			"--description", "Tracked by klanky — runner reads/writes this issue.",
			"--color", "0E8A16"},
		[]byte(""), nil,
	)
}

func stubInventory(fake *gh.FakeRunner, projectID string, count int) {
	const q = `query($pid: ID!) {
  node(id: $pid) {
    ... on ProjectV2 {
      items(first: 1, query: "label:klanky:tracked is:open") { totalCount }
    }
  }
}`
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=" + q, "-f", "pid=" + projectID},
		[]byte(`{"data":{"node":{"items":{"totalCount":`+itoa(count)+`}}}}`),
		nil,
	)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestRunProjectLink_HappyPath(t *testing.T) {
	cfgPath := seedConfig(t, nil)

	fake := gh.NewFakeRunner()
	stubConformantProject(fake, "joshuapeters", 4, "PVT_abc", "Auth System")
	stubLabelMissing(fake, "joshuapeters/klanky")
	stubInventory(fake, "PVT_abc", 7)

	var out bytes.Buffer
	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		ConfigPath: cfgPath,
	}, &out)
	if err != nil {
		t.Fatalf("RunProjectLink: %v", err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	p, ok := cfg.Projects["auth-system"]
	if !ok {
		t.Fatalf("expected slug 'auth-system' in config, got %v", cfg.Projects)
	}
	if p.NodeID != "PVT_abc" {
		t.Errorf("NodeID = %q", p.NodeID)
	}
	if p.Title != "Auth System" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Fields.Status.Options["In Review"] != "opt_inr" {
		t.Errorf("Status options[In Review] = %q", p.Fields.Status.Options["In Review"])
	}
	if !strings.Contains(out.String(), "auth-system") || !strings.Contains(out.String(), "7") {
		t.Errorf("expected slug + count in output, got %q", out.String())
	}
}

func TestRunProjectLink_RejectsNonConformantProject(t *testing.T) {
	cfgPath := seedConfig(t, nil)

	fake := gh.NewFakeRunner()
	fake.Stub(
		[]string{"gh", "project", "view", "4", "--owner", "joshuapeters", "--format", "json"},
		[]byte(`{"id":"PVT_x","number":4,"url":"https://github.com/users/joshuapeters/projects/4","title":"Auth"}`),
		nil,
	)
	// Status field missing 2 options.
	fake.Stub(
		[]string{"gh", "project", "field-list", "4", "--owner", "joshuapeters", "--format", "json"},
		[]byte(`{"fields":[
			{"id":"PVTSSF","name":"Status","type":"ProjectV2SingleSelectField","options":[
				{"id":"a","name":"Todo"},{"id":"b","name":"Done"}
			]}
		]}`), nil,
	)
	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected schema-mismatch error")
	}
	for _, want := range []string{"In Review", "Needs Attention", "SCHEMA.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestRunProjectLink_IdempotentReLinkKeepsSlug(t *testing.T) {
	// Pre-populate config with the same nodeID under a custom slug that doesn't
	// match the auto-derived one. Re-link should keep the existing slug and
	// refresh the entry in place.
	cfgPath := seedConfig(t, map[string]config.Project{
		"my-custom-slug": {
			NodeID: "PVT_abc",
			Title:  "Old Title",
			URL:    "stale",
			Number: 4,
		},
	})

	fake := gh.NewFakeRunner()
	stubConformantProject(fake, "joshuapeters", 4, "PVT_abc", "Auth System")
	// Label already exists this time.
	fake.Stub(
		[]string{"gh", "label", "list", "--repo", "joshuapeters/klanky",
			"--search", "klanky:tracked", "--json", "name"},
		[]byte(`[{"name":"klanky:tracked"}]`), nil,
	)
	stubInventory(fake, "PVT_abc", 0)

	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunProjectLink: %v", err)
	}
	cfg, _ := config.LoadConfig(cfgPath)
	if _, ok := cfg.Projects["my-custom-slug"]; !ok {
		t.Fatalf("expected slug 'my-custom-slug' to be kept; got %v", cfg.Projects)
	}
	if _, ok := cfg.Projects["auth-system"]; ok {
		t.Errorf("did not expect a second slug entry for the same project")
	}
	p := cfg.Projects["my-custom-slug"]
	if p.Title != "Auth System" || p.URL == "stale" {
		t.Errorf("re-link did not refresh in place: %+v", p)
	}
}

func TestRunProjectLink_SlugCollisionAppendsSuffix(t *testing.T) {
	cfgPath := seedConfig(t, map[string]config.Project{
		"auth-system": {NodeID: "OTHER", Title: "Other Auth", Number: 99},
	})
	fake := gh.NewFakeRunner()
	stubConformantProject(fake, "joshuapeters", 4, "PVT_abc", "Auth System")
	stubLabelMissing(fake, "joshuapeters/klanky")
	stubInventory(fake, "PVT_abc", 0)

	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunProjectLink: %v", err)
	}
	cfg, _ := config.LoadConfig(cfgPath)
	if _, ok := cfg.Projects["auth-system-2"]; !ok {
		t.Errorf("expected -2 suffix on collision; got %v", cfg.Projects)
	}
}

func TestRunProjectLink_SlugOverrideRefusesIfTakenByDifferentProject(t *testing.T) {
	cfgPath := seedConfig(t, map[string]config.Project{
		"taken": {NodeID: "OTHER"},
	})
	fake := gh.NewFakeRunner()
	stubConformantProject(fake, "joshuapeters", 4, "PVT_abc", "Auth System")
	stubLabelMissing(fake, "joshuapeters/klanky")
	stubInventory(fake, "PVT_abc", 0)

	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		Slug:       "taken",
		ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected 'already in use' error, got %v", err)
	}
}

func TestRunProjectLink_SlugOverrideValidates(t *testing.T) {
	cfgPath := seedConfig(t, nil)
	fake := gh.NewFakeRunner()
	stubConformantProject(fake, "joshuapeters", 4, "PVT_abc", "Auth System")
	stubLabelMissing(fake, "joshuapeters/klanky")
	stubInventory(fake, "PVT_abc", 0)

	err := RunProjectLink(context.Background(), fake, Options{
		ProjectURL: "https://github.com/users/joshuapeters/projects/4",
		Slug:       "Bad Slug",
		ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "slug") {
		t.Errorf("expected slug validation error, got %v", err)
	}
}
