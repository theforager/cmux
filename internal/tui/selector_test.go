package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if got := resolvedAgentMode(row{linearState: "Todo", queueIssue: "REB-1"}, agentModeFresh); got != agentModeFresh {
		t.Fatalf("fresh mode = %s, want Fresh", agentModeLabel(got))
	}
}

func TestRenderAgentTabsShowsOnlyConcreteModes(t *testing.T) {
	r := row{linearState: "Backlog", queueIssue: "REB-1"}
	got := renderAgentTabs(r, agentModeScoping)
	if strings.Contains(got, "Auto") {
		t.Fatalf("tabs should not show an Auto tab: %q", got)
	}
	for _, want := range []string{"Fresh", "Scoping", "Implementation"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tabs missing %q: %q", want, got)
		}
	}
}

func TestFreshAgentModeUsesFreshCreation(t *testing.T) {
	m := model{filtered: []row{{id: "REB-1", kind: "queue", queueIssue: "REB-1", repo: "/tmp/repo"}}, agentMode: agentModeFresh}
	if err := m.prepareAgentStart(); err != nil {
		t.Fatal(err)
	}
	if m.create != "queueFresh" {
		t.Fatalf("create = %q, want queueFresh", m.create)
	}
}

func TestAgentPanelStartPromptsForRepo(t *testing.T) {
	m := model{filtered: []row{{id: "REB-1", kind: "queue", queueIssue: "REB-1", repo: "/tmp/repo"}}, agentMode: agentModeImplementation}
	if err := m.startAgentFromPanel("codex"); err != nil {
		t.Fatal(err)
	}
	if m.mode != "repoPick" {
		t.Fatalf("mode = %q, want repoPick", m.mode)
	}
	if m.createAgent != "codex" {
		t.Fatalf("createAgent = %q, want codex", m.createAgent)
	}
	if m.create != "queue" || m.target != "REB-1" || m.createRepo != "/tmp/repo" {
		t.Fatalf("create state = %q/%q/%q", m.create, m.target, m.createRepo)
	}
}

func TestShiftSpaceTogglesQueueSelection(t *testing.T) {
	m := model{
		filtered:      []row{{id: "REB-1", kind: "queue", queueIssue: "REB-1"}},
		fullQueue:     true,
		mode:          "browse",
		selectedQueue: map[string]bool{},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\u00a0'}})
	got := next.(model)
	if !got.selectedQueue["REB-1"] {
		t.Fatalf("shift-space-like key did not toggle queue selection")
	}
}

func TestDashboardGroupKeepsAttentionAsRowStatus(t *testing.T) {
	for _, status := range []types.AgentStatus{types.StatusWaiting, types.StatusBlocked, types.StatusReadyForReview, types.StatusRunning} {
		if got := dashboardGroup(status); got != "Agent sessions" {
			t.Fatalf("%s group = %q, want Agent sessions", status, got)
		}
	}
	if got := dashboardGroup(types.StatusDone); got != "Done / other" {
		t.Fatalf("done group = %q", got)
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

func TestSubmitActionRunsAsBusyCommand(t *testing.T) {
	m := model{
		filtered: []row{{
			id:         "REB-1",
			status:     string(types.StatusRunning),
			workspace:  "/tmp/cmux-worktree",
			repo:       "/tmp/repo",
			structured: true,
			active:     true,
		}},
		mode:           "actionMenu",
		actionSelected: 2,
		selectedQueue:  map[string]bool{},
	}
	cmd, err := m.chooseAction()
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil {
		t.Fatal("submit action should return a command")
	}
	if m.mode != "busy" || !strings.Contains(m.message, "Submitting") {
		t.Fatalf("mode/message = %q/%q, want busy submitting", m.mode, m.message)
	}
}

func TestLocalStateClusterKeepsBadgesWithCmuxState(t *testing.T) {
	r := row{status: string(types.StatusWaiting), workspaceShells: []string{"cmux@workspace@REB-1"}}
	if got := localStateCluster(r); got != "▲ waiting ⧉" {
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
