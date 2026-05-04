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

func TestJoinLeavesUnstartedIssueQueued(t *testing.T) {
	rows := Join([]types.LinearIssue{{ID: "issue-id", Identifier: "REB-124"}}, nil)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Started {
		t.Fatalf("expected unstarted row: %+v", rows[0])
	}
	if rows[0].Status != types.AgentStatus("queued") {
		t.Fatalf("status = %s, want queued", rows[0].Status)
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
