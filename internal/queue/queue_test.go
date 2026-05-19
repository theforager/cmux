package queue

import (
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestJoinMarksExistingSessionByIdentifier(t *testing.T) {
	rows := Join(
		[]types.LinearIssue{{ID: "issue-id", Identifier: "REB-123", Title: "Fix bug"}},
		[]types.AgentSession{{ID: "REB-123", Linear: types.LinearData{IssueID: "issue-id", Identifier: "REB-123"}, Status: types.StatusWaiting}},
	)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if !rows[0].Started || !rows[0].Duplicate {
		t.Fatalf("expected started duplicate row: %+v", rows[0])
	}
	if rows[0].Status != types.StatusWaiting {
		t.Fatalf("status = %s, want %s", rows[0].Status, types.StatusWaiting)
	}
}

func TestJoinLeavesUnstartedIssueWithLinearStateBadge(t *testing.T) {
	rows := Join([]types.LinearIssue{{ID: "issue-id", Identifier: "REB-124", State: "In Progress"}}, nil)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Started {
		t.Fatalf("expected unstarted row: %+v", rows[0])
	}
	if rows[0].Status != types.AgentStatus("in-progress") {
		t.Fatalf("status = %s, want in-progress", rows[0].Status)
	}
}

func TestJoinPreservesLinearIssueOrder(t *testing.T) {
	rows := Join([]types.LinearIssue{
		{Identifier: "REB-3", State: "Backlog", SortOrder: 1},
		{Identifier: "REB-2", State: "Scoping", SortOrder: 1},
		{Identifier: "REB-1", State: "Todo", SortOrder: 2},
		{Identifier: "REB-0", State: "Todo", SortOrder: 1},
	}, nil)
	got := []string{rows[0].Issue.Identifier, rows[1].Issue.Identifier, rows[2].Issue.Identifier, rows[3].Issue.Identifier}
	want := []string{"REB-3", "REB-2", "REB-1", "REB-0"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestJoinWithPresetSortsByStateOrderThenManualOrder(t *testing.T) {
	rows := JoinWithPreset([]types.LinearIssue{
		{Identifier: "REB-3", StateID: "backlog", SortOrder: 1},
		{Identifier: "REB-2", StateID: "scoping", SortOrder: 1},
		{Identifier: "REB-1", StateID: "todo", SortOrder: 2},
		{Identifier: "REB-0", StateID: "todo", SortOrder: 1},
		{Identifier: "REB-4", StateID: "progress", SortOrder: 1},
	}, nil, types.QueuePreset{States: []string{"progress", "todo", "scoping", "backlog"}})
	got := []string{rows[0].Issue.Identifier, rows[1].Issue.Identifier, rows[2].Issue.Identifier, rows[3].Issue.Identifier}
	want := []string{"REB-4", "REB-0", "REB-1", "REB-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestJoinKeepsExistingSessionIndependentOfStatus(t *testing.T) {
	rows := Join(
		[]types.LinearIssue{{ID: "issue-id", Identifier: "REB-126"}},
		[]types.AgentSession{{ID: "REB-126-plan", Profile: types.ProfilePlan, Linear: types.LinearData{IssueID: "issue-id", Identifier: "REB-126"}, Status: types.StatusDone}},
	)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if !rows[0].Started {
		t.Fatalf("existing session should claim queue row independently of status: %+v", rows[0])
	}
}

func TestJoinPrefersFirstMatchingSession(t *testing.T) {
	rows := Join(
		[]types.LinearIssue{{ID: "issue-id", Identifier: "REB-127"}},
		[]types.AgentSession{
			{ID: "REB-127", Linear: types.LinearData{IssueID: "issue-id", Identifier: "REB-127"}, Status: types.StatusRunning},
			{ID: "REB-127-old", Linear: types.LinearData{IssueID: "issue-id", Identifier: "REB-127"}, Status: types.StatusDone},
		},
	)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Session == nil || rows[0].Session.ID != "REB-127" {
		t.Fatalf("session = %+v, want first matching session", rows[0].Session)
	}
	if rows[0].Status != types.StatusRunning {
		t.Fatalf("status = %s, want running", rows[0].Status)
	}
}
