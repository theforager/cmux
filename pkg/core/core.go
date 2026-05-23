package core

import (
	"strings"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/process"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"
)

type AgentProfile string
type AgentStatus string

const (
	ProfileGeneral   AgentProfile = "general"
	ProfilePlan      AgentProfile = "plan"
	ProfileImplement AgentProfile = "implement"
	ProfileDebug     AgentProfile = "debug"
	ProfileReview    AgentProfile = "review"
	ProfileCustom    AgentProfile = "custom"
)

type StartOptions struct {
	Cwd         string
	IssueKey    string
	Title       string
	Scratch     bool
	Fresh       bool
	Profile     AgentProfile
	ProfileSet  bool
	Worktree    bool
	NoWorktree  bool
	PrepareOnly bool
	Agent       string
	AgentSet    bool
}

type SessionSummary struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Status       AgentStatus  `json:"status"`
	Profile      AgentProfile `json:"profile,omitempty"`
	Provider     string       `json:"provider,omitempty"`
	TmuxSession  string       `json:"tmuxSession"`
	RepoPath     string       `json:"repoPath,omitempty"`
	WorktreePath string       `json:"worktreePath,omitempty"`
	Branch       string       `json:"branch,omitempty"`
	LinearURL    string       `json:"linearUrl,omitempty"`
	LastSummary  string       `json:"lastSummary,omitempty"`
	Active       bool         `json:"active"`
}

type GitStatus struct {
	RepoPath string `json:"repoPath"`
	Dirty    bool   `json:"dirty"`
	Summary  string `json:"summary"`
}

type DiffFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Patch  string `json:"patch,omitempty"`
}

type ConflictFile struct {
	Path   string `json:"path"`
	Base   string `json:"base,omitempty"`
	Ours   string `json:"ours,omitempty"`
	Theirs string `json:"theirs,omitempty"`
}

func Sessions(refreshLinear bool) ([]SessionSummary, error) {
	_, _ = agent.ScanWithOptions(agent.ScanOptions{RefreshLinear: refreshLinear})
	sessions, err := state.List()
	if err != nil {
		return nil, err
	}
	tmuxSessions, _ := tmux.List()
	active := map[string]bool{}
	for _, s := range tmuxSessions {
		active[s.Name] = true
	}
	out := make([]SessionSummary, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, SummarizeSession(s, active[s.TmuxSession]))
	}
	return out, nil
}

func SummarizeSession(s types.AgentSession, active bool) SessionSummary {
	return SessionSummary{
		ID:           s.ID,
		Title:        s.Title,
		Status:       AgentStatus(s.Status),
		Profile:      AgentProfile(s.Profile),
		Provider:     s.Provider,
		TmuxSession:  s.TmuxSession,
		RepoPath:     s.RepoPath,
		WorktreePath: s.WorktreePath,
		Branch:       s.Branch,
		LinearURL:    s.Linear.URL,
		LastSummary:  s.LastSummary,
		Active:       active,
	}
}

func StartAgent(o StartOptions) (SessionSummary, error) {
	s, err := agent.Start(agent.StartOptions{
		Cwd:         o.Cwd,
		IssueKey:    o.IssueKey,
		Title:       o.Title,
		Scratch:     o.Scratch,
		Fresh:       o.Fresh,
		Profile:     types.AgentProfile(o.Profile),
		ProfileSet:  o.ProfileSet,
		Worktree:    o.Worktree,
		NoWorktree:  o.NoWorktree,
		PrepareOnly: o.PrepareOnly,
		Agent:       o.Agent,
		AgentSet:    o.AgentSet,
	})
	if err != nil {
		return SessionSummary{}, err
	}
	return SummarizeSession(s, true), nil
}

func OpenSession(id string) error {
	s, err := state.Read(id)
	if err != nil {
		return err
	}
	return tmux.AttachOrSwitch(s.TmuxSession)
}

func StopAgent(id string) error {
	return agent.Kill(id)
}

func GitStatusForPath(path string) (GitStatus, error) {
	repo, err := gitx.Root(path)
	if err != nil {
		return GitStatus{}, err
	}
	dirty, summary := gitx.StatusSummary(repo)
	return GitStatus{RepoPath: repo, Dirty: dirty, Summary: summary}, nil
}

func Diff(path string, staged bool) ([]DiffFile, error) {
	repo, err := gitx.Root(path)
	if err != nil {
		return nil, err
	}
	nameArgs := []string{"diff", "--name-status"}
	patchArgs := []string{"diff", "--"}
	if staged {
		nameArgs = []string{"diff", "--cached", "--name-status"}
		patchArgs = []string{"diff", "--cached", "--"}
	}
	names, err := process.RunDir(repo, "git", nameArgs...)
	if err != nil {
		return nil, err
	}
	var out []DiffFile
	for _, line := range strings.Split(strings.TrimSpace(names), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		file := parts[len(parts)-1]
		args := append(append([]string{}, patchArgs...), file)
		patch, _ := process.RunDir(repo, "git", args...)
		out = append(out, DiffFile{Path: file, Status: parts[0], Patch: patch})
	}
	return out, nil
}

func Conflicts(path string) ([]ConflictFile, error) {
	repo, err := gitx.Root(path)
	if err != nil {
		return nil, err
	}
	out, err := process.RunDir(repo, "git", "ls-files", "-u")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var conflicts []ConflictFile
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		file := fields[3]
		if seen[file] {
			continue
		}
		seen[file] = true
		conflicts = append(conflicts, ConflictFile{
			Path:   file,
			Base:   gitShow(repo, ":1:"+file),
			Ours:   gitShow(repo, ":2:"+file),
			Theirs: gitShow(repo, ":3:"+file),
		})
	}
	return conflicts, nil
}

func gitShow(repo, spec string) string {
	out, err := process.RunDir(repo, "git", "show", spec)
	if err != nil {
		return ""
	}
	return out
}
