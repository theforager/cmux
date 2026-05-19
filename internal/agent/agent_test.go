package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theforager/cmux/internal/brief"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/types"
)

func TestInitialPromptIncludesSessionBriefContext(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	writeBrief(t, "REB-1", "# Session Brief\n\n## Summary\n- Parser tests are failing.\n\n## Next steps\n- Fix the token boundary.\n")
	s := types.AgentSession{ID: "REB-1", Type: types.TypeIssueBacked, Title: "Fix parser", Profile: types.ProfileImplement, RepoPath: "/repo", WorktreePath: "/repo/wt"}
	issue := types.LinearIssue{Identifier: "REB-1", Title: "Fix parser", State: "Todo", Description: "Linear description"}

	got := initialPrompt(s, issue)
	for _, want := range []string{
		"Profile: implement.",
		"Session brief:",
		"Current session brief:",
		"Parser tests are failing.",
		"Linear status: Todo",
		"Do not call Linear APIs directly.",
		"Linear description",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("initial prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBriefPromptContextDropsPlaceholders(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	writeBrief(t, "REB-2", brief.DefaultContent("REB-2: Empty", types.BriefGeneral))

	if got := briefPromptContext("REB-2"); got != "" {
		t.Fatalf("briefPromptContext = %q, want empty", got)
	}
}

func TestEnsureBriefMigratesLegacyRunbook(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "REB-3"
	runbookPath := filepath.Join(home.SessionDir(id), "RUNBOOK.md")
	if err := os.MkdirAll(filepath.Dir(runbookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runbookPath, []byte("# Agent Runbook\n\n## Current state\n- Keep this context.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := types.AgentSession{ID: id, Type: types.TypeIssueBacked, Title: "Legacy", Brief: types.BriefData{Kind: types.BriefImplementation}}
	if err := ensureBrief(s, types.LinearIssue{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(home.BriefPath(id))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "# Session Brief") || !strings.Contains(string(got), "Keep this context") {
		t.Fatalf("migrated brief missing legacy context:\n%s", got)
	}
}

func TestNormalizeExistingIssueSessionDefaultsToImplementBrief(t *testing.T) {
	s := normalizeExistingSession(types.AgentSession{ID: "REB-4", Type: types.TypeIssueBacked})
	if s.Profile != types.ProfileImplement {
		t.Fatalf("profile = %s, want implement", s.Profile)
	}
	if s.Brief.Kind != types.BriefImplementation {
		t.Fatalf("brief kind = %s, want implementation", s.Brief.Kind)
	}
	if !strings.HasSuffix(s.Brief.SourcePath, "BRIEF.md") {
		t.Fatalf("brief source = %q, want BRIEF.md", s.Brief.SourcePath)
	}
}

func TestProviderRecognizesAbsoluteAgentPath(t *testing.T) {
	tests := map[string]string{
		"/opt/homebrew/bin/codex":  "codex",
		"/usr/local/bin/claude -c": "claude",
		"codex --model gpt-5":      "codex",
		"custom-agent":             "custom",
	}
	for command, want := range tests {
		if got := Provider(command); got != want {
			t.Fatalf("Provider(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestCommandWithSessionAccessAddsCodexAndClaudeSessionDir(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	for _, command := range []string{"codex", "claude --model sonnet"} {
		got := commandWithSessionAccess(command, "REB-5")
		if !strings.Contains(got, "--add-dir "+home.SessionDir("REB-5")) {
			t.Fatalf("commandWithSessionAccess(%q) = %q, want session add-dir", command, got)
		}
	}
}

func TestCommandWithSessionAccessDoesNotTouchCustomOrDuplicateAddDir(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	if got := commandWithSessionAccess("my-agent", "REB-6"); got != "my-agent" {
		t.Fatalf("custom command changed: %q", got)
	}
	command := "codex --add-dir /tmp/context"
	if got := commandWithSessionAccess(command, "REB-6"); got != command {
		t.Fatalf("command duplicated add-dir: %q", got)
	}
}

func TestStartExistingTaskDoesNotOverwriteAgentOrProfileWithDefaults(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	workspace := t.TempDir()
	existing := types.AgentSession{
		SchemaVersion: 1,
		ID:            "task-existing-task",
		Type:          types.TypeTaskBacked,
		Title:         "Existing task",
		AgentCommand:  "codex",
		Provider:      "codex",
		Profile:       types.ProfileReview,
		Brief:         types.BriefData{Kind: types.BriefReview},
		RepoPath:      workspace,
		WorktreePath:  workspace,
		TmuxSession:   "missing-session",
		Status:        types.StatusIdle,
	}
	if err := state.Write(existing); err != nil {
		t.Fatal(err)
	}
	got, err := Start(StartOptions{Cwd: workspace, Title: "Existing task", Agent: "sh", Profile: types.ProfileGeneral, PrepareOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.AgentCommand, "codex") {
		t.Fatalf("agent command = %q, want existing codex", got.AgentCommand)
	}
	if got.Profile != types.ProfileReview || got.Brief.Kind != types.BriefReview {
		t.Fatalf("profile/brief = %s/%s, want review/review", got.Profile, got.Brief.Kind)
	}
}

func TestStartExistingTaskPrepareDoesNotRequireDefaultAgent(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	workspace := t.TempDir()
	existing := types.AgentSession{
		SchemaVersion: 1,
		ID:            "task-existing-task",
		Type:          types.TypeTaskBacked,
		Title:         "Existing task",
		AgentCommand:  "sh",
		Provider:      "custom",
		RepoPath:      workspace,
		WorktreePath:  workspace,
		TmuxSession:   "missing-session",
		Status:        types.StatusIdle,
	}
	if err := state.Write(existing); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(StartOptions{Cwd: workspace, Title: "Existing task", Agent: "definitely-missing-cmux-agent", PrepareOnly: true}); err != nil {
		t.Fatalf("existing prepare should use saved session state before validating launch agent: %v", err)
	}
}

func TestStartExistingTaskAllowsExplicitAgentAndProfileOverride(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	workspace := t.TempDir()
	existing := types.AgentSession{
		SchemaVersion: 1,
		ID:            "task-existing-task",
		Type:          types.TypeTaskBacked,
		Title:         "Existing task",
		AgentCommand:  "codex",
		Provider:      "codex",
		Profile:       types.ProfileReview,
		Brief:         types.BriefData{Kind: types.BriefReview},
		RepoPath:      workspace,
		WorktreePath:  workspace,
		TmuxSession:   "missing-session",
		Status:        types.StatusIdle,
	}
	if err := state.Write(existing); err != nil {
		t.Fatal(err)
	}
	got, err := Start(StartOptions{Cwd: workspace, Title: "Existing task", Agent: "sh", AgentSet: true, Profile: types.ProfileDebug, ProfileSet: true, PrepareOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentCommand != "sh" || got.Provider != "custom" {
		t.Fatalf("agent/provider = %q/%q, want sh/custom", got.AgentCommand, got.Provider)
	}
	if got.Profile != types.ProfileDebug || got.Brief.Kind != types.BriefDebug {
		t.Fatalf("profile/brief = %s/%s, want debug/debug", got.Profile, got.Brief.Kind)
	}
}

func TestCloseRefusesLinearSessionWithMissingBrief(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	if err := state.Write(types.AgentSession{ID: "REB-10", Type: types.TypeIssueBacked, Linear: types.LinearData{IssueID: "issue-id"}, Status: types.StatusIdle}); err != nil {
		t.Fatal(err)
	}
	err := Close("REB-10")
	if err == nil || !strings.Contains(err.Error(), "brief missing") {
		t.Fatalf("Close error = %v, want missing brief refusal", err)
	}
}

func TestCleanupWorktreeRefusesExternalEvenWithForce(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	if err := state.Write(types.AgentSession{ID: "REB-11", RepoPath: "/repo", WorktreePath: "/tmp/external-worktree", TmuxSession: "missing-session"}); err != nil {
		t.Fatal(err)
	}
	err := CleanupWorktree("REB-11", true)
	if err == nil || !strings.Contains(err.Error(), "outside cmux worktrees") {
		t.Fatalf("CleanupWorktree error = %v, want external refusal", err)
	}
}

func TestCommandExecutableTrimsQuotes(t *testing.T) {
	if got := commandExecutable(`"codex" --model gpt-5`); got != "codex" {
		t.Fatalf("commandExecutable = %q, want codex", got)
	}
	if got := commandExecutable(`  codex --model gpt-5`); got != "codex" {
		t.Fatalf("commandExecutable with leading spaces = %q, want codex", got)
	}
}

func writeBrief(t *testing.T, id, content string) {
	t.Helper()
	path := home.BriefPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
