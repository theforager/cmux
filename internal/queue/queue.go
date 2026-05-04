package queue

import (
	"os"
	"strings"

	"github.com/theforager/cmux/internal/config"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/types"
)

const BatchLimit = 3

type Row struct {
	Issue     types.LinearIssue
	Session   *types.AgentSession
	Status    types.AgentStatus
	Started   bool
	Duplicate bool
}

func Configured() bool {
	return os.Getenv("LINEAR_API_KEY") != ""
}

func Rows(presetName string, limit int) ([]Row, types.QueuePreset, error) {
	cfg, err := config.LoadOrDefault()
	if err != nil {
		return nil, types.QueuePreset{}, err
	}
	preset, ok := config.Preset(cfg, presetName)
	if !ok {
		return nil, types.QueuePreset{}, os.ErrNotExist
	}
	outputLimit := preset.Limit
	if outputLimit <= 0 {
		outputLimit = 8
	}
	if limit > 0 {
		outputLimit = limit
	}
	fetchLimit := outputLimit
	if fetchLimit < 250 {
		fetchLimit = 250
	}
	if fetchLimit > 250 {
		fetchLimit = 250
	}
	preset.Limit = fetchLimit
	issues, err := linear.ListIssues(preset)
	if err != nil {
		return nil, preset, err
	}
	sessions, _ := state.List()
	rows := JoinWithPreset(issues, sessions, preset)
	if outputLimit > 0 && len(rows) > outputLimit {
		rows = rows[:outputLimit]
	}
	preset.Limit = outputLimit
	return rows, preset, nil
}

func Join(issues []types.LinearIssue, sessions []types.AgentSession) []Row {
	return JoinWithPreset(issues, sessions, types.QueuePreset{})
}

func JoinWithPreset(issues []types.LinearIssue, sessions []types.AgentSession, preset types.QueuePreset) []Row {
	byIssueID := map[string]*types.AgentSession{}
	byIdentifier := map[string]*types.AgentSession{}
	for i := range sessions {
		s := sessions[i]
		if s.Linear.IssueID != "" {
			byIssueID[s.Linear.IssueID] = &sessions[i]
		}
		if s.Linear.Identifier != "" {
			byIdentifier[strings.ToUpper(s.Linear.Identifier)] = &sessions[i]
		}
		if s.ID != "" {
			byIdentifier[strings.ToUpper(s.ID)] = &sessions[i]
		}
	}
	rows := make([]Row, 0, len(issues))
	for _, issue := range issues {
		var session *types.AgentSession
		if issue.ID != "" {
			session = byIssueID[issue.ID]
		}
		if session == nil && issue.Identifier != "" {
			session = byIdentifier[strings.ToUpper(issue.Identifier)]
		}
		status := types.AgentStatus("queued")
		started := false
		if session != nil {
			status = session.Status
			started = true
		}
		rows = append(rows, Row{Issue: issue, Session: session, Status: status, Started: started, Duplicate: started})
	}
	return rows
}
