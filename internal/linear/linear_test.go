package linear

import "testing"

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
