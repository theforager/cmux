package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/runbook"
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

func TestScopedDescriptionBlockUsesRunbookHandoff(t *testing.T) {
	block := scopedDescriptionBlock(types.AgentSession{}, "Build the parser path.", []runbook.Section{
		{Heading: "Goal", Body: "Original issue title."},
		{Heading: "Current state", Body: "- Parser entrypoint identified.\n- Types are already available."},
		{Heading: "Decisions made", Body: "- Keep existing token model."},
		{Heading: "Proposed implementation", Body: "1. Add parser test.\n2. Implement parser."},
		{Heading: "Next coding steps", Body: "- Implement parser tests first."},
		{Heading: "Blockers", Body: "- None."},
		{Heading: "Review summary", Body: "Not needed for handoff."},
	})
	for _, want := range []string{"cmux scoped handoff", "Build the parser path.", "Parser entrypoint identified.", "Types are already available.", "Keep existing token model.", "Add parser test.", "Implement parser tests first."} {
		if !strings.Contains(block, want) {
			t.Fatalf("scoped block missing %q:\n%s", want, block)
		}
	}
	for _, unwanted := range []string{"Original issue title", "None", "Not needed for handoff"} {
		if strings.Contains(block, unwanted) {
			t.Fatalf("scoped block should omit %q:\n%s", unwanted, block)
		}
	}
}

func TestReplaceScopedBlockReplacesExistingBlock(t *testing.T) {
	description := "Original\n\n" + scopedStartMarker + "\nold\n" + scopedEndMarker
	got := replaceScopedBlock(description, "new")
	if !strings.Contains(got, "Original") || !strings.Contains(got, "new") {
		t.Fatalf("description not preserved/replaced:\n%s", got)
	}
	if strings.Contains(got, "old") {
		t.Fatalf("old scoped block remained:\n%s", got)
	}
}
