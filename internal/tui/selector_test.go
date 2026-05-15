package tui

import (
	"reflect"
	"testing"
)

func TestSplitEditorCommand(t *testing.T) {
	got, err := splitEditorCommand(`open -a "Visual Studio Code"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "-a", "Visual Studio Code"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitEditorCommand = %#v, want %#v", got, want)
	}
}

func TestSplitEditorCommandRejectsUnterminatedQuote(t *testing.T) {
	if _, err := splitEditorCommand(`open -a "Visual Studio Code`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
}

func TestRemoteWorkspaceCommand(t *testing.T) {
	got := remoteWorkspaceCommand("remoteCursor", "devship", "/home/dev/rebar cosmos")
	want := "cursor --remote ssh-remote+devship '/home/dev/rebar cosmos'"
	if got != want {
		t.Fatalf("remoteWorkspaceCommand = %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/home/dev/rebar-cosmos"); got != "/home/dev/rebar-cosmos" {
		t.Fatalf("shellQuote safe = %q", got)
	}
	if got := shellQuote("/home/dev/rebar cosmos"); got != "'/home/dev/rebar cosmos'" {
		t.Fatalf("shellQuote spaced = %q", got)
	}
}
