package linear

import (
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestBuildIssueFilter(t *testing.T) {
	filter := BuildIssueFilter(IssueFilterOptions{
		Teams:        []string{"team-id"},
		States:       []string{"state-id"},
		Labels:       []string{"label-id"},
		AssigneeMode: "viewer",
		ViewerID:     "viewer-id",
		Priority:     []int{1, 2},
	})
	if filter["team"] == nil || filter["state"] == nil || filter["labels"] == nil || filter["assignee"] == nil || filter["priority"] == nil {
		t.Fatalf("missing filter keys: %+v", filter)
	}
}

func TestBuildIssueFilterUnassigned(t *testing.T) {
	filter := BuildIssueFilter(IssueFilterOptions{AssigneeMode: "unassigned"})
	if filter["assignee"] == nil {
		t.Fatalf("missing assignee filter: %+v", filter)
	}
}

func TestBuildIssueFilterOpenOnly(t *testing.T) {
	filter := BuildIssueFilter(IssueFilterOptions{OpenOnly: true})
	if filter["state"] == nil {
		t.Fatalf("missing state filter: %+v", filter)
	}
}

func TestResolveWorkflowStateIDPrefersTeamMatch(t *testing.T) {
	states := []types.LinearWorkflowState{
		{ID: "review-a", Name: "Review", TeamID: "team-a"},
		{ID: "review-b", Name: "Review", TeamID: "team-b"},
	}
	got, err := ResolveWorkflowStateIDFromStates(states, "review", "team-b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "review-b" {
		t.Fatalf("state id = %q, want review-b", got)
	}
}

func TestResolveWorkflowStateIDAcceptsID(t *testing.T) {
	states := []types.LinearWorkflowState{{ID: "todo-id", Name: "Todo"}}
	got, err := ResolveWorkflowStateIDFromStates(states, "todo-id", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "todo-id" {
		t.Fatalf("state id = %q, want todo-id", got)
	}
}
