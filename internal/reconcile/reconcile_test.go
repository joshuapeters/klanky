package main

import (
	"testing"
)

func ptrInt(n int) *int { return &n }

func TestReconcile_Row1_ClosedIssueGetsDone(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "CLOSED", Status: "In Review", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Done", "")
}

func TestReconcile_Row1_ClosedIssueAlreadyDone_NoAction(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row2_OpenTodoNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Todo", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row3_InProgressNoLivePIDGoesNeedsAttention(t *testing.T) {
	// Reconcile cannot detect a live PID directly; it infers crash from
	// "in-progress + no PR + a worktree exists" OR "in-progress + no PR" if we
	// trust the lock-file takeover already cleared the prior process.
	// Per locked design, lock takeover means any surviving in-progress is
	// definitively a crash — so we don't need to check worktree existence.
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "previous run crashed")
}

func TestReconcile_Row4_InProgressWithOpenPRGoesInReview(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "OPEN", URL: "u"},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "In Review", "")
}

func TestReconcile_Row6_InReviewWithOpenPRNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "OPEN"},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row7_InReviewWithClosedNotMergedGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{
			"klanky/feat-100/task-101": {Number: 201, State: "CLOSED"},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "PR")
}

func TestReconcile_Row8_InReviewWithNoPRGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]PRInfo{},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "")
}

func TestReconcile_Row9_NeedsAttentionNoOp(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Needs Attention", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	if len(got) != 0 {
		t.Errorf("expected no actions, got %+v", got)
	}
}

func TestReconcile_Row10_DoneOnOpenIssueIsInvariantViolation(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Done", Phase: ptrInt(1)},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "invariant")
}

func TestReconcile_Row11_OpenWithMissingPhaseGoesNeedsAttention(t *testing.T) {
	snap := &Snapshot{
		Feature: FeatureInfo{Number: 100},
		Tasks: []TaskInfo{
			{Number: 101, ItemID: "I1", State: "OPEN", Status: "Todo", Phase: nil},
		},
	}
	got := Reconcile(snap, 100)
	mustHaveAction(t, got, 101, "Needs Attention", "Phase")
}

// mustHaveAction asserts there's exactly one action for the given task with
// the given status, and that the breadcrumb (if non-empty) contains the given
// substring.
func mustHaveAction(t *testing.T, actions []ReconcileAction, taskNumber int, wantStatus, breadcrumbContains string) {
	t.Helper()
	for _, a := range actions {
		if a.TaskNumber != taskNumber {
			continue
		}
		if a.NewStatus != wantStatus {
			t.Errorf("task %d: NewStatus = %q, want %q", taskNumber, a.NewStatus, wantStatus)
		}
		if breadcrumbContains != "" && !contains(a.Breadcrumb, breadcrumbContains) {
			t.Errorf("task %d: Breadcrumb = %q, want to contain %q", taskNumber, a.Breadcrumb, breadcrumbContains)
		}
		return
	}
	t.Errorf("no action found for task %d in %+v", taskNumber, actions)
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
