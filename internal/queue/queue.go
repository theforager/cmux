package queue

import (
	"os"
	"sort"
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
	if len(preset.States) == 0 {
		preset.States = defaultStateIDs()
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
			if _, exists := byIssueID[s.Linear.IssueID]; !exists {
				byIssueID[s.Linear.IssueID] = &sessions[i]
			}
		}
		if s.Linear.Identifier != "" {
			key := strings.ToUpper(s.Linear.Identifier)
			if _, exists := byIdentifier[key]; !exists {
				byIdentifier[key] = &sessions[i]
			}
		}
		if s.ID != "" {
			key := strings.ToUpper(s.ID)
			if _, exists := byIdentifier[key]; !exists {
				byIdentifier[key] = &sessions[i]
			}
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
		status := types.AgentStatus(linearStateBadge(issue.State))
		started := false
		if session != nil {
			status = session.Status
			started = true
		}
		rows = append(rows, Row{Issue: issue, Session: session, Status: status, Started: started, Duplicate: started})
	}
	sortRows(rows, preset)
	return rows
}

func sortRows(rows []Row, preset types.QueuePreset) {
	if len(preset.States) == 0 {
		return
	}
	stateRank := map[string]int{}
	for i, state := range preset.States {
		stateRank[state] = i
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, okI := stateRank[rows[i].Issue.StateID]
		rj, okJ := stateRank[rows[j].Issue.StateID]
		if okI && okJ && ri != rj {
			return ri < rj
		}
		if okI != okJ {
			return okI
		}
		if rows[i].Issue.SortOrder != rows[j].Issue.SortOrder {
			return rows[i].Issue.SortOrder < rows[j].Issue.SortOrder
		}
		return rows[i].Issue.Identifier < rows[j].Issue.Identifier
	})
}

func defaultStateIDs() []string {
	states, err := linear.ListWorkflowStates()
	if err != nil {
		return nil
	}
	wanted := []string{"in progress", "todo", "to do", "scoping", "backlog"}
	out := []string{}
	seen := map[string]bool{}
	for _, want := range wanted {
		for _, state := range states {
			if seen[state.ID] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(state.Name), want) {
				out = append(out, state.ID)
				seen[state.ID] = true
			}
		}
	}
	return out
}

func linearStateBadge(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	state = strings.ReplaceAll(state, " ", "-")
	if state == "" {
		return "linear"
	}
	return state
}
