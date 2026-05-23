package core

import (
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestSummarizeSession(t *testing.T) {
	s := types.AgentSession{
		ID:           "task-example",
		Title:        "Example",
		Status:       "running",
		Profile:      types.ProfileImplement,
		Provider:     "codex",
		TmuxSession:  "cmux@agent@task-example",
		RepoPath:     "/repo",
		WorktreePath: "/worktree",
		Branch:       "agent/example",
	}
	got := SummarizeSession(s, true)
	if got.ID != s.ID || !got.Active || got.Profile != ProfileImplement {
		t.Fatalf("summary = %+v", got)
	}
}
