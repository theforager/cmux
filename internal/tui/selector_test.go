package tui

import (
	"reflect"
	"testing"

	"github.com/theforager/cmux/internal/types"
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

func TestResolvedAgentModeFromLinearState(t *testing.T) {
	if got := resolvedAgentMode(row{linearState: "Backlog", queueIssue: "REB-1"}, agentModeAuto); got != agentModeScoping {
		t.Fatalf("Backlog mode = %s, want Scoping", agentModeLabel(got))
	}
	if got := resolvedAgentMode(row{linearState: "Todo", queueIssue: "REB-1"}, agentModeAuto); got != agentModeImplementation {
		t.Fatalf("Todo mode = %s, want Implementation", agentModeLabel(got))
	}
	if got := resolvedAgentMode(row{linearState: "Backlog", queueIssue: "REB-1"}, agentModeImplementation); got != agentModeImplementation {
		t.Fatalf("override mode = %s, want Implementation", agentModeLabel(got))
	}
}

func TestStructuredActionsAreSimple(t *testing.T) {
	r := row{
		id:         "REB-1",
		status:     string(types.StatusRunning),
		workspace:  "/tmp/cmux-worktree",
		repo:       "/tmp/repo",
		linear:     "https://linear.app/acme/issue/REB-1",
		structured: true,
		active:     true,
	}
	actions := actionsFor(r, false)
	got := make([]string, 0, len(actions))
	for _, action := range actions {
		got = append(got, action.id)
	}
	want := []string{"agentOpen", "workspaceOpen", "submit", "abandon", "detail", "title", "deleteWorktree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actionsFor structured = %#v, want %#v", got, want)
	}
	if actionSection("agentOpen") != "Open" || actionSection("workspaceOpen") != "Open" {
		t.Fatalf("agent and workspace actions should share the Open section")
	}
}

func TestLocalStateClusterKeepsBadgesWithCmuxState(t *testing.T) {
	r := row{status: string(types.StatusWaiting), workspaceShells: []string{"cmux@workspace@REB-1"}}
	if got := localStateCluster(r); got != "◐ waiting ⧉" {
		t.Fatalf("localStateCluster = %q, want waiting with workspace badge", got)
	}
	r = row{kind: "queue", status: "queued", queueIssue: "REB-2"}
	if got := localStateCluster(r); got != "-" {
		t.Fatalf("unstarted queue localStateCluster = %q, want -", got)
	}
}

func TestCmuxOwnedSeparateWorktree(t *testing.T) {
	r := row{repo: "/tmp/repo", workspace: "/not/cmux/worktree"}
	if isCmuxOwnedSeparateWorktree(r) {
		t.Fatal("external worktree should not be treated as cmux-owned")
	}
}
