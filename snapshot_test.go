package main

import (
	"context"
	"strings"
	"testing"
)

func mockConfig() *Config {
	return &Config{
		SchemaVersion: 1,
		Repo:          ConfigRepo{Owner: "alice", Name: "proj"},
		Project: ConfigProject{
			URL: "https://github.com/users/alice/projects/1", Number: 1,
			NodeID: "PVT_x", OwnerLogin: "alice", OwnerType: "User",
			Fields: ConfigFields{
				Phase: ConfigField{ID: "PVTF_p", Name: "Phase"},
				Status: ConfigStatusField{ID: "PVTSSF_s", Name: "Status",
					Options: map[string]string{
						"Todo": "a", "In Progress": "b",
						"In Review": "c", "Needs Attention": "d", "Done": "e",
					}},
			},
		},
		FeatureLabel: ConfigLabel{Name: "klanky:feature"},
	}
}

func TestFetchSnapshot_ParsesTasksAndProjectFields(t *testing.T) {
	r := NewFakeRunner()

	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "Auth refactor",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "Add login form", "body": "## Context\n...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [{
					"id": "PVTI_101",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}
					]}
				}]}
			},
			{
				"number": 102, "title": "Add session middleware", "body": "## Context\n...",
				"state": "CLOSED", "id": "I_102",
				"projectItems": {"nodes": [{
					"id": "PVTI_102",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": [
						{"field": {"name": "Phase"}, "number": 1},
						{"field": {"name": "Status"}, "name": "Done", "optionId": "e"}
					]}
				}]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)

	prResp := `[{"headRefName":"klanky/feat-100/task-101","number":201,"url":"https://github.com/alice/proj/pull/201","state":"OPEN","closed":false,"merged":false}]`
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(prResp), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}

	if snap.Feature.Number != 100 {
		t.Errorf("Feature.Number = %d, want 100", snap.Feature.Number)
	}
	if len(snap.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(snap.Tasks))
	}

	t101 := findTask(t, snap.Tasks, 101)
	if t101.State != "OPEN" {
		t.Errorf("task 101 State = %q, want OPEN", t101.State)
	}
	if t101.ItemID != "PVTI_101" {
		t.Errorf("task 101 ItemID = %q, want PVTI_101", t101.ItemID)
	}
	if t101.Phase == nil || *t101.Phase != 1 {
		t.Errorf("task 101 Phase = %v, want 1", t101.Phase)
	}
	if t101.Status != "Todo" {
		t.Errorf("task 101 Status = %q, want Todo", t101.Status)
	}

	t102 := findTask(t, snap.Tasks, 102)
	if t102.State != "CLOSED" {
		t.Errorf("task 102 State = %q, want CLOSED", t102.State)
	}
	if t102.Status != "Done" {
		t.Errorf("task 102 Status = %q, want Done", t102.Status)
	}

	pr, ok := snap.PRsByBranch["klanky/feat-100/task-101"]
	if !ok {
		t.Fatal("expected PR for klanky/feat-100/task-101")
	}
	if pr.Number != 201 || pr.State != "OPEN" {
		t.Errorf("PR = %+v, want number=201 state=OPEN", pr)
	}
}

func TestFetchSnapshot_HandlesMissingPhaseAndStatus(t *testing.T) {
	r := NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "T", "body": "...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [{
					"id": "PVTI_101",
					"project": {"id": "PVT_x"},
					"fieldValues": {"nodes": []}
				}]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.Tasks[0].Phase != nil {
		t.Errorf("expected Phase nil, got %v", snap.Tasks[0].Phase)
	}
	if snap.Tasks[0].Status != "" {
		t.Errorf("expected Status empty, got %q", snap.Tasks[0].Status)
	}
}

func TestFetchSnapshot_FiltersForeignProjectItems(t *testing.T) {
	r := NewFakeRunner()
	graphqlResp := `{"data":{"repository":{"issue":{
		"number": 100, "title": "F",
		"subIssues": {"nodes": [
			{
				"number": 101, "title": "T", "body": "...",
				"state": "OPEN", "id": "I_101",
				"projectItems": {"nodes": [
					{"id": "PVTI_other", "project": {"id": "PVT_other"},
					 "fieldValues": {"nodes": [{"field": {"name": "Status"}, "name": "Done", "optionId": "z"}]}},
					{"id": "PVTI_ours", "project": {"id": "PVT_x"},
					 "fieldValues": {"nodes": [{"field": {"name": "Status"}, "name": "Todo", "optionId": "a"}]}}
				]}
			}
		]}
	}}}}`
	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(graphqlResp), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	snap, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if snap.Tasks[0].ItemID != "PVTI_ours" {
		t.Errorf("ItemID = %q, want PVTI_ours (foreign project should be filtered)", snap.Tasks[0].ItemID)
	}
	if snap.Tasks[0].Status != "Todo" {
		t.Errorf("Status = %q, want Todo (foreign project's Done should not leak)", snap.Tasks[0].Status)
	}
}

func TestFetchSnapshot_RejectsTooManySubIssues(t *testing.T) {
	r := NewFakeRunner()

	var sb strings.Builder
	sb.WriteString(`{"data":{"repository":{"issue":{"number":100,"title":"F","subIssues":{"nodes":[`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"number":1,"title":"t","body":"","state":"OPEN","id":"x","projectItems":{"nodes":[]}}`)
	}
	sb.WriteString(`]}}}}}`)

	r.Stub(
		[]string{"gh", "api", "graphql",
			"-f", "query=" + snapshotQuery,
			"-F", "number=100",
			"-f", "owner=alice",
			"-f", "repo=proj"},
		[]byte(sb.String()), nil)
	r.Stub(
		[]string{"gh", "pr", "list",
			"--repo", "alice/proj",
			"--state", "all",
			"--search", "head:klanky/feat-100/",
			"--json", "headRefName,number,url,state,closed,merged",
			"--limit", "200"},
		[]byte(`[]`), nil)

	_, err := FetchSnapshot(context.Background(), r, mockConfig(), 100)
	if err != nil {
		t.Fatalf("100 sub-issues should be allowed; got error: %v", err)
	}
}

// Helper used in tests above.
func findTask(t *testing.T, tasks []TaskInfo, number int) TaskInfo {
	t.Helper()
	for _, task := range tasks {
		if task.Number == number {
			return task
		}
	}
	t.Fatalf("no task with number %d", number)
	return TaskInfo{}
}
