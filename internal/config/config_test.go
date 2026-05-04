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
