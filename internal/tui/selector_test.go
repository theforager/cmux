package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/theforager/cmux/internal/queue"
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

func TestResolvedAgentModeFromProfile(t *testing.T) {
	if got := resolvedAgentMode(row{profile: string(types.ProfilePlan), queueIssue: "REB-1"}, agentModeAuto); got != agentModePlan {
		t.Fatalf("plan profile mode = %s, want Plan", agentModeLabel(got))
	}
	if got := resolvedAgentMode(row{linearState: "Backlog", queueIssue: "REB-1"}, agentModeAuto); got != agentModeImplementation {
		t.Fatalf("Linear state should not infer profile, got %s", agentModeLabel(got))
	}
	if got := resolvedAgentMode(row{profile: string(types.ProfilePlan), queueIssue: "REB-1"}, agentModeImplementation); got != agentModeImplementation {
		t.Fatalf("override mode = %s, want Implementation", agentModeLabel(got))
	}
	if got := resolvedAgentMode(row{linearState: "Todo", queueIssue: "REB-1"}, agentModeFresh); got != agentModeFresh {
		t.Fatalf("fresh mode = %s, want Fresh", agentModeLabel(got))
	}
}

func TestRenderAgentTabsShowsOnlyConcreteModes(t *testing.T) {
	r := row{linearState: "Backlog", queueIssue: "REB-1"}
	got := renderAgentTabs(r, agentModePlan)
	if strings.Contains(got, "Auto") {
		t.Fatalf("tabs should not show an Auto tab: %q", got)
	}
	for _, want := range []string{"Fresh", "Plan", "Implement", "Debug", "Review"} {
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

func TestDebugReviewAgentModeIsPreservedForQueueStart(t *testing.T) {
	m := model{filtered: []row{{id: "REB-1", kind: "queue", queueIssue: "REB-1", repo: "/tmp/repo"}}, agentMode: agentModeDebug}
	if err := m.prepareAgentStart(); err != nil {
		t.Fatal(err)
	}
	if m.create != "queue" {
		t.Fatalf("create = %q, want queue", m.create)
	}
	if got := profileForAgentMode(m.agentMode); got != types.ProfileDebug {
		t.Fatalf("profile = %s, want debug", got)
	}

	m.agentMode = agentModeReview
	if err := m.prepareAgentStart(); err != nil {
		t.Fatal(err)
	}
	if got := profileForAgentMode(m.agentMode); got != types.ProfileReview {
		t.Fatalf("profile = %s, want review", got)
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

func TestManualIssueOrTaskStartPromptsForRepoBeforeAgent(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "test")
	m := model{mode: "start", input: "REB-1"}
	if err := m.commitAction(); err != nil {
		t.Fatal(err)
	}
	if m.mode != "repoPick" {
		t.Fatalf("mode = %q, want repoPick", m.mode)
	}
	if m.create != "start" || m.target != "REB-1" {
		t.Fatalf("create state = %q/%q", m.create, m.target)
	}
}

func TestStructuredAgentPanelOffersRuntimeControls(t *testing.T) {
	r := row{
		id:         "REB-1",
		agent:      "codex",
		active:     true,
		structured: true,
		linear:     "https://linear.app/acme/issue/REB-1",
		queueIssue: "REB-1",
		profile:    string(types.ProfileImplement),
	}
	choices := agentActionChoices(r, agentModeReview)
	got := make([]string, 0, len(choices))
	for _, choice := range choices {
		got = append(got, choice.id)
	}
	want := []string{"openExisting", "stopExisting", "restartExisting", "startFreshExisting", "startFreshExisting", "startFreshExisting"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentActionChoices = %#v, want %#v", got, want)
	}
	if choices[3].command != "codex" || choices[3].label != "Fresh current" || !strings.Contains(choices[3].description, "Review") {
		t.Fatalf("fresh current choice = %+v, want codex review", choices[3])
	}
	if choices[len(choices)-1].label != "Fresh custom" || !choices[len(choices)-1].custom {
		t.Fatalf("last fresh choice = %+v, want custom", choices[len(choices)-1])
	}
}

func TestStoppedStructuredAgentPanelOffersRestartAndFresh(t *testing.T) {
	r := row{id: "REB-1", agent: "claude", structured: true, queueIssue: "REB-1"}
	choices := agentActionChoices(r, agentModePlan)
	got := make([]string, 0, len(choices))
	for _, choice := range choices {
		got = append(got, choice.id)
	}
	want := []string{"restartExisting", "startFreshExisting", "startFreshExisting", "startFreshExisting"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentActionChoices = %#v, want %#v", got, want)
	}
}

func TestStructuredFreshCustomPromptsForAgentCommand(t *testing.T) {
	m := model{
		filtered: []row{{
			id:         "REB-1",
			agent:      "codex",
			structured: true,
			active:     true,
		}},
		agentMode: agentModeDebug,
	}
	choices := agentActionChoices(m.filtered[0], m.agentMode)
	customIndex := -1
	for i, choice := range choices {
		if choice.custom {
			customIndex = i
			break
		}
	}
	if customIndex < 0 {
		t.Fatal("custom fresh choice not found")
	}
	m.agentPick = customIndex
	if err := m.chooseAgentAction(); err != nil {
		t.Fatal(err)
	}
	if m.mode != "agentCustom" || m.create != "freshExisting" || m.target != "REB-1" {
		t.Fatalf("mode/create/target = %q/%q/%q, want custom fresh", m.mode, m.create, m.target)
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

func TestDashboardQueueRowsFillAfterSkippingStarted(t *testing.T) {
	rows := []queue.Row{
		{Issue: types.LinearIssue{Identifier: "REB-1"}, Started: true, Session: &types.AgentSession{ID: "REB-1"}},
		{Issue: types.LinearIssue{Identifier: "REB-2"}, Started: true, Session: &types.AgentSession{ID: "REB-2"}},
		{Issue: types.LinearIssue{Identifier: "REB-3"}},
		{Issue: types.LinearIssue{Identifier: "REB-4"}},
		{Issue: types.LinearIssue{Identifier: "REB-5"}},
	}
	got := queueRowsFromLinearRows(rows, types.QueuePreset{Name: "test"}, false, nil, 3)
	ids := make([]string, 0, len(got))
	for _, row := range got {
		ids = append(ids, row.id)
	}
	want := []string{"REB-3", "REB-4", "REB-5"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("queue rows = %#v, want %#v", ids, want)
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
	want := []string{"agentOpen", "workspaceOpen", "briefPublish", "linearMove", "detail", "title", "close", "forget", "deleteWorktree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actionsFor structured = %#v, want %#v", got, want)
	}
	if actionSection("agentOpen") != "Agent" || actionSection("workspaceOpen") != "Workspace" || actionSection("briefPublish") != "Brief" || actionSection("linearMove") != "Linear" {
		t.Fatalf("actions should use stable sections")
	}
}

func TestPublishBriefActionRunsAsBusyCommand(t *testing.T) {
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
		t.Fatal("publish action should return a command")
	}
	if m.mode != "busy" || !strings.Contains(m.message, "Publishing") {
		t.Fatalf("mode/message = %q/%q, want busy publishing", m.mode, m.message)
	}
}

func TestCloseActionRequiresConfirmation(t *testing.T) {
	r := row{
		id:         "REB-1",
		status:     string(types.StatusRunning),
		workspace:  "/tmp/cmux-worktree",
		repo:       "/tmp/repo",
		structured: true,
		active:     true,
	}
	actions := actionsFor(r, false)
	closeIndex := -1
	for i, action := range actions {
		if action.id == "close" {
			closeIndex = i
			break
		}
	}
	if closeIndex < 0 {
		t.Fatal("close action not found")
	}
	m := model{filtered: []row{r}, mode: "actionMenu", actionSelected: closeIndex, selectedQueue: map[string]bool{}}
	cmd, err := m.chooseAction()
	if err != nil {
		t.Fatal(err)
	}
	if cmd != nil {
		t.Fatal("close should not run before confirmation")
	}
	if m.mode != "close" {
		t.Fatalf("mode = %q, want close confirmation", m.mode)
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

func TestQueuePathActionShowsLinearURL(t *testing.T) {
	m := model{filtered: []row{{id: "REB-1", kind: "queue", queueIssue: "REB-1", linear: "https://linear.app/acme/issue/REB-1"}}}
	m.showWorkspacePath()
	if m.message != "Linear: https://linear.app/acme/issue/REB-1" {
		t.Fatalf("message = %q", m.message)
	}
}
