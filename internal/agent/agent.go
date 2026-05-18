package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/theforager/cmux/internal/brief"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/process"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"
)

type StartOptions struct {
	Cwd         string
	IssueKey    string
	Title       string
	Scratch     bool
	Fresh       bool
	Profile     types.AgentProfile
	Worktree    bool
	NoWorktree  bool
	PrepareOnly bool
	Agent       string
}

func Provider(command string) string {
	first := strings.Trim(filepath.Base(firstWord(command)), `"'`)
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
	sessionID := issue.Identifier
	profile := normalizeProfile(o.Profile)
	if o.Fresh {
		sessionID = issue.Identifier + "-fresh"
	}
	if existing, err := state.Read(sessionID); err == nil {
		if strings.TrimSpace(o.Agent) != "" {
			existing.AgentCommand = o.Agent
			existing.Provider = Provider(o.Agent)
		}
		existing.Profile = profile
		existing.Brief.Kind = briefKindForProfile(profile)
		existing.Brief.SourcePath = home.BriefPath(existing.ID)
		existing.Title = issue.Title
		existing.Linear.IssueID = issue.ID
		existing.Linear.Identifier = issue.Identifier
		existing.Linear.URL = issue.URL
		existing.Linear.State = issue.State
		existing.Linear.StateID = issue.StateID
		if issue.BranchName != "" {
			existing.Branch = issue.BranchName
		}
		existing = normalizeExistingSession(existing)
		if o.PrepareOnly || sessionAlive(existing.TmuxSession) {
			_ = state.Write(existing)
			return existing, nil
		}
		return restartSession(existing, issue)
	}
	repo := o.Cwd
	worktree := o.Cwd
	branch := ""
	if !o.NoWorktree {
		repo, branch, worktree, err = gitx.EnsureWorktree(o.Cwd, sessionID, issue.Title, issue.BranchName)
		if err != nil {
			return types.AgentSession{}, err
		}
	}
	name, err := tmux.GenerateSessionName(worktree, sessionID)
	if err != nil {
		return types.AgentSession{}, err
	}
	now := format.Now()
	s := types.AgentSession{SchemaVersion: 1, ID: sessionID, Type: types.TypeIssueBacked, Title: issue.Title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: repo, WorktreePath: worktree, Branch: branch, Linear: types.LinearData{IssueID: issue.ID, Identifier: issue.Identifier, URL: issue.URL, State: issue.State, StateID: issue.StateID, OriginalState: issue.State, OriginalStateID: issue.StateID}, Profile: profile, Brief: types.BriefData{Kind: briefKindForProfile(profile), SourcePath: home.BriefPath(sessionID)}, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	s = normalizeAgentCommandForSession(s)
	if o.PrepareOnly {
		s.Status = types.StatusIdle
	}
	if err := ensureBrief(s, issue); err != nil {
		return s, err
	}
	if err := state.Write(s); err != nil {
		return s, err
	}
	if !o.PrepareOnly {
		if err := launch(s, issue); err != nil {
			s.Status = types.StatusCrashed
			s.LastSummary = err.Error()
			_ = state.Write(s)
			return s, err
		}
	}
	_ = state.Write(s)
	return s, nil
}

func startTask(o StartOptions) (types.AgentSession, error) {
	id := "task-" + format.Slug(o.Title)
	if existing, err := state.Read(id); err == nil {
		existing = normalizeExistingSession(existing)
		_ = state.Write(existing)
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
	profile := normalizeProfile(o.Profile)
	s := types.AgentSession{SchemaVersion: 1, ID: id, Type: types.TypeTaskBacked, Title: o.Title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: repo, WorktreePath: worktree, Branch: branch, Profile: profile, Brief: types.BriefData{Kind: briefKindForProfile(profile), SourcePath: home.BriefPath(id)}, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	s = normalizeAgentCommandForSession(s)
	if o.PrepareOnly {
		s.Status = types.StatusIdle
	}
	if err := ensureBrief(s, types.LinearIssue{}); err != nil {
		return s, err
	}
	if err := state.Write(s); err != nil {
		return s, err
	}
	if !o.PrepareOnly {
		if err := launch(s, types.LinearIssue{}); err != nil {
			s.Status = types.StatusCrashed
			s.LastSummary = err.Error()
			_ = state.Write(s)
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
	profile := normalizeProfile(o.Profile)
	s := types.AgentSession{SchemaVersion: 1, ID: id, Type: types.TypeScratch, Title: title, Provider: Provider(o.Agent), AgentCommand: o.Agent, TmuxSession: name, RepoPath: o.Cwd, WorktreePath: o.Cwd, Profile: profile, Brief: types.BriefData{Kind: briefKindForProfile(profile), SourcePath: home.BriefPath(id)}, Status: types.StatusRunning, CreatedAt: now, LastUpdatedAt: now}
	s = normalizeAgentCommandForSession(s)
	if err := ensureBrief(s, types.LinearIssue{}); err != nil {
		return s, err
	}
	if err := state.Write(s); err != nil {
		return s, err
	}
	if err := launch(s, types.LinearIssue{}); err != nil {
		s.Status = types.StatusCrashed
		s.LastSummary = err.Error()
		_ = state.Write(s)
		return s, err
	}
	return s, nil
}

func launch(s types.AgentSession, issue types.LinearIssue) error {
	if err := tmux.Create(tmux.CreateOptions{Name: s.TmuxSession, Dir: s.WorktreePath, Command: s.AgentCommand, Title: s.Title, Agent: s.Provider}); err != nil {
		return err
	}
	if err := ensureAgentRunning(s); err != nil {
		_ = tmux.KillIfExists(s.TmuxSession)
		return err
	}
	if s.Type != types.TypeScratch && s.Profile != types.ProfileCustom {
		// Let the agent finish drawing before sending the initial prompt.
		timeSleep()
		tmp, err := os.CreateTemp("", "cmux-agent-prompt-*")
		if err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
		_, writeErr := tmp.WriteString(initialPrompt(s, issue))
		closeErr := tmp.Close()
		defer os.Remove(tmp.Name())
		if writeErr != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return writeErr
		}
		if closeErr != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return closeErr
		}
		if _, err := process.Run("tmux", "load-buffer", "-b", "cmux-agent-prompt", tmp.Name()); err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
		if _, err := process.Run("tmux", "paste-buffer", "-t", s.TmuxSession, "-b", "cmux-agent-prompt"); err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
		timeSleep()
		if err := ensureSessionAlive(s); err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
		if _, err := process.Run("tmux", "send-keys", "-t", s.TmuxSession, "C-m"); err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
		if err := ensureAgentRunning(s); err != nil {
			_ = tmux.KillIfExists(s.TmuxSession)
			return err
		}
	}
	if err := ensureSessionAlive(s); err != nil {
		_ = tmux.KillIfExists(s.TmuxSession)
		return err
	}
	return nil
}

func ensureSessionAlive(s types.AgentSession) error {
	info, err := tmux.Inspect(s.TmuxSession)
	if err != nil {
		return err
	}
	if !info.Alive {
		return fmt.Errorf("agent session exited during startup: %s", s.TmuxSession)
	}
	if info.PaneDead {
		return fmt.Errorf("agent pane exited during startup: %s exit=%s", s.TmuxSession, valueOr(info.ExitStatus, "unknown"))
	}
	return nil
}

func ensureAgentRunning(s types.AgentSession) error {
	var last tmux.PaneInfo
	for i := 0; i < 20; i++ {
		info, err := tmux.Inspect(s.TmuxSession)
		if err != nil {
			return err
		}
		last = info
		if !info.Alive || info.PaneDead {
			return agentStartupError(s, info)
		}
		if !looksLikeShell(info.CurrentCommand) && strings.TrimSpace(info.CurrentCommand) != "" {
			return nil
		}
		<-time.After(200 * time.Millisecond)
	}
	return agentStartupError(s, last)
}

func agentStartupError(s types.AgentSession, info tmux.PaneInfo) error {
	preview := strings.TrimSpace(tmux.Capture(s.TmuxSession, 8))
	detail := strings.TrimSpace(info.CurrentCommand)
	if detail == "" {
		detail = "unknown"
	}
	msg := fmt.Sprintf("agent command did not stay running: %s (current command: %s)", firstWord(s.AgentCommand), detail)
	if info.PaneDead {
		msg = fmt.Sprintf("agent pane exited during startup: %s exit=%s", s.TmuxSession, valueOr(info.ExitStatus, "unknown"))
	}
	if preview != "" {
		msg += "\n" + preview
	}
	return fmt.Errorf("%s", msg)
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
	_, err = SetStatus(id, types.StatusStopped, "Agent stopped from cmux")
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

func Close(id string) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	if err := ensureCleanWorkspace(s); err != nil {
		return err
	}
	if s.Linear.IssueID != "" {
		switch state := brief.State(s); state {
		case "not published", "changed since publish", "publish failed":
			return fmt.Errorf("brief %s; publish brief or forget session to keep local state", state)
		}
	}
	if hasSeparateWorktree(s) {
		if !worktreeOwnedByCmux(s) {
			return fmt.Errorf("worktree is outside cmux worktrees; use Forget session to keep it: %s", s.WorktreePath)
		}
		if other, ok := otherSessionUsingWorktree(s); ok {
			return fmt.Errorf("worktree is also used by session %s", other)
		}
		if !gitx.WorktreeListed(s.RepoPath, s.WorktreePath) {
			return fmt.Errorf("git does not list this worktree: %s", s.WorktreePath)
		}
	}
	if err := tmux.KillIfExists(s.TmuxSession); err != nil {
		return err
	}
	if hasSeparateWorktree(s) {
		if err := gitx.RemoveWorktree(s.RepoPath, s.WorktreePath, false); err != nil {
			return err
		}
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

func ensureCleanWorkspace(s types.AgentSession) error {
	workspace := valueOr(s.WorktreePath, s.RepoPath)
	if workspace == "" {
		return nil
	}
	dirty, summary := gitx.StatusSummary(workspace)
	if dirty {
		return WorktreeDirtyError{Path: workspace, Summary: summary}
	}
	return nil
}

func Restart(id string) (types.AgentSession, error) {
	s, err := state.Read(id)
	if err != nil {
		return s, err
	}
	s = normalizeExistingSession(s)
	issue := restartIssueContext(s)
	if err := ensureBrief(s, issue); err != nil {
		return s, err
	}
	_ = tmux.KillIfExists(s.TmuxSession)
	return restartSession(s, issue)
}

func Fresh(id, agentCommand string, profile types.AgentProfile) (types.AgentSession, error) {
	s, err := state.Read(id)
	if err != nil {
		return s, err
	}
	s = normalizeExistingSession(s)
	if strings.TrimSpace(agentCommand) != "" {
		s.AgentCommand = agentCommand
		s.Provider = Provider(agentCommand)
	}
	if profile != "" {
		s.Profile = normalizeProfile(profile)
		s.Brief.Kind = briefKindForProfile(s.Profile)
	}
	s = normalizeAgentCommandForSession(s)
	issue := restartIssueContext(s)
	if err := ensureBrief(s, issue); err != nil {
		return s, err
	}
	_ = tmux.KillIfExists(s.TmuxSession)
	return restartSession(s, issue)
}

func restartSession(s types.AgentSession, issue types.LinearIssue) (types.AgentSession, error) {
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
	if err := launch(s, issue); err != nil {
		return s, err
	}
	return s, state.Write(s)
}

func sessionAlive(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	info, err := tmux.Inspect(name)
	return err == nil && info.Alive && !info.PaneDead
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

func PreviewBrief(id string) (string, error) {
	return brief.Render(id)
}

func PublishBrief(id string) (types.AgentSession, string, error) {
	s, err := state.Read(id)
	if err != nil {
		return s, "", err
	}
	if s.Linear.IssueID == "" {
		return s, "", fmt.Errorf("session is not backed by a Linear issue")
	}
	rendered, err := brief.Render(s.ID)
	if err != nil {
		return s, "", err
	}
	hash := brief.Hash(rendered)
	body := brief.WrapPublishedBlock(s.ID, s.Brief.Kind, hash, rendered)
	commentID, err := linear.UpsertComment(s.Linear.IssueID, s.Linear.CommentID, body)
	if err != nil {
		s.Brief.LastPublishError = err.Error()
		s.Brief.LastSyncError = err.Error()
		_ = state.Write(s)
		return s, "", err
	}
	if commentID != "" {
		s.Linear.CommentID = commentID
	}
	now := format.Now()
	s.Brief.PublishedHash = hash
	s.Brief.PublishedAt = now
	s.Brief.PublishedLinearIssueID = s.Linear.IssueID
	s.Brief.PublishedBlockVersion = "comment"
	s.Brief.LastPublishError = ""
	s.Brief.LastSyncAt = now
	s.Brief.LastSyncError = ""
	if head, err := gitx.Head(valueOr(s.WorktreePath, s.RepoPath)); err == nil {
		s.Brief.PublishedGitHead = head
	}
	s.Brief.PublishedBranch = s.Branch
	if diffHash, err := gitx.DiffHash(valueOr(s.WorktreePath, s.RepoPath)); err == nil {
		s.Brief.PublishedGitDiffHash = diffHash
	}
	return s, hash, state.Write(s)
}

func MoveLinear(id, stateID string) (types.AgentSession, error) {
	s, err := state.Read(id)
	if err != nil {
		return s, err
	}
	if s.Linear.IssueID == "" {
		return s, fmt.Errorf("session is not backed by a Linear issue")
	}
	teamID := ""
	if issue, err := linear.Issue(valueOr(s.Linear.Identifier, s.Linear.IssueID)); err == nil {
		teamID = issue.TeamID
	}
	resolvedStateID, err := linear.ResolveWorkflowStateID(stateID, teamID)
	if err != nil {
		s.Linear.LastSyncError = err.Error()
		_ = state.Write(s)
		return s, err
	}
	updated, err := linear.UpdateIssue(s.Linear.IssueID, resolvedStateID, nil)
	if err != nil {
		s.Linear.LastSyncError = err.Error()
		_ = state.Write(s)
		return s, err
	}
	if updated.State != "" {
		s.Linear.State = updated.State
		s.Linear.StateID = updated.StateID
	}
	s.Linear.LastSyncAt = format.Now()
	s.Linear.LastSyncError = ""
	return s, state.Write(s)
}

func ensureBrief(s types.AgentSession, issue types.LinearIssue) error {
	dir := home.SessionDir(s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := home.BriefPath(s.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if legacy, err := os.ReadFile(legacyRunbookPath(s.ID)); err == nil && strings.TrimSpace(string(legacy)) != "" {
			if err := os.WriteFile(path, legacyBriefContent(string(legacy)), 0o644); err != nil {
				return err
			}
			return nil
		}
		goal := s.Title
		if issue.Identifier != "" {
			goal = issue.Identifier + ": " + issue.Title
		}
		content := brief.DefaultContent(goal, s.Brief.Kind)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func legacyRunbookPath(id string) string {
	return filepath.Join(home.SessionDir(id), "RUNBOOK.md")
}

func legacyBriefContent(value string) []byte {
	value = strings.TrimSpace(value)
	value = strings.Replace(value, "# Agent Runbook", "# Session Brief", 1)
	if !strings.HasPrefix(value, "# Session Brief") {
		value = "# Session Brief\n\n" + value
	}
	return []byte(value + "\n")
}

func normalizeExistingSession(s types.AgentSession) types.AgentSession {
	if s.Profile == "" {
		if s.Type == types.TypeIssueBacked {
			s.Profile = types.ProfileImplement
		} else {
			s.Profile = types.ProfileGeneral
		}
	}
	s.Profile = normalizeProfile(s.Profile)
	if s.Brief.Kind == "" {
		s.Brief.Kind = briefKindForProfile(s.Profile)
	}
	if s.Brief.SourcePath == "" {
		s.Brief.SourcePath = home.BriefPath(s.ID)
	}
	s = normalizeAgentCommandForSession(s)
	return s
}

func normalizeAgentCommandForSession(s types.AgentSession) types.AgentSession {
	detected := Provider(s.AgentCommand)
	if detected != "custom" {
		s.Provider = detected
	} else if s.Provider == "" {
		s.Provider = "custom"
	}
	s.AgentCommand = commandWithSessionAccess(s.AgentCommand, s.ID)
	return s
}

func commandWithSessionAccess(command, sessionID string) string {
	command = strings.TrimSpace(command)
	switch Provider(command) {
	case "claude", "codex":
	default:
		return command
	}
	if strings.Contains(command, "--add-dir") {
		return command
	}
	return command + " --add-dir " + shellQuote(home.SessionDir(sessionID))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("/._-:", r) {
			continue
		}
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
	return value
}

func restartIssueContext(s types.AgentSession) types.LinearIssue {
	identifier := valueOr(s.Linear.Identifier, s.Linear.IssueID)
	if identifier != "" && os.Getenv("LINEAR_API_KEY") != "" {
		if issue, err := linear.Issue(identifier); err == nil {
			return issue
		}
	}
	return types.LinearIssue{Identifier: s.Linear.Identifier, Title: s.Title, Description: "Resume from the cmux session brief.", URL: s.Linear.URL, BranchName: s.Branch, State: s.Linear.State, StateID: s.Linear.StateID}
}

func initialPrompt(s types.AgentSession, issue types.LinearIssue) string {
	text := "You are working in a cmux managed " + string(s.Type) + " session.\n\n"
	if issue.Identifier != "" {
		text += "Linear issue: " + issue.Identifier + " - " + issue.Title + "\n"
		if issue.State != "" {
			text += "Linear status: " + issue.State + "\n"
		}
	} else {
		text += "Task: " + s.Title + "\n"
	}
	if s.Branch != "" {
		text += "Branch: " + s.Branch + "\n"
	}
	text += "Workspace: " + valueOr(s.WorktreePath, s.RepoPath) + "\n"
	text += "Session brief: " + home.BriefPath(s.ID) + "\n"
	text += "Agent profile: " + string(s.Profile) + "\n\n"
	text += "Brief rules: keep it concise and user-facing. Do not mirror Linear/cmux status. Replace stale notes only when there is durable context: decisions, files or packages touched, exact tests run, real blockers, or a concrete next engineering step. Prefer 1-2 bullets per changed section and do not paste transcripts.\n\n"
	if notes := briefPromptContext(s.ID); notes != "" {
		text += "Current session brief:\n" + notes + "\n\n"
	}
	switch s.Profile {
	case types.ProfilePlan:
		text += "Profile: plan/scope. Do not implement code unless the user explicitly asks. Clarify the problem, repo/package, approach, acceptance criteria, risks, and next coding steps. Keep the session brief useful for handoff.\n\n"
	case types.ProfileDebug:
		text += "Profile: debug. Reproduce or inspect the symptom, identify findings and root cause when possible, make focused fixes only when asked, and keep verification notes in the session brief.\n\n"
	case types.ProfileReview:
		text += "Profile: review/fix. Prioritize bugs, regressions, risks, and missing tests. Keep reviewer notes and any fixes in the session brief.\n\n"
	case types.ProfileImplement:
		text += "Profile: implement. Use the Linear issue, workspace, and session brief as context. Make the requested code changes and keep summary/tests/risks/reviewer notes current.\n\n"
	default:
		text += "Profile: general. Work from the available context and keep the session brief useful for handoff.\n\n"
	}
	text += "Requirements:\n- Work only in this workspace unless the user explicitly says otherwise.\n- Keep the session brief updated at " + home.BriefPath(s.ID) + ".\n- Do not call Linear APIs directly. Use cmux commands for publishing or moving Linear status only when the user explicitly asks.\n- When blocked, run: cmux agent status " + s.ID + " blocked \"<reason>\"\n"
	if issue.Description != "" {
		text += "\nIssue description:\n" + issue.Description
	}
	return text
}

func briefPromptContext(sessionID string) string {
	sections := brief.ReadSections(sessionID)
	parts := []string{}
	for _, section := range sections {
		heading := strings.TrimSpace(section.Heading)
		if heading == "" || strings.EqualFold(heading, "Goal") {
			continue
		}
		body := brief.CleanBlock(section.Body)
		if body == "" {
			continue
		}
		parts = append(parts, "## "+heading+"\n"+body)
	}
	return truncPromptContext(strings.Join(parts, "\n\n"), 4000)
}

func truncPromptContext(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func firstWord(command string) string {
	for i, r := range command {
		if r == ' ' || r == '\t' {
			return command[:i]
		}
	}
	return command
}

func normalizeProfile(profile types.AgentProfile) types.AgentProfile {
	switch profile {
	case types.ProfilePlan, types.ProfileImplement, types.ProfileDebug, types.ProfileReview, types.ProfileCustom:
		return profile
	default:
		return types.ProfileGeneral
	}
}

func briefKindForProfile(profile types.AgentProfile) types.BriefKind {
	switch normalizeProfile(profile) {
	case types.ProfilePlan:
		return types.BriefPlan
	case types.ProfileImplement:
		return types.BriefImplementation
	case types.ProfileDebug:
		return types.BriefDebug
	case types.ProfileReview:
		return types.BriefReview
	default:
		return types.BriefGeneral
	}
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
