package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/process"
	"github.com/theforager/cmux/internal/runbook"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"
)

type StartOptions struct {
	Cwd         string
	IssueKey    string
	Title       string
	Scratch     bool
	Worktree    bool
	NoWorktree  bool
	PrepareOnly bool
	Agent       string
}

func Provider(command string) string {
	first := firstWord(command)
	switch first {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	default:
		return "custom"
	}
}

func Start(o StartOptions) (types.AgentSession, error) {
	if o.Cwd == "" {
		o.Cwd = "."
	}
	abs, _ := filepath.Abs(o.Cwd)
	o.Cwd = abs
	if o.Agent == "" {
		o.Agent = os.Getenv("CMUX_AGENT")
	}
	if o.Agent == "" {
		o.Agent = "claude"
	}
	if !process.Exists(firstWord(o.Agent)) {
		return types.AgentSession{}, fmt.Errorf("agent command not found: %s", firstWord(o.Agent))
	}
	if o.Scratch {
		return startScratch(o)
	}
	if o.IssueKey != "" {
		return startIssue(o)
	}
	if o.Title == "" {
		return types.AgentSession{}, fmt.Errorf("use --title for task-backed sessions")
	}
	return startTask(o)
}

func startIssue(o StartOptions) (types.AgentSession, error) {
	issue, err := linear.Issue(o.IssueKey)
	if err != nil {
		return types.AgentSession{}, err
	}
	if existing, err := state.Read(issue.Identifier); err == nil {
		return existing, nil
	}
	repo := o.Cwd
	worktree := o.Cwd
	branch := ""
	if !o.NoWorktree {
		repo, branch, worktree, err = gitx.EnsureWorktree(o.Cwd, issue.Identifier, issue.Title, issue.BranchName)
		if err != nil {
			return types.AgentSession{}, err
		}
	}
	name, err := tmux.GenerateSessionName(worktree, issue.Identifier)
	if err != nil {
		return types.AgentSession{}, err
	}
	now := format.Now()
	s := types.AgentSession{SchemaVersion: 1, ID: issue.Identifier, Type: types.TypeIssueBacked, Title: issue.Title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: repo, WorktreePath: worktree, Branch: branch, Linear: types.LinearData{IssueID: issue.ID, Identifier: issue.Identifier, URL: issue.URL, State: issue.State}, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	if o.PrepareOnly {
		s.Status = types.StatusIdle
	}
	if err := ensureRunbook(s, issue); err != nil {
		return s, err
	}
	if err := state.Write(s); err != nil {
		return s, err
	}
	if !o.PrepareOnly {
		if err := launch(s, issue); err != nil {
			return s, err
		}
	}
	_ = syncLinear(&s)
	_ = state.Write(s)
	return s, nil
}

func startTask(o StartOptions) (types.AgentSession, error) {
	id := "task-" + format.Slug(o.Title)
	if existing, err := state.Read(id); err == nil {
		return existing, nil
	}
	repo := o.Cwd
	worktree := o.Cwd
	branch := ""
	var err error
	if o.Worktree {
		repo, branch, worktree, err = gitx.EnsureWorktree(o.Cwd, id, o.Title, "")
		if err != nil {
			return types.AgentSession{}, err
		}
	}
	name, err := tmux.GenerateSessionName(worktree, id)
	if err != nil {
		return types.AgentSession{}, err
	}
	now := format.Now()
	s := types.AgentSession{SchemaVersion: 1, ID: id, Type: types.TypeTaskBacked, Title: o.Title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: repo, WorktreePath: worktree, Branch: branch, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	if o.PrepareOnly {
		s.Status = types.StatusIdle
	}
	if err := ensureRunbook(s, types.LinearIssue{}); err != nil {
		return s, err
	}
	if err := state.Write(s); err != nil {
		return s, err
	}
	if !o.PrepareOnly {
		if err := launch(s, types.LinearIssue{}); err != nil {
			return s, err
		}
	}
	return s, nil
}

func startScratch(o StartOptions) (types.AgentSession, error) {
	id, err := state.NextScratchID()
	if err != nil {
		return types.AgentSession{}, err
	}
	title := o.Title
	if title == "" {
		title = "Scratch session"
	}
	name, err := tmux.GenerateSessionName(o.Cwd, id)
	if err != nil {
		return types.AgentSession{}, err
	}
	now := format.Now()
	s := types.AgentSession{SchemaVersion: 1, ID: id, Type: types.TypeScratch, Title: title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: o.Cwd, WorktreePath: o.Cwd, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	if err := state.Write(s); err != nil {
		return s, err
	}
	return s, tmux.Create(tmux.CreateOptions{Name: name, Dir: o.Cwd, Command: o.Agent, Title: title, Agent: s.Provider})
}

func launch(s types.AgentSession, issue types.LinearIssue) error {
	if err := tmux.Create(tmux.CreateOptions{Name: s.TmuxSession, Dir: s.WorktreePath, Command: s.AgentCommand, Title: s.Title, Agent: s.Provider}); err != nil {
		return err
	}
	if s.Type != types.TypeScratch {
		// Let the agent finish drawing before sending the initial prompt.
		timeSleep()
		tmp, err := os.CreateTemp("", "cmux-agent-prompt-*")
		if err == nil {
			_, _ = tmp.WriteString(initialPrompt(s, issue))
			_ = tmp.Close()
			_, _ = process.Run("tmux", "load-buffer", "-b", "cmux-agent-prompt", tmp.Name())
			_, _ = process.Run("tmux", "paste-buffer", "-t", s.TmuxSession, "-b", "cmux-agent-prompt")
			timeSleep()
			_, _ = process.Run("tmux", "send-keys", "-t", s.TmuxSession, "C-m")
			_ = os.Remove(tmp.Name())
		}
	}
	return nil
}

func SetStatus(id string, status types.AgentStatus, summary string) (types.AgentSession, error) {
	s, err := state.Update(id, func(s types.AgentSession) types.AgentSession {
		s.Status = status
		s.LastSummary = summary
		s.NeedsHuman = status == types.StatusBlocked || status == types.StatusTestsFailed || status == types.StatusReadyForReview
		return s
	})
	if err != nil {
		return s, err
	}
	_ = syncLinear(&s)
	_ = state.Write(s)
	return s, nil
}

func Kill(id string) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	if err := tmux.KillIfExists(s.TmuxSession); err != nil {
		return err
	}
	_, err = SetStatus(id, types.StatusDone, "Session killed from cmux")
	return err
}

func Delete(id string) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	if err := tmux.KillIfExists(s.TmuxSession); err != nil {
		return err
	}
	return state.Delete(id)
}

type WorktreeDirtyError struct {
	Path    string
	Summary string
}

func (e WorktreeDirtyError) Error() string {
	return "worktree has uncommitted changes: " + e.Path + "\n" + e.Summary
}

func Restart(id string) (types.AgentSession, error) {
	s, err := state.Read(id)
	if err != nil {
		return s, err
	}
	if tmux.Has(s.TmuxSession) {
		return s, nil
	}
	name, err := tmux.GenerateSessionName(valueOr(s.WorktreePath, s.RepoPath), s.ID)
	if err != nil {
		return s, err
	}
	s.TmuxSession = name
	s.Status = types.StatusRunning
	s.LastUpdatedAt = format.Now()
	if err := state.Write(s); err != nil {
		return s, err
	}
	issue := types.LinearIssue{Identifier: s.Linear.Identifier, Title: s.Title, Description: "Resume from the cmux runbook.", URL: s.Linear.URL, BranchName: s.Branch, State: s.Linear.State}
	if err := launch(s, issue); err != nil {
		return s, err
	}
	return s, state.Write(s)
}

func CleanupWorktree(id string, force bool) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	if !hasSeparateWorktree(s) {
		return fmt.Errorf("session has no separate worktree")
	}
	if !worktreeOwnedByCmux(s) && !force {
		return fmt.Errorf("worktree is outside cmux worktrees; refusing cleanup without force: %s", s.WorktreePath)
	}
	if other, ok := otherSessionUsingWorktree(s); ok {
		return fmt.Errorf("worktree is also used by session %s", other)
	}
	dirty, summary := gitx.StatusSummary(s.WorktreePath)
	if dirty && !force {
		return WorktreeDirtyError{Path: s.WorktreePath, Summary: summary}
	}
	if !gitx.WorktreeListed(s.RepoPath, s.WorktreePath) && !force {
		return fmt.Errorf("git does not list this worktree: %s", s.WorktreePath)
	}
	if err := tmux.KillIfExists(s.TmuxSession); err != nil {
		return err
	}
	if err := gitx.RemoveWorktree(s.RepoPath, s.WorktreePath, force); err != nil {
		return err
	}
	return state.Delete(id)
}

func ResetWorkspace(id string) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	if !hasSeparateWorktree(s) {
		return fmt.Errorf("session has no separate worktree")
	}
	if !worktreeOwnedByCmux(s) {
		return fmt.Errorf("workspace is outside cmux worktrees; refusing reset: %s", s.WorktreePath)
	}
	return gitx.ResetHardClean(s.WorktreePath)
}

func hasSeparateWorktree(s types.AgentSession) bool {
	return strings.TrimSpace(s.WorktreePath) != "" && s.WorktreePath != s.RepoPath
}

func worktreeOwnedByCmux(s types.AgentSession) bool {
	rel, err := filepath.Rel(home.WorktreesDir(), s.WorktreePath)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func otherSessionUsingWorktree(s types.AgentSession) (string, bool) {
	sessions, err := state.List()
	if err != nil {
		return "", false
	}
	for _, other := range sessions {
		if other.ID != s.ID && other.WorktreePath == s.WorktreePath {
			return other.ID, true
		}
	}
	return "", false
}

func syncLinear(s *types.AgentSession) error {
	if s.Linear.IssueID == "" {
		return nil
	}
	notes := runbook.Read(s.ID)
	body := fmt.Sprintf("cmux session\n\nStatus: %s\nBranch: %s\nWorkspace: %s\nRunbook: %s\ntmux: %s", s.Status, valueOr(s.Branch, "current"), valueOr(s.WorktreePath, s.RepoPath), home.RunbookPath(s.ID), s.TmuxSession)
	if s.LastSummary != "" {
		body += "\nLast summary: " + s.LastSummary
	}
	if notes.CurrentState != "" {
		body += "\n\nCurrent state:\n" + notes.CurrentState
	}
	if notes.NextAction != "" {
		body += "\n\nNext action:\n" + notes.NextAction
	}
	if notes.Blockers != "" {
		body += "\n\nBlockers:\n" + notes.Blockers
	}
	if notes.TestsRun != "" {
		body += "\n\nTests run:\n" + notes.TestsRun
	}
	if notes.ReviewSummary != "" {
		body += "\n\nReview summary:\n" + notes.ReviewSummary
	}
	commentID, err := linear.UpsertComment(s.Linear.IssueID, s.Linear.CommentID, body)
	if err != nil {
		s.Linear.LastSyncError = err.Error()
		return err
	}
	if commentID != "" {
		s.Linear.CommentID = commentID
	}
	s.Linear.LastSyncAt = format.Now()
	s.Linear.LastSyncError = ""
	return nil
}

func ensureRunbook(s types.AgentSession, issue types.LinearIssue) error {
	dir := home.SessionDir(s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := home.RunbookPath(s.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		goal := s.Title
		if issue.Identifier != "" {
			goal = issue.Identifier + ": " + issue.Title
		}
		content := "# Agent Runbook\n\n## Goal\n" + goal + "\n\n## Current state\nSession created by cmux.\n\n## Decisions made\n- None yet.\n\n## Blockers\n- None.\n\n## Tests run\n- Not run yet.\n\n## Next action\n- Start implementation.\n\n## Review summary\n- Not ready for review yet.\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func initialPrompt(s types.AgentSession, issue types.LinearIssue) string {
	text := "You are working in a cmux managed " + string(s.Type) + " session.\n\n"
	if issue.Identifier != "" {
		text += "Linear issue: " + issue.Identifier + " - " + issue.Title + "\n"
	} else {
		text += "Task: " + s.Title + "\n"
	}
	if s.Branch != "" {
		text += "Branch: " + s.Branch + "\n"
	}
	text += "Workspace: " + valueOr(s.WorktreePath, s.RepoPath) + "\n"
	text += "Runbook: " + home.RunbookPath(s.ID) + "\n\n"
	text += "Requirements:\n- Work only in this workspace unless the user explicitly says otherwise.\n- Keep the cmux runbook updated at " + home.RunbookPath(s.ID) + ".\n- When blocked, run: cmux agent status " + s.ID + " blocked \"<reason>\"\n- When ready for review, run: cmux agent status " + s.ID + " ready_for_review \"<summary>\"\n"
	if issue.Description != "" {
		text += "\nIssue description:\n" + issue.Description
	}
	return text
}

func firstWord(command string) string {
	for i, r := range command {
		if r == ' ' || r == '\t' {
			return command[:i]
		}
	}
	return command
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func timeSleep() {
	// Small delay avoids pasting the prompt before Claude/Codex has entered raw mode.
	<-time.After(900 * time.Millisecond)
}
