package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/types"
)

func TestCleanDropsBriefPlaceholders(t *testing.T) {
	placeholders := []string{
		"- None.",
		"- Not started.",
		"- Not run.",
		"- Not ready.",
		"- Not confirmed.",
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
	path := home.BriefPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Session Brief\n\n## Current state\n- One\n- Two\n\n## Proposed implementation\n1. Build it.\n2. Test it.\n"
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

func TestDefaultContentUsesProfileTemplates(t *testing.T) {
	plan := DefaultContent("Scope issue", types.BriefPlan)
	for _, want := range []string{"## Current understanding", "## Decisions", "## Plan", "## Open questions", "## Next steps"} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan template missing %q:\n%s", want, plan)
		}
	}

	implementation := DefaultContent("Build issue", types.BriefImplementation)
	for _, want := range []string{"## Summary", "## Changes made", "## Tests run", "## Risks / follow-up", "## Reviewer notes", "## Branch / PR"} {
		if !strings.Contains(implementation, want) {
			t.Fatalf("implementation template missing %q:\n%s", want, implementation)
		}
	}
}

func TestRenderDropsTopLevelBriefTitle(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "REB-1"
	path := home.BriefPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Session Brief\n\n## Summary\nDone.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Render(id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "# Session Brief") || !strings.Contains(got, "## Summary") {
		t.Fatalf("rendered brief = %q", got)
	}
}

func TestBriefStateTracksPublishedHash(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	id := "REB-2"
	path := home.BriefPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Session Brief\n\n## Summary\nOne.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(id)
	if err != nil {
		t.Fatal(err)
	}
	session := types.AgentSession{ID: id, Brief: types.BriefData{PublishedHash: Hash(rendered)}}
	if got := State(session); got != "published" {
		t.Fatalf("state = %q, want published", got)
	}
	if err := os.WriteFile(path, []byte("# Session Brief\n\n## Summary\nTwo.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := State(session); got != "changed since publish" {
		t.Fatalf("state = %q, want changed since publish", got)
	}
}
