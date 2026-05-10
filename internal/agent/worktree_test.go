package agent

import (
	"path/filepath"
	"testing"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/types"
)

func TestWorktreeOwnedByCmux(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	s := types.AgentSession{RepoPath: "/repo", WorktreePath: filepath.Join(home.WorktreesDir(), "repo", "REB-1")}
	if !worktreeOwnedByCmux(s) {
		t.Fatalf("expected cmux worktree to be owned")
	}
}

func TestWorktreeOwnedByCmuxRejectsExternal(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	s := types.AgentSession{RepoPath: "/repo", WorktreePath: "/tmp/external"}
	if worktreeOwnedByCmux(s) {
		t.Fatalf("expected external worktree to be rejected")
	}
}

func TestHasSeparateWorktree(t *testing.T) {
	if hasSeparateWorktree(types.AgentSession{RepoPath: "/repo", WorktreePath: "/repo"}) {
		t.Fatalf("repo path should not be treated as separate worktree")
	}
	if !hasSeparateWorktree(types.AgentSession{RepoPath: "/repo", WorktreePath: "/worktree"}) {
		t.Fatalf("expected separate worktree")
	}
}
