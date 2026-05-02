package workqueue

import (
	"testing"

	"github.com/joshuapeters/klanky/internal/snapshot"
)

func ptrInt(n int) *int { return &n }

func TestSelectWork_PicksLowestOpenPhase(t *testing.T) {
	snap := &snapshot.Snapshot{
		Tasks: []snapshot.TaskInfo{
			{Number: 1, State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "Todo", Phase: ptrInt(2)},
			{Number: 3, State: "OPEN", Status: "Todo", Phase: ptrInt(3)},
		},
	}
	got := SelectWork(snap)
	if got.AllDone {
		t.Error("AllDone = true, want false")
	}
	if got.CurrentPhase != 2 {
		t.Errorf("CurrentPhase = %d, want 2", got.CurrentPhase)
	}
	if len(got.Eligible) != 1 || got.Eligible[0].Number != 2 {
		t.Errorf("Eligible = %+v, want one task #2", got.Eligible)
	}
}

func TestSelectWork_AllClosed_AllDone(t *testing.T) {
	snap := &snapshot.Snapshot{
		Tasks: []snapshot.TaskInfo{
			{Number: 1, State: "CLOSED", Status: "Done", Phase: ptrInt(1)},
			{Number: 2, State: "CLOSED", Status: "Done", Phase: ptrInt(2)},
		},
	}
	got := SelectWork(snap)
	if !got.AllDone {
		t.Error("AllDone = false, want true")
	}
}

func TestSelectWork_OnlyInReviewInPhase(t *testing.T) {
	snap := &snapshot.Snapshot{
		Tasks: []snapshot.TaskInfo{
			{Number: 1, State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "In Review", Phase: ptrInt(1)},
		},
		PRsByBranch: map[string]snapshot.PRInfo{
			"klanky/feat-7/task-1": {Number: 11, URL: "u1", State: "OPEN"},
			"klanky/feat-7/task-2": {Number: 12, URL: "u2", State: "OPEN"},
		},
	}
	got := SelectWork(snap)
	if got.AllDone {
		t.Error("AllDone = true, want false")
	}
	if got.CurrentPhase != 1 {
		t.Errorf("CurrentPhase = %d, want 1", got.CurrentPhase)
	}
	if len(got.Eligible) != 0 {
		t.Errorf("Eligible = %+v, want empty", got.Eligible)
	}
	if len(got.AwaitingReview) != 2 {
		t.Errorf("len(AwaitingReview) = %d, want 2", len(got.AwaitingReview))
	}
}

func TestSelectWork_IncludesNeedsAttentionInEligible(t *testing.T) {
	snap := &snapshot.Snapshot{
		Tasks: []snapshot.TaskInfo{
			{Number: 1, State: "OPEN", Status: "Todo", Phase: ptrInt(1)},
			{Number: 2, State: "OPEN", Status: "Needs Attention", Phase: ptrInt(1)},
		},
	}
	got := SelectWork(snap)
	if len(got.Eligible) != 2 {
		t.Errorf("Eligible len = %d, want 2 (todo + needs-attention)", len(got.Eligible))
	}
}

func TestSelectWork_FlagsSurvivingInProgress(t *testing.T) {
	snap := &snapshot.Snapshot{
		Tasks: []snapshot.TaskInfo{
			{Number: 1, State: "OPEN", Status: "In Progress", Phase: ptrInt(1)},
		},
	}
	got := SelectWork(snap)
	if len(got.SurvivingInProgress) != 1 {
		t.Errorf("len(SurvivingInProgress) = %d, want 1", len(got.SurvivingInProgress))
	}
}

func TestSelectWork_NoTasks_AllDone(t *testing.T) {
	snap := &snapshot.Snapshot{Tasks: []snapshot.TaskInfo{}}
	got := SelectWork(snap)
	if !got.AllDone {
		t.Error("AllDone should be true when no tasks exist")
	}
}
