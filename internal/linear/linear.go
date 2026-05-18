package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/theforager/cmux/internal/types"
)

const endpoint = "https://api.linear.app/graphql"

func Issue(identifier string) (types.LinearIssue, error) {
	query := `query Issue($id: String!) {
  issue(id: $id) {
    id identifier title description url branchName
    state { id name }
    team { id key name }
    labels(first: 100) { nodes { id name } }
  }
}`
	var out struct {
		Issue struct {
			ID          string `json:"id"`
			Identifier  string `json:"identifier"`
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			BranchName  string `json:"branchName"`
			State       struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"state"`
			Team struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"team"`
			Labels struct {
				Nodes []types.LinearLabel `json:"nodes"`
			} `json:"labels"`
		} `json:"issue"`
	}
	if err := graphql(query, map[string]any{"id": identifier}, &out); err != nil {
		return types.LinearIssue{}, err
	}
	if out.Issue.ID == "" {
		return types.LinearIssue{}, fmt.Errorf("Linear issue not found: %s", identifier)
	}
	return types.LinearIssue{
		ID:          out.Issue.ID,
		Identifier:  out.Issue.Identifier,
		Title:       out.Issue.Title,
		Description: out.Issue.Description,
		URL:         out.Issue.URL,
		BranchName:  out.Issue.BranchName,
		State:       out.Issue.State.Name,
		StateID:     out.Issue.State.ID,
		TeamID:      out.Issue.Team.ID,
		TeamKey:     out.Issue.Team.Key,
		TeamName:    out.Issue.Team.Name,
		Labels:      out.Issue.Labels.Nodes,
	}, nil
}

type IssueFilterOptions struct {
	Teams        []string
	States       []string
	Labels       []string
	AssigneeMode string
	ViewerID     string
	Priority     []int
	OpenOnly     bool
}

func Viewer() (types.LinearViewer, error) {
	query := `query Viewer { viewer { id name email } }`
	var out struct {
		Viewer types.LinearViewer `json:"viewer"`
	}
	if err := graphql(query, nil, &out); err != nil {
		return types.LinearViewer{}, err
	}
	return out.Viewer, nil
}

func ListTeams() ([]types.LinearTeam, error) {
	query := `query Teams { teams(first: 100) { nodes { id key name } } }`
	var out struct {
		Teams struct {
			Nodes []types.LinearTeam `json:"nodes"`
		} `json:"teams"`
	}
	if err := graphql(query, nil, &out); err != nil {
		return nil, err
	}
	return out.Teams.Nodes, nil
}

func ListWorkflowStates() ([]types.LinearWorkflowState, error) {
	query := `query WorkflowStates {
  workflowStates(first: 250) { nodes { id name type team { id } } }
}`
	var raw struct {
		WorkflowStates struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
				Team struct {
					ID string `json:"id"`
				} `json:"team"`
			} `json:"nodes"`
		} `json:"workflowStates"`
	}
	if err := graphql(query, nil, &raw); err != nil {
		return nil, err
	}
	var out []types.LinearWorkflowState
	for _, node := range raw.WorkflowStates.Nodes {
		out = append(out, types.LinearWorkflowState{ID: node.ID, Name: node.Name, Type: node.Type, TeamID: node.Team.ID})
	}
	return out, nil
}

func ResolveWorkflowStateID(value, teamID string) (string, error) {
	states, err := ListWorkflowStates()
	if err != nil {
		return "", err
	}
	return ResolveWorkflowStateIDFromStates(states, value, teamID)
}

func ResolveWorkflowStateIDFromStates(states []types.LinearWorkflowState, value, teamID string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
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
	if teamID != "" {
		for _, state := range matches {
			if state.TeamID == teamID {
				return state.ID, nil
			}
		}
	}
	return matches[0].ID, nil
}

func ListLabels() ([]types.LinearLabel, error) {
	query := `query Labels { issueLabels(first: 250) { nodes { id name } } }`
	var out struct {
		IssueLabels struct {
			Nodes []types.LinearLabel `json:"nodes"`
		} `json:"issueLabels"`
	}
	if err := graphql(query, nil, &out); err != nil {
		return nil, err
	}
	return out.IssueLabels.Nodes, nil
}

func ListIssues(preset types.QueuePreset) ([]types.LinearIssue, error) {
	viewerID := ""
	if preset.AssigneeMode == "viewer" {
		viewer, err := Viewer()
		if err != nil {
			return nil, err
		}
		viewerID = viewer.ID
	}
	limit := preset.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 250 {
		limit = 250
	}
	filter := BuildIssueFilter(IssueFilterOptions{Teams: preset.Teams, States: preset.States, Labels: preset.Labels, AssigneeMode: preset.AssigneeMode, ViewerID: viewerID, Priority: preset.Priority, OpenOnly: len(preset.States) == 0})
	query := `query Issues($filter: IssueFilter, $first: Int!) {
  issues(filter: $filter, first: $first, sort: [{ manual: { order: Ascending } }]) {
    nodes {
      id identifier title description url branchName priority sortOrder
      state { id name }
      team { id key name }
      assignee { id name }
    }
  }
}`
	var raw struct {
		Issues struct {
			Nodes []struct {
				ID          string  `json:"id"`
				Identifier  string  `json:"identifier"`
				Title       string  `json:"title"`
				Description string  `json:"description"`
				URL         string  `json:"url"`
				BranchName  string  `json:"branchName"`
				Priority    int     `json:"priority"`
				SortOrder   float64 `json:"sortOrder"`
				State       struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"state"`
				Team struct {
					ID   string `json:"id"`
					Key  string `json:"key"`
					Name string `json:"name"`
				} `json:"team"`
				Assignee struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"assignee"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := graphql(query, map[string]any{"filter": filter, "first": limit}, &raw); err != nil {
		return nil, err
	}
	var issues []types.LinearIssue
	for _, node := range raw.Issues.Nodes {
		issues = append(issues, types.LinearIssue{
			ID:           node.ID,
			Identifier:   node.Identifier,
			Title:        node.Title,
			Description:  node.Description,
			URL:          node.URL,
			BranchName:   node.BranchName,
			State:        node.State.Name,
			StateID:      node.State.ID,
			TeamID:       node.Team.ID,
			TeamKey:      node.Team.Key,
			TeamName:     node.Team.Name,
			Priority:     node.Priority,
			SortOrder:    node.SortOrder,
			AssigneeID:   node.Assignee.ID,
			AssigneeName: node.Assignee.Name,
		})
	}
	return issues, nil
}

func BuildIssueFilter(o IssueFilterOptions) map[string]any {
	filter := map[string]any{}
	if len(o.Teams) > 0 {
		filter["team"] = map[string]any{"id": map[string]any{"in": o.Teams}}
	}
	if len(o.States) > 0 {
		filter["state"] = map[string]any{"id": map[string]any{"in": o.States}}
	}
	if len(o.Labels) > 0 {
		filter["labels"] = map[string]any{"id": map[string]any{"in": o.Labels}}
	}
	switch o.AssigneeMode {
	case "viewer":
		if o.ViewerID != "" {
			filter["assignee"] = map[string]any{"id": map[string]any{"eq": o.ViewerID}}
		}
	case "unassigned":
		filter["assignee"] = map[string]any{"null": true}
	}
	if len(o.Priority) > 0 {
		filter["priority"] = map[string]any{"in": o.Priority}
	}
	if o.OpenOnly {
		filter["state"] = map[string]any{"type": map[string]any{"nin": []string{"completed", "canceled"}}}
	}
	return filter
}

func UpsertComment(issueID, commentID, body string) (string, error) {
	if issueID == "" {
		return commentID, nil
	}
	if commentID != "" {
		query := `mutation UpdateComment($id: String!, $body: String!) { commentUpdate(id: $id, input: { body: $body }) { success } }`
		var out any
		return commentID, graphql(query, map[string]any{"id": commentID, "body": body}, &out)
	}
	query := `mutation CreateComment($issueId: String!, $body: String!) {
  commentCreate(input: { issueId: $issueId, body: $body }) { comment { id } }
}`
	var out struct {
		CommentCreate struct {
			Comment struct {
				ID string `json:"id"`
			} `json:"comment"`
		} `json:"commentCreate"`
	}
	if err := graphql(query, map[string]any{"issueId": issueID, "body": body}, &out); err != nil {
		return "", err
	}
	return out.CommentCreate.Comment.ID, nil
}

type IssueUpdateOptions struct {
	SortOrder   *float64
	Description *string
}

func TopSortOrder(stateID, teamID string) (float64, bool, error) {
	if stateID == "" {
		return 0, false, nil
	}
	filter := map[string]any{"state": map[string]any{"id": map[string]any{"in": []string{stateID}}}}
	if teamID != "" {
		filter["team"] = map[string]any{"id": map[string]any{"in": []string{teamID}}}
	}
	query := `query TopIssue($filter: IssueFilter!) {
  issues(filter: $filter, first: 1, sort: [{ manual: { order: Ascending } }]) {
    nodes { id sortOrder }
  }
}`
	var out struct {
		Issues struct {
			Nodes []struct {
				ID        string  `json:"id"`
				SortOrder float64 `json:"sortOrder"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := graphql(query, map[string]any{"filter": filter}, &out); err != nil {
		return 0, false, err
	}
	if len(out.Issues.Nodes) == 0 {
		return 0, false, nil
	}
	return out.Issues.Nodes[0].SortOrder, true, nil
}

func UpdateIssue(issueID string, stateID string, labelIDs []string) (types.LinearIssue, error) {
	return UpdateIssueWithOptions(issueID, stateID, labelIDs, IssueUpdateOptions{})
}

func UpdateIssueWithOptions(issueID string, stateID string, labelIDs []string, options IssueUpdateOptions) (types.LinearIssue, error) {
	if issueID == "" {
		return types.LinearIssue{}, nil
	}
	input := map[string]any{}
	if stateID != "" {
		input["stateId"] = stateID
	}
	if labelIDs != nil {
		input["labelIds"] = labelIDs
	}
	if options.SortOrder != nil {
		input["sortOrder"] = *options.SortOrder
	}
	if options.Description != nil {
		input["description"] = *options.Description
	}
	if len(input) == 0 {
		return types.LinearIssue{}, nil
	}
	query := `mutation UpdateIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {
      id identifier title url
      state { id name }
      labels(first: 100) { nodes { id name } }
    }
  }
}`
	var out struct {
		IssueUpdate struct {
			Issue struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				URL        string `json:"url"`
				State      struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"state"`
				Labels struct {
					Nodes []types.LinearLabel `json:"nodes"`
				} `json:"labels"`
			} `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := graphql(query, map[string]any{"id": issueID, "input": input}, &out); err != nil {
		return types.LinearIssue{}, err
	}
	issue := out.IssueUpdate.Issue
	return types.LinearIssue{
		ID:         issue.ID,
		Identifier: issue.Identifier,
		Title:      issue.Title,
		URL:        issue.URL,
		State:      issue.State.Name,
		StateID:    issue.State.ID,
		Labels:     issue.Labels.Nodes,
	}, nil
}

func graphql(query string, vars map[string]any, out any) error {
	key := os.Getenv("LINEAR_API_KEY")
	if key == "" {
		return fmt.Errorf("LINEAR_API_KEY is not set")
	}
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("Linear HTTP %d: %s", res.StatusCode, string(bodyBytes))
		}
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Linear: %s", envelope.Errors[0].Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Linear HTTP %d: %s", res.StatusCode, string(bodyBytes))
	}
	return json.Unmarshal(envelope.Data, out)
}
