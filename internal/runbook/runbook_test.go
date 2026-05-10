package runbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theforager/cmux/internal/home"
)

func TestCleanDropsRunbookPlaceholders(t *testing.T) {
	placeholders := []string{
		"- None.",
		"- Not started.",
		"- Not run.",
		"- Not ready.",
		"- Not confirmed.",
		"- Start implementation.",
		"- Pick the first concrete implementation step.",
	}
	for _, placeholder := range placeholders {
		if got := Clean(placeholder); got != "" {
			t.Fatalf("Clean(%q) = %q, want empty", placeholder, got)
		}
	}
}

func TestReadSectionsPreservesMarkdown(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "test-session"
	path := home.RunbookPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Agent Runbook\n\n## Current state\n- One\n- Two\n\n## Proposed implementation\n1. Build it.\n2. Test it.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sections := ReadSections(id)
	if len(sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(sections))
	}
	if sections[0].Heading != "Current state" || sections[0].Body != "- One\n- Two" {
		t.Fatalf("first section = %+v", sections[0])
	}
	if sections[1].Heading != "Proposed implementation" || !strings.Contains(sections[1].Body, "2. Test it.") {
		t.Fatalf("second section = %+v", sections[1])
	}
}

func TestDefaultContentUsesLeanScopingTemplate(t *testing.T) {
	content := DefaultContent("Scope issue", "scoping")
	for _, want := range []string{"## Current understanding", "## Key decisions", "## Proposed plan", "## Acceptance criteria", "## Open questions / risks", "## User confirmation", "## Next coding steps"} {
		if !strings.Contains(content, want) {
			t.Fatalf("scoping template missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"## Review summary", "## Tests run", "## Blockers"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("scoping template should not include %q:\n%s", unwanted, content)
		}
	}
}

func TestReadSupportsScopingSectionNames(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "scope-session"
	path := home.RunbookPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Agent Runbook\n\n## Current understanding\n- Found parser package.\n\n## Key decisions\n- Keep token model.\n\n## Open questions / risks\n- Migration risk.\n\n## User confirmation\n- User approved plan.\n\n## Next coding steps\n- Add parser tests.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := Read(id)
	if !strings.Contains(summary.CurrentState, "parser package") {
		t.Fatalf("CurrentState = %q", summary.CurrentState)
	}
	if !strings.Contains(summary.DecisionsMade, "token model") {
		t.Fatalf("DecisionsMade = %q", summary.DecisionsMade)
	}
	if !strings.Contains(summary.Blockers, "Migration risk") {
		t.Fatalf("Blockers = %q", summary.Blockers)
	}
	if !strings.Contains(summary.NextAction, "parser tests") {
		t.Fatalf("NextAction = %q", summary.NextAction)
	}
	if !strings.Contains(summary.UserConfirmation, "approved") {
		t.Fatalf("UserConfirmation = %q", summary.UserConfirmation)
	}
}

func TestValidateScopedHandoffRequiresUserConfirmation(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "REB-1-scope"
	path := home.RunbookPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(DefaultContent("Scope issue", "scoping")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateScopedHandoff(id); err == nil {
		t.Fatalf("expected missing user confirmation to be rejected")
	}
}

func TestScopedHandoffUsesRunbookSections(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "REB-2-scope"
	path := home.RunbookPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Agent Runbook\n\n## Goal\nOriginal issue title.\n\n## Current understanding\n- Parser entrypoint identified.\n- Types are already available.\n\n## Key decisions\n- Keep existing token model.\n\n## Proposed plan\n1. Add parser test.\n2. Implement parser.\n\n## User confirmation\n- User approved the parser-first plan.\n\n## Next coding steps\n- Implement parser tests first.\n\n## Open questions / risks\n- None.\n\n## Review notes\nNot needed for handoff.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	block := ScopedHandoff(id, "Build the parser path.")
	for _, want := range []string{"cmux scoped handoff", "Build the parser path.", "Parser entrypoint identified.", "Types are already available.", "Keep existing token model.", "Add parser test.", "User approved the parser-first plan.", "Implement parser tests first."} {
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

func TestReplaceScopedHandoffReplacesExistingBlock(t *testing.T) {
	description := "Original\n\n" + ScopedStartMarker + "\nold\n" + ScopedEndMarker
	got := ReplaceScopedHandoff(description, "new")
	if !strings.Contains(got, "Original") || !strings.Contains(got, "new") {
		t.Fatalf("description not preserved/replaced:\n%s", got)
	}
	if strings.Contains(got, "old") {
		t.Fatalf("old scoped block remained:\n%s", got)
	}
}
