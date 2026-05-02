package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func seedConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects:      map[string]config.Project{},
	}
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return path
}

const (
	viewerQuery = `query { viewer { id login } }`
	repoQuery   = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { id }
}`
	createMutation = `mutation($ownerId: ID!, $title: String!, $repoId: ID!) {
  createProjectV2(input: {ownerId: $ownerId, title: $title, repositoryId: $repoId}) {
    projectV2 { id number url title }
  }
}`
	statusFieldQuery = `query($pid: ID!) {
  node(id: $pid) {
    ... on ProjectV2 {
      field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          name
          options { id name }
        }
      }
    }
  }
}`
)

func stubViewer(fake *gh.FakeRunner, login, id string) {
	fake.Stub(
		[]string{"gh", "api", "graphql", "-f", "query=" + viewerQuery},
		[]byte(`{"data":{"viewer":{"id":"`+id+`","login":"`+login+`"}}}`),
		nil,
	)
}

func stubRepoID(fake *gh.FakeRunner, owner, name, id string) {
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + repoQuery,
			"-f", "name=" + name,
			"-f", "owner=" + owner},
		[]byte(`{"data":{"repository":{"id":"`+id+`"}}}`),
		nil,
	)
}

func stubCreate(fake *gh.FakeRunner, ownerID, title, repoID, projectID string, number int, url string) {
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + createMutation,
			"-f", "ownerId=" + ownerID,
			"-f", "repoId=" + repoID,
			"-f", "title=" + title},
		[]byte(`{"data":{"createProjectV2":{"projectV2":{"id":"`+projectID+`","number":`+itoa(number)+`,"url":"`+url+`","title":"`+title+`"}}}}`),
		nil,
	)
}

func stubStatusField(fake *gh.FakeRunner, projectID string) {
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + statusFieldQuery,
			"-f", "pid=" + projectID},
		[]byte(`{"data":{"node":{"field":{
			"id":"PVTSSF_status",
			"name":"Status",
			"options":[
				{"id":"opt_todo","name":"Todo"},
				{"id":"opt_inp","name":"In Progress"},
				{"id":"opt_inr","name":"In Review"},
				{"id":"opt_na","name":"Needs Attention"},
				{"id":"opt_done","name":"Done"}
			]
		}}}}`),
		nil,
	)
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

func TestRunProjectNew_HappyPath_AtMe(t *testing.T) {
	cfgPath := seedConfig(t)

	fake := gh.NewFakeRunner()
	stubViewer(fake, "joshuapeters", "U_owner")
	stubRepoID(fake, "joshuapeters", "klanky", "R_repo")
	stubCreate(fake, "U_owner", "Auth System", "R_repo",
		"PVT_new", 12, "https://github.com/users/joshuapeters/projects/12")
	stubStatusField(fake, "PVT_new")
	stubLabelMissing(fake, "joshuapeters/klanky")

	var out bytes.Buffer
	err := RunProjectNew(context.Background(), fake, Options{
		Slug: "auth", Title: "Auth System", Owner: "@me", ConfigPath: cfgPath,
	}, &out)
	if err != nil {
		t.Fatalf("RunProjectNew: %v", err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	p, ok := cfg.Projects["auth"]
	if !ok {
		t.Fatalf("expected slug 'auth' in config, got %v", cfg.Projects)
	}
	if p.NodeID != "PVT_new" || p.Number != 12 {
		t.Errorf("project = %+v", p)
	}
	if p.OwnerLogin != "joshuapeters" || p.OwnerType != "User" {
		t.Errorf("owner = (%q,%q)", p.OwnerLogin, p.OwnerType)
	}
	if p.Fields.Status.Options["In Review"] != "opt_inr" {
		t.Errorf("Status options[In Review] = %q", p.Fields.Status.Options["In Review"])
	}
	if !strings.Contains(out.String(), "auth") {
		t.Errorf("expected slug in confirmation, got %q", out.String())
	}
}

func TestRunProjectNew_RejectsDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".klankyrc.json")
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "o", Name: "r"},
		Projects: map[string]config.Project{
			"taken": {NodeID: "X"},
		},
	}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := RunProjectNew(context.Background(), gh.NewFakeRunner(), Options{
		Slug: "taken", Title: "Whatever", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected duplicate-slug error, got %v", err)
	}
}

func TestRunProjectNew_BadSlug(t *testing.T) {
	cfgPath := seedConfig(t)
	err := RunProjectNew(context.Background(), gh.NewFakeRunner(), Options{
		Slug: "Bad Slug", Title: "X", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "slug") {
		t.Errorf("expected slug validation error, got %v", err)
	}
}

func TestRunProjectNew_ResolveOwnerOrgFallback(t *testing.T) {
	cfgPath := seedConfig(t)

	fake := gh.NewFakeRunner()
	const ownerLookupQuery = `query($login: String!) {
  user(login: $login) { id }
  organization(login: $login) { id }
}`
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + ownerLookupQuery,
			"-f", "login=wistia"},
		[]byte(`{"data":{"user":null,"organization":{"id":"O_wistia"}}}`),
		nil,
	)
	stubRepoID(fake, "joshuapeters", "klanky", "R_repo")
	stubCreate(fake, "O_wistia", "Auth", "R_repo",
		"PVT_new", 1, "https://github.com/orgs/wistia/projects/1")
	stubStatusField(fake, "PVT_new")
	stubLabelMissing(fake, "joshuapeters/klanky")

	err := RunProjectNew(context.Background(), fake, Options{
		Slug: "auth", Title: "Auth", Owner: "wistia", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunProjectNew: %v", err)
	}
	cfg, _ := config.LoadConfig(cfgPath)
	p := cfg.Projects["auth"]
	if p.OwnerLogin != "wistia" || p.OwnerType != "Organization" {
		t.Errorf("owner = (%q,%q)", p.OwnerLogin, p.OwnerType)
	}
}

func TestRunProjectNew_OwnerNotFound(t *testing.T) {
	cfgPath := seedConfig(t)
	fake := gh.NewFakeRunner()
	const ownerLookupQuery = `query($login: String!) {
  user(login: $login) { id }
  organization(login: $login) { id }
}`
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + ownerLookupQuery,
			"-f", "login=ghost"},
		[]byte(`{"data":{"user":null,"organization":null}}`),
		nil,
	)
	err := RunProjectNew(context.Background(), fake, Options{
		Slug: "auth", Title: "Auth", Owner: "ghost", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no GitHub user or organization") {
		t.Errorf("expected owner-not-found error, got %v", err)
	}
}
