package lifecycle

import (
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestTransitionLabelIDsAddsAndRemoves(t *testing.T) {
	current := []types.LinearLabel{
		{ID: "old-review-id", Name: "ready-for-review"},
		{ID: "keep-id", Name: "frontend"},
	}
	transition := types.LinearTransition{
		AddLabels:    []string{"needs-review"},
		RemoveLabels: []string{"ready-for-review"},
	}
	labels := []types.LinearLabel{
		{ID: "old-review-id", Name: "ready-for-review"},
		{ID: "review-id", Name: "needs-review"},
	}
	got := transitionLabelIDsWithLabels(current, transition, labels)
	if got["old-review-id"] {
		t.Fatalf("ready-for-review was not removed: %+v", got)
	}
	if !got["review-id"] || !got["keep-id"] {
		t.Fatalf("expected review and keep labels: %+v", got)
	}
}
