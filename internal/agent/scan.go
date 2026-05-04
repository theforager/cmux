package agent

import (
	"strings"
	"time"

	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/runbook"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"
)

const (
	waitingAfter = 10 * time.Minute
	staleAfter   = 6 * time.Hour
)

type ScanResult struct {
	Scanned int
	Updated int
	Crashed int
	Waiting int
	Stale   int
}

func Scan() (ScanResult, error) {
	sessions, err := state.List()
	if err != nil {
		return ScanResult{}, err
	}
	var result ScanResult
	for _, s := range sessions {
		result.Scanned++
		next := scanSession(s, time.Now())
		if next.Status != s.Status || next.Runtime != s.Runtime || next.LastSummary != s.LastSummary {
			result.Updated++
			switch next.Status {
			case types.StatusCrashed:
				result.Crashed++
			case types.StatusWaiting:
				result.Waiting++
			case types.StatusStale:
				result.Stale++
			}
		}
		if err := state.Write(next); err != nil {
			return result, err
		}
	}
	return result, nil
}

func scanSession(s types.AgentSession, now time.Time) types.AgentSession {
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
	notes := runbook.Read(s.ID)
	if preview := notes.Preview(); preview != "" {
		s.LastSummary = preview
	}
	s.Runtime = runtime
	s.Status = ClassifyRuntimeStatus(s.Status, runtime, now)
	s.NeedsHuman = s.Status == types.StatusBlocked || s.Status == types.StatusTestsFailed || s.Status == types.StatusReadyForReview || s.Status == types.StatusWaiting || s.Status == types.StatusCrashed
	s.LastUpdatedAt = format.Now()
	return s
}

func ClassifyRuntimeStatus(current types.AgentStatus, runtime types.RuntimeData, now time.Time) types.AgentStatus {
	if runtime.TmuxAlive && runtime.PaneDead {
		return types.StatusCrashed
	}
	if !runtime.TmuxAlive {
		if current == types.StatusIdle || isManualStatus(current) {
			return current
		}
		return types.StatusCrashed
	}
	if isManualStatus(current) {
		return current
	}
	lastActivity := parseRFC3339(runtime.LastActivityAt)
	if lastActivity.IsZero() {
		return current
	}
	idle := now.Sub(lastActivity)
	if idle >= staleAfter {
		return types.StatusStale
	}
	if idle >= waitingAfter && looksLikePrompt(runtime.CurrentCommand, runtime.Preview) {
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

func looksLikePrompt(command, preview string) bool {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "zsh" || command == "bash" || command == "fish" || command == "sh" {
		return true
	}
	lines := strings.Split(strings.TrimSpace(preview), "\n")
	if len(lines) == 0 {
		return false
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	return strings.HasPrefix(last, ">") || strings.HasSuffix(last, "$") || strings.HasSuffix(last, "%") || strings.HasSuffix(last, "#")
}

func parseRFC3339(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
