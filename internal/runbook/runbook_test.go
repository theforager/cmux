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
