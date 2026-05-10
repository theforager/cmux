package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theforager/cmux/internal/types"
)

func TestDefaultStateIDs(t *testing.T) {
	states := []types.LinearWorkflowState{
		{ID: "done", Name: "Done"},
		{ID: "backlog", Name: "Backlog"},
		{ID: "todo", Name: "Todo"},
		{ID: "scoping", Name: "Scoping"},
	}
	got := defaultStateIDs(states)
	want := []string{"todo", "scoping", "backlog"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("defaultStateIDs = %v, want %v", got, want)
		}
	}
}

func TestResolveSSHHostLiteral(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := resolveSSHHost("dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "dev@example.com" {
		t.Fatalf("resolveSSHHost literal = %v", got)
	}
}

func TestResolveSSHHostAlias(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	dir := filepath.Join(configHome, "cmux")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hosts := "# comment\nprod=-p 2222 user@prod.example.com\n"
	if err := os.WriteFile(filepath.Join(dir, "hosts"), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSSHHost("prod")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "2222", "user@prod.example.com"}
	if len(got) != len(want) {
		t.Fatalf("resolveSSHHost alias = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolveSSHHost alias = %v, want %v", got, want)
		}
	}
}
