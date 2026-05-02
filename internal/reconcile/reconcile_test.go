package reconcile

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/joshuapeters/klanky/internal/snapshot"
)

func snap(slug string, issues []snapshot.Issue, prs map[string]snapshot.PR) *snapshot.Snapshot {
	if prs == nil {
		prs = map[string]snapshot.PR{}
	}
	return &snapshot.Snapshot{ProjectSlug: slug, Issues: issues, PRsByBranch: prs}
}

func issue(num int, state, status string) snapshot.Issue {
	return snapshot.Issue{Number: num, ItemID: "PVTI_" + itoa(num), State: state, Status: status}
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

// ignoreBreadcrumb compares Action values without asserting on the freeform
// breadcrumb text — tests that care assert on it separately.
var ignoreBreadcrumb = cmpopts.IgnoreFields(Action{}, "Breadcrumb")

func find(actions []Action, num int) *Action {
	for i := range actions {
		if actions[i].IssueNumber == num {
			return &actions[i]
		}
	}
	return nil
}

func TestRow1_ClosedIssueGoesToDone(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{
		issue(1, "CLOSED", "In Progress"),
		issue(2, "CLOSED", "Done"), // already Done — no action
		issue(3, "CLOSED", ""),
	}, nil))
	want := []Action{
		{IssueNumber: 1, ItemID: "PVTI_1", NewStatus: "Done"},
		{IssueNumber: 3, ItemID: "PVTI_3", NewStatus: "Done"},
	}
	if diff := cmp.Diff(want, got, ignoreBreadcrumb); diff != "" {
		t.Errorf("actions mismatch (-want +got):\n%s", diff)
	}
}

func TestRow2_TodoIsNoOp(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{issue(1, "OPEN", "Todo")}, nil))
	if diff := cmp.Diff([]Action(nil), got); diff != "" {
		t.Errorf("expected no actions (-want +got):\n%s", diff)
	}
}

func TestRow3_InProgressWithoutPRGoesToNeedsAttention(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "In Progress")}, nil))
	a := find(got, 7)
	if a == nil || a.NewStatus != "Needs Attention" {
		t.Fatalf("got %+v, want Needs Attention", a)
	}
	if !strings.Contains(a.Breadcrumb, "crashed") {
		t.Errorf("breadcrumb should mention crash; got %q", a.Breadcrumb)
	}
}

func TestRow4_InProgressWithOpenPRGoesToInReview(t *testing.T) {
	prs := map[string]snapshot.PR{
		snapshot.BranchForIssue("auth", 7): {Number: 99, State: "OPEN", URL: "x", HeadRefName: snapshot.BranchForIssue("auth", 7)},
	}
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "In Progress")}, prs))
	want := []Action{{IssueNumber: 7, ItemID: "PVTI_7", NewStatus: "In Review"}}
	if diff := cmp.Diff(want, got, ignoreBreadcrumb); diff != "" {
		t.Errorf("actions mismatch (-want +got):\n%s", diff)
	}
}

func TestRow6_InReviewWithOpenPRIsNoOp(t *testing.T) {
	prs := map[string]snapshot.PR{
		snapshot.BranchForIssue("auth", 7): {Number: 99, State: "OPEN", HeadRefName: snapshot.BranchForIssue("auth", 7)},
	}
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "In Review")}, prs))
	if diff := cmp.Diff([]Action(nil), got); diff != "" {
		t.Errorf("expected no actions (-want +got):\n%s", diff)
	}
}

func TestRow7_InReviewWithClosedPRGoesToNeedsAttention(t *testing.T) {
	prs := map[string]snapshot.PR{
		snapshot.BranchForIssue("auth", 7): {Number: 99, State: "CLOSED", HeadRefName: snapshot.BranchForIssue("auth", 7)},
	}
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "In Review")}, prs))
	a := find(got, 7)
	if a == nil || a.NewStatus != "Needs Attention" {
		t.Fatalf("got %+v", a)
	}
	if !strings.Contains(a.Breadcrumb, "#99") {
		t.Errorf("breadcrumb should reference PR; got %q", a.Breadcrumb)
	}
}

func TestRow8_InReviewWithoutPRGoesToNeedsAttention(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "In Review")}, nil))
	a := find(got, 7)
	if a == nil || a.NewStatus != "Needs Attention" {
		t.Fatalf("got %+v", a)
	}
	if !strings.Contains(a.Breadcrumb, "no PR exists") {
		t.Errorf("breadcrumb should explain missing PR; got %q", a.Breadcrumb)
	}
}

func TestRow9_NeedsAttentionIsNoOp(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "Needs Attention")}, nil))
	if diff := cmp.Diff([]Action(nil), got); diff != "" {
		t.Errorf("expected no actions (-want +got):\n%s", diff)
	}
}

func TestRow10_OpenIssueWithDoneStatusGoesToNeedsAttention(t *testing.T) {
	got := Reconcile(snap("auth", []snapshot.Issue{issue(7, "OPEN", "Done")}, nil))
	a := find(got, 7)
	if a == nil || a.NewStatus != "Needs Attention" {
		t.Fatalf("got %+v", a)
	}
	if !strings.Contains(a.Breadcrumb, "invariant") {
		t.Errorf("breadcrumb should mention invariant; got %q", a.Breadcrumb)
	}
}
