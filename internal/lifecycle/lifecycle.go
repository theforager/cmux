package lifecycle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/theforager/cmux/internal/config"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/types"
)

const (
	EventStartScoping = "start_scoping"
	EventMarkScoped   = "mark_scoped"
	EventStartWork    = "start_work"
	EventNeedsReview  = "mark_needs_review"
	EventDone         = "done"
	EventAbandon      = "abandon"
)

func Apply(s *types.AgentSession, event string) error {
	if s.Linear.IssueID == "" {
		return nil
	}
	cfg, err := config.LoadOrDefault()
	if err != nil {
		return err
	}
	transition, ok := config.Transition(cfg, event)
	if !ok {
		return nil
	}
	issue, err := linear.Issue(valueOr(s.Linear.Identifier, s.Linear.IssueID))
	if err != nil {
		return err
	}
	if s.Linear.OriginalState == "" {
		s.Linear.OriginalState = issue.State
		s.Linear.OriginalStateID = issue.StateID
	}
	stateID, err := resolveStateID(transition.State, *s, issue)
	if err != nil {
		return err
	}
	labelIDs, err := transitionLabelIDs(issue.Labels, transition)
	if err != nil {
		return err
	}
	if stateID == "" && labelIDs == nil {
		return nil
	}
	options := linear.IssueUpdateOptions{}
	if transition.PlaceAtTop && stateID != "" {
		if top, ok, err := linear.TopSortOrder(stateID, issue.TeamID); err != nil {
			return err
		} else if ok {
			sortOrder := top - 1
			options.SortOrder = &sortOrder
		}
	}
	updated, err := linear.UpdateIssueWithOptions(s.Linear.IssueID, stateID, labelIDs, options)
	if err != nil {
		return err
	}
	if updated.State != "" {
		s.Linear.State = updated.State
		s.Linear.StateID = updated.StateID
	}
	return nil
}

func resolveStateID(value string, s types.AgentSession, issue types.LinearIssue) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value == "$previous_queue_state" {
		if s.Linear.OriginalStateID != "" {
			return s.Linear.OriginalStateID, nil
		}
		value = s.Linear.OriginalState
		if value == "" {
			value = issue.State
		}
	}
	states, err := linear.ListWorkflowStates()
	if err != nil {
		return "", err
	}
	for _, state := range states {
		if state.ID == value {
			return state.ID, nil
		}
	}
	var matches []types.LinearWorkflowState
	for _, state := range states {
		if strings.EqualFold(strings.TrimSpace(state.Name), value) {
			matches = append(matches, state)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("Linear workflow state not found: %s", value)
	}
	if issue.TeamID != "" {
		for _, state := range matches {
			if state.TeamID == issue.TeamID {
				return state.ID, nil
			}
		}
	}
	return matches[0].ID, nil
}

func transitionLabelIDs(current []types.LinearLabel, transition types.LinearTransition) ([]string, error) {
	if len(transition.AddLabels) == 0 && len(transition.RemoveLabels) == 0 {
		return nil, nil
	}
	labels, err := linear.ListLabels()
	if err != nil {
		return nil, err
	}
	ids := transitionLabelIDsWithLabels(current, transition, labels)
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func transitionLabelIDsWithLabels(current []types.LinearLabel, transition types.LinearTransition, labels []types.LinearLabel) map[string]bool {
	add := resolveLabelIDs(labels, transition.AddLabels)
	remove := resolveLabelIDs(labels, transition.RemoveLabels)
	ids := map[string]bool{}
	for _, label := range current {
		if label.ID != "" {
			ids[label.ID] = true
		}
	}
	for _, id := range remove {
		delete(ids, id)
	}
	for _, id := range add {
		ids[id] = true
	}
	return ids
}

func resolveLabelIDs(labels []types.LinearLabel, values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, label := range labels {
			if label.ID == value || strings.EqualFold(strings.TrimSpace(label.Name), value) {
				out = append(out, label.ID)
				break
			}
		}
	}
	return out
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
