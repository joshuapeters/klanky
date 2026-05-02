package add

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapeters/klanky/internal/config"
	"github.com/joshuapeters/klanky/internal/gh"
)

func seedConfig(t *testing.T, p config.Project) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".klankyrc.json")
	cfg := &config.Config{
		SchemaVersion: config.SchemaVersion,
		Repo:          config.Repo{Owner: "joshuapeters", Name: "klanky"},
		Projects:      map[string]config.Project{"auth": p},
	}
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return path
}

func defaultProject() config.Project {
	return config.Project{
		URL: "https://github.com/users/joshuapeters/projects/4", Number: 4,
		NodeID: "PVT_proj", Title: "Auth", OwnerLogin: "joshuapeters", OwnerType: "User",
		Fields: config.ProjectFields{
			Status: config.StatusField{ID: "PVTSSF_status", Name: "Status",
				Options: map[string]string{
					"Todo": "opt_todo", "In Progress": "opt_inp",
					"In Review": "opt_inr", "Needs Attention": "opt_na", "Done": "opt_done",
				},
			},
		},
	}
}

const setStatusMutation = `mutation($pid: ID!, $iid: ID!, $fid: ID!, $oid: String!) {
  updateProjectV2ItemFieldValue(input: {projectId: $pid, itemId: $iid, fieldId: $fid, value: {singleSelectOptionId: $oid}}) {
    projectV2Item { id }
  }
}`

const addBlockedByMutation = `mutation($issueId: ID!, $blockingIssueId: ID!) {
  addBlockedBy(input: {issueId: $issueId, blockingIssueId: $blockingIssueId}) {
    issue { id }
  }
}`

func stubCreateIssue(fake *gh.FakeRunner, repo, title, body, url string, number int, nodeID string) {
	fake.Stub(
		[]string{"gh", "issue", "create",
			"--repo", repo,
			"--title", title,
			"--body", body,
			"--label", "klanky:tracked"},
		[]byte(url+"\n"),
		nil,
	)
	fake.Stub(
		[]string{"gh", "issue", "view", itoa(number),
			"--repo", repo, "--json", "id"},
		[]byte(`{"id":"`+nodeID+`"}`),
		nil,
	)
}

func stubItemAdd(fake *gh.FakeRunner, ownerLogin string, projNumber int, url, itemID string) {
	fake.Stub(
		[]string{"gh", "project", "item-add", itoa(projNumber),
			"--owner", ownerLogin,
			"--url", url,
			"--format", "json"},
		[]byte(`{"id":"`+itemID+`"}`),
		nil,
	)
}

func stubSetStatusTodo(fake *gh.FakeRunner, p config.Project, itemID string) {
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + setStatusMutation,
			"-f", "fid=" + p.Fields.Status.ID,
			"-f", "iid=" + itemID,
			"-f", "oid=" + p.Fields.Status.Options["Todo"],
			"-f", "pid=" + p.NodeID},
		[]byte(`{"data":{"updateProjectV2ItemFieldValue":{"projectV2Item":{"id":"`+itemID+`"}}}}`),
		nil,
	)
}

func stubAddBlockedBy(fake *gh.FakeRunner, issueID, blockingID string) {
	fake.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + addBlockedByMutation,
			"-f", "blockingIssueId=" + blockingID,
			"-f", "issueId=" + issueID},
		[]byte(`{"data":{"addBlockedBy":{"issue":{"id":"`+issueID+`"}}}}`),
		nil,
	)
}

func stubDepView(fake *gh.FakeRunner, repo string, number int, state, nodeID string) {
	fake.Stub(
		[]string{"gh", "issue", "view", itoa(number),
			"--repo", repo, "--json", "id,state,number"},
		[]byte(`{"id":"`+nodeID+`","state":"`+state+`","number":`+itoa(number)+`}`),
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

func TestParseDependsOn(t *testing.T) {
	cases := map[string][]int{
		"":            nil,
		"   ":         nil,
		"42":          {42},
		"#42":         {42},
		"12, 7, 9":    {12, 7, 9},
		"  3 , 4  ,5": {3, 4, 5},
	}
	for in, want := range cases {
		got, err := parseDependsOn(in)
		if err != nil {
			t.Errorf("parseDependsOn(%q) error: %v", in, err)
			continue
		}
		if !slicesEqualInt(got, want) {
			t.Errorf("parseDependsOn(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseDependsOn("12,abc,5"); err == nil {
		t.Errorf("expected error for non-integer token")
	}
}

func slicesEqualInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunIssueAdd_HappyPath_NoDeps(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	fake := gh.NewFakeRunner()
	stubCreateIssue(fake, "joshuapeters/klanky",
		"Refactor login", "explanation here",
		"https://github.com/joshuapeters/klanky/issues/42", 42, "I_42")
	stubItemAdd(fake, "joshuapeters", 4, "https://github.com/joshuapeters/klanky/issues/42", "PVTI_42")
	stubSetStatusTodo(fake, defaultProject(), "PVTI_42")

	var out bytes.Buffer
	err := RunIssueAdd(context.Background(), fake, Options{
		ProjectSlug: "auth",
		Title:       "Refactor login",
		Body:        "explanation here",
		ConfigPath:  cfgPath,
	}, &out)
	if err != nil {
		t.Fatalf("RunIssueAdd: %v", err)
	}
	if !strings.Contains(out.String(), "#42") {
		t.Errorf("expected #42 in output, got %q", out.String())
	}
}

func TestRunIssueAdd_WithDeps_SortedAndOpen(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	fake := gh.NewFakeRunner()
	// Dep validation (in input order: 9, 7).
	stubDepView(fake, "joshuapeters/klanky", 9, "OPEN", "I_9")
	stubDepView(fake, "joshuapeters/klanky", 7, "OPEN", "I_7")
	// Create.
	stubCreateIssue(fake, "joshuapeters/klanky", "Login UI", "",
		"https://github.com/joshuapeters/klanky/issues/42", 42, "I_42")
	stubItemAdd(fake, "joshuapeters", 4, "https://github.com/joshuapeters/klanky/issues/42", "PVTI_42")
	stubSetStatusTodo(fake, defaultProject(), "PVTI_42")
	// addBlockedBy issued in sorted dep order: 7 then 9.
	stubAddBlockedBy(fake, "I_42", "I_7")
	stubAddBlockedBy(fake, "I_42", "I_9")

	err := RunIssueAdd(context.Background(), fake, Options{
		ProjectSlug: "auth",
		Title:       "Login UI",
		DependsOn:   []int{9, 7},
		ConfigPath:  cfgPath,
		Output:      "json",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunIssueAdd: %v", err)
	}
}

func TestRunIssueAdd_RefusesClosedDep(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	fake := gh.NewFakeRunner()
	stubDepView(fake, "joshuapeters/klanky", 7, "CLOSED", "I_7")
	err := RunIssueAdd(context.Background(), fake, Options{
		ProjectSlug: "auth", Title: "x", DependsOn: []int{7}, ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "OPEN") {
		t.Errorf("expected closed-dep error, got %v", err)
	}
}

func TestRunIssueAdd_UnknownProjectSlug(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	err := RunIssueAdd(context.Background(), gh.NewFakeRunner(), Options{
		ProjectSlug: "nope", Title: "x", ConfigPath: cfgPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no project") {
		t.Errorf("expected unknown-slug error, got %v", err)
	}
}

func TestRunIssueAdd_PartialFailureAfterCreate(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	fake := gh.NewFakeRunner()
	stubCreateIssue(fake, "joshuapeters/klanky", "x", "",
		"https://github.com/joshuapeters/klanky/issues/42", 42, "I_42")
	// Project add fails — no stub registered for it.

	var out bytes.Buffer
	err := RunIssueAdd(context.Background(), fake, Options{
		ProjectSlug: "auth", Title: "x", ConfigPath: cfgPath,
	}, &out)
	if err == nil {
		t.Fatal("expected error from missing project item-add stub")
	}
	if !strings.Contains(out.String(), "#42") {
		t.Errorf("expected partial-success message mentioning issue, got %q", out.String())
	}
}

func TestRunIssueAdd_JSONOutput(t *testing.T) {
	cfgPath := seedConfig(t, defaultProject())
	fake := gh.NewFakeRunner()
	stubCreateIssue(fake, "joshuapeters/klanky", "x", "",
		"https://github.com/joshuapeters/klanky/issues/42", 42, "I_42")
	stubItemAdd(fake, "joshuapeters", 4, "https://github.com/joshuapeters/klanky/issues/42", "PVTI_42")
	stubSetStatusTodo(fake, defaultProject(), "PVTI_42")

	var out bytes.Buffer
	err := RunIssueAdd(context.Background(), fake, Options{
		ProjectSlug: "auth", Title: "x", ConfigPath: cfgPath, Output: "json",
	}, &out)
	if err != nil {
		t.Fatalf("RunIssueAdd: %v", err)
	}
	var got Result
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out.String())
	}
	if got.Number != 42 || got.NodeID != "I_42" || got.Status != "Todo" {
		t.Errorf("got %+v", got)
	}
}
