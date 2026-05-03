package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/theforager/cmux/internal/types"
)

const endpoint = "https://api.linear.app/graphql"

func Issue(identifier string) (types.LinearIssue, error) {
	query := `query Issue($id: String!) {
  issue(id: $id) {
    id identifier title description url branchName
    state { name }
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
				Name string `json:"name"`
			} `json:"state"`
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
	}, nil
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
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Linear: %s", envelope.Errors[0].Message)
	}
	return json.Unmarshal(envelope.Data, out)
}
