package main

import (
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestDefaultStateIDs(t *testing.T) {
	states := []types.LinearWorkflowState{
		{ID: "done", Name: "Done"},
		{ID: "backlog", Name: "Backlog"},
		{ID: "todo", Name: "Todo"},
		{ID: "scoping", Name: "Scoping"},
	}
	got := defaultStateIDs(states)
	want := []string{"todo", "scoping", "backlog"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("defaultStateIDs = %v, want %v", got, want)
		}
	}
}
