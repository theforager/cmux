package agent

import (
	"os"
	"strings"
	"time"

	"github.com/theforager/cmux/internal/brief"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"
)

const (
	waitingAfter = 10 * time.Minute
)

type ScanResult struct {
	Scanned int
	Updated int
	Crashed int
	Waiting int
}

type ScanOptions struct {
	RefreshLinear bool
}

func Scan() (ScanResult, error) {
	return ScanWithOptions(ScanOptions{RefreshLinear: true})
}

func ScanWithOptions(o ScanOptions) (ScanResult, error) {
	sessions, err := state.List()
	if err != nil {
		return ScanResult{}, err
	}
	var result ScanResult
	for _, s := range sessions {
		result.Scanned++
		next := scanSession(s, time.Now(), o)
		if next.Status != s.Status || next.LastSummary != s.LastSummary {
			next.LastUpdatedAt = format.Now()
		}
		if next.Status != s.Status || next.Runtime != s.Runtime || next.LastSummary != s.LastSummary {
			result.Updated++
			switch next.Status {
			case types.StatusCrashed:
				result.Crashed++
			case types.StatusWaiting:
				result.Waiting++
			}
		}
		if err := state.Write(next); err != nil {
			return result, err
		}
	}
	return result, nil
}

func scanSession(s types.AgentSession, now time.Time, o ScanOptions) types.AgentSession {
	info, _ := tmux.Inspect(s.TmuxSession)
	runtime := types.RuntimeData{
		LastScannedAt:  now.UTC().Format(time.RFC3339),
		TmuxAlive:      info.Alive,
		PaneDead:       info.PaneDead,
		ExitStatus:     info.ExitStatus,
		CurrentCommand: info.CurrentCommand,
	}
	if info.LastActivityUnix > 0 {
		runtime.LastActivityAt = time.Unix(info.LastActivityUnix, 0).UTC().Format(time.RFC3339)
	}
	if info.Alive {
		runtime.Preview = tmux.Capture(s.TmuxSession, 12)
	}
	workspace := valueOr(s.WorktreePath, s.RepoPath)
	if workspace != "" {
		runtime.GitDirty, runtime.GitSummary = gitx.StatusSummary(workspace)
	}
	notes := brief.Read(s.ID)
	if preview := notes.Preview(); preview != "" {
		s.LastSummary = preview
	}
	if s.Brief.SourcePath == "" {
		s.Brief.SourcePath = home.BriefPath(s.ID)
	}
	if s.Brief.Kind == "" {
		s.Brief.Kind = types.BriefGeneral
	}
	s.Runtime = runtime
	s.Status = ClassifyRuntimeStatus(s.Status, runtime, now)
	if o.RefreshLinear {
		refreshLinearMetadata(&s)
	}
	s.NeedsHuman = s.Status == types.StatusBlocked || s.Status == types.StatusTestsFailed || s.Status == types.StatusReadyForReview || s.Status == types.StatusWaiting || s.Status == types.StatusCrashed
	return s
}

func refreshLinearMetadata(s *types.AgentSession) {
	if os.Getenv("LINEAR_API_KEY") == "" {
		return
	}
	identifier := valueOr(s.Linear.Identifier, s.Linear.IssueID)
	if identifier == "" {
		return
	}
	issue, err := linear.Issue(identifier)
	if err != nil {
		s.Linear.LastSyncError = err.Error()
		return
	}
	if issue.ID != "" {
		s.Linear.IssueID = issue.ID
	}
	if issue.Identifier != "" {
		s.Linear.Identifier = issue.Identifier
	}
	if issue.URL != "" {
		s.Linear.URL = issue.URL
	}
	if issue.State != "" {
		s.Linear.State = issue.State
		s.Linear.StateID = issue.StateID
	}
	if issue.BranchName != "" {
		s.Branch = issue.BranchName
	}
	if issue.Title != "" {
		s.Title = issue.Title
	}
	s.Linear.LastSyncAt = format.Now()
	s.Linear.LastSyncError = ""
}

func ClassifyRuntimeStatus(current types.AgentStatus, runtime types.RuntimeData, now time.Time) types.AgentStatus {
	if runtime.TmuxAlive && runtime.PaneDead {
		return types.StatusCrashed
	}
	if !runtime.TmuxAlive {
		if current == types.StatusIdle || current == types.StatusStopped || isManualStatus(current) {
			return current
		}
		return types.StatusCrashed
	}
	if isManualStatus(current) {
		return current
	}
	if looksLikeShell(runtime.CurrentCommand) {
		return types.StatusIdle
	}
	lastActivity := parseRFC3339(runtime.LastActivityAt)
	if lastActivity.IsZero() {
		return current
	}
	idle := now.Sub(lastActivity)
	if idle >= waitingAfter && looksLikeUserApprovalPrompt(runtime.Preview) {
		return types.StatusWaiting
	}
	if current == "" || current == types.StatusIdle || current == types.StatusStale || current == types.StatusCrashed || current == types.StatusWaiting {
		return types.StatusRunning
	}
	return current
}

func isManualStatus(status types.AgentStatus) bool {
	switch status {
	case types.StatusBlocked, types.StatusReadyForReview, types.StatusDone, types.StatusPROpened, types.StatusTestsFailed:
		return true
	default:
		return false
	}
}

func looksLikeShell(command string) bool {
	switch strings.TrimSpace(strings.ToLower(command)) {
	case "zsh", "bash", "fish", "sh":
		return true
	default:
		return false
	}
}

func looksLikeUserApprovalPrompt(preview string) bool {
	text := strings.ToLower(strings.TrimSpace(preview))
	if text == "" {
		return false
	}
	approval := []string{
		"permission",
		"approval",
		"approve",
		"allow",
		"authorize",
		"authorization",
		"confirm",
		"do you want to",
	}
	choice := []string{
		"yes/no",
		"y/n",
		"[y/n]",
		"(y/n)",
		"allow/deny",
		"approve/deny",
		"accept",
		"deny",
	}
	return containsAny(text, approval) && containsAny(text, choice)
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func parseRFC3339(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
