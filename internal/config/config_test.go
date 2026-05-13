package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestLoadMissingConfig(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.QueuePresets) != 0 {
		t.Fatalf("QueuePresets = %d, want 0", len(cfg.QueuePresets))
	}
}

func TestSaveLoadConfig(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	want := types.Config{DefaultQueuePreset: "Mine", QueuePresets: []types.QueuePreset{{Name: "Mine", Teams: []string{"team-id"}, Limit: 3}}}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultQueuePreset != want.DefaultQueuePreset || len(got.QueuePresets) != 1 || got.QueuePresets[0].Teams[0] != "team-id" {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestLoadMalformedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CMUX_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected malformed config error")
	}
}

func TestLoadOrDefaultAddsWorkflowTransitions(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	cfg, err := LoadOrDefault()
	if err != nil {
		t.Fatal(err)
	}
	transition, ok := Transition(cfg, "mark_needs_review")
	if !ok {
		t.Fatalf("missing mark_needs_review transition: %+v", cfg.Linear.Workflow.Transitions)
	}
	if len(transition.AddLabels) != 2 || transition.AddLabels[1] != "needs-review" {
		t.Fatalf("mark_needs_review transition = %+v", transition)
	}
	done, ok := Transition(cfg, "done")
	if !ok {
		t.Fatalf("missing done transition: %+v", cfg.Linear.Workflow.Transitions)
	}
	if !done.PlaceAtTop {
		t.Fatalf("done transition should place issue at top: %+v", done)
	}
}

func TestRememberRepoSavesRepoAndDefault(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	repo := t.TempDir()
	if err := RememberRepo(repo); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRepoPath != repo {
		t.Fatalf("DefaultRepoPath = %q, want %q", cfg.DefaultRepoPath, repo)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Path != repo {
		t.Fatalf("Repos = %+v, want %q", cfg.Repos, repo)
	}
	if err := RememberRepo(repo); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("RememberRepo duplicated repo: %+v", cfg.Repos)
	}
}

func TestRememberEditorSavesCommandAndDefault(t *testing.T) {
	t.Setenv("CMUX_HOME", t.TempDir())
	if err := RememberEditor("cursor"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultEditorCommand != "cursor" {
		t.Fatalf("DefaultEditorCommand = %q, want cursor", cfg.DefaultEditorCommand)
	}
	if len(cfg.EditorCommands) != 1 || cfg.EditorCommands[0] != "cursor" {
		t.Fatalf("EditorCommands = %+v, want cursor", cfg.EditorCommands)
	}
	if err := RememberEditor("cursor"); err != nil {
		t.Fatal(err)
	}
	if err := RememberEditor("code"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultEditorCommand != "code" {
		t.Fatalf("DefaultEditorCommand = %q, want code", cfg.DefaultEditorCommand)
	}
	if len(cfg.EditorCommands) != 2 || cfg.EditorCommands[0] != "code" || cfg.EditorCommands[1] != "cursor" {
		t.Fatalf("EditorCommands = %+v, want code/cursor without duplicates", cfg.EditorCommands)
	}
}
