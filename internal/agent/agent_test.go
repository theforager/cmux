package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/runbook"
	"github.com/theforager/cmux/internal/types"
)

func TestInitialPromptIncludesImplementationContext(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	writeRunbook(t, "REB-1", "# Agent Runbook\n\n## Goal\nREB-1\n\n## Current state\n- Parser tests are failing.\n\n## Next action\n- Fix the token boundary.\n")
	writeRunbook(t, "REB-1-scope", "# Agent Runbook\n\n## Goal\nScope REB-1\n\n## Proposed plan\n- Update parser and add regression tests.\n\n## User confirmation\n- User approved the plan.\n")
	s := types.AgentSession{ID: "REB-1", Type: types.TypeIssueBacked, Title: "Fix parser", Phase: types.PhaseWork, RepoPath: "/repo", WorktreePath: "/repo/wt"}
	issue := types.LinearIssue{Identifier: "REB-1", Title: "Fix parser", Description: "Linear description"}

	got := initialPrompt(s, issue)
	for _, want := range []string{
		"Mode: implementation.",
		"Current runbook notes:",
		"Parser tests are failing.",
		"Prior scoping runbook notes:",
		"Update parser and add regression tests.",
		"Linear description",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("initial prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRunbookPromptContextDropsPlaceholders(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	writeRunbook(t, "REB-2", runbook.DefaultContent("REB-2: Empty", types.PhaseWork))

	if got := runbookPromptContext("REB-2"); got != "" {
		t.Fatalf("runbookPromptContext = %q, want empty", got)
	}
}

func writeRunbook(t *testing.T, id, content string) {
	t.Helper()
	path := home.RunbookPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
