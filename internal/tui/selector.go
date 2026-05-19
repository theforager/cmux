package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/brief"
	"github.com/theforager/cmux/internal/config"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/queue"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const resetMouseReporting = "\x1b[?9l\x1b[?1000l\x1b[?1001l\x1b[?1002l\x1b[?1003l\x1b[?1005l\x1b[?1006l\x1b[?1015l\x1b[?1016l"

type row struct {
	id              string
	title           string
	status          string
	group           string
	updated         string
	updatedAt       time.Time
	queueRank       int
	session         string
	repo            string
	workspace       string
	branch          string
	agent           string
	brief           string
	linear          string
	profile         string
	briefState      string
	lastSummary     string
	linearState     string
	currentState    string
	nextAction      string
	blockers        string
	testsRun        string
	reviewSummary   string
	preview         string
	terminalPreview string
	detail          string
	active          bool
	kind            string
	structured      bool
	queueIssue      string
	workspaceShells []string
}

type model struct {
	rows           []row
	filtered       []row
	selected       int
	agentMode      int
	agentPick      int
	agentSelected  int
	repoSelected   int
	editorSelected int
	workspaceMode  int
	workspacePick  int
	actionSelected int
	newSelected    int
	filter         string
	input          string
	create         string
	createRepo     string
	createAgent    string
	createCustom   bool
	target         string
	pendingAction  string
	pending        types.AgentStatus
	mode           string
	message        string
	chosen         string
	fullQueue      bool
	selectedQueue  map[string]bool
	width          int
	height         int
}

var issuePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)

type agentChoice struct {
	label       string
	command     string
	description string
	custom      bool
}

var agentChoices = []agentChoice{
	{label: "Claude", command: "claude", description: "Launch the Claude CLI agent"},
	{label: "Codex", command: "codex", description: "Launch the Codex CLI agent"},
	{label: "Other", description: "Type a custom agent command", custom: true},
}

type menuChoice struct {
	id          string
	label       string
	description string
	danger      bool
}

type editorChoice struct {
	label       string
	command     string
	description string
	custom      bool
}

type workspaceChoice struct {
	id          string
	label       string
	description string
	disabled    bool
}

type agentActionChoice struct {
	id          string
	label       string
	command     string
	description string
	custom      bool
	disabled    bool
}

type actionDoneMsg struct {
	id       string
	selectID string
	err      error
}

const (
	agentModeAuto = iota
	agentModeFresh
	agentModePlan
	agentModeImplementation
	agentModeDebug
	agentModeReview
)

var agentModeNames = []string{"Fresh", "Plan", "Implement", "Debug", "Review"}
var agentModeValues = []int{agentModeFresh, agentModePlan, agentModeImplementation, agentModeDebug, agentModeReview}

const (
	workspaceModeTerminal = iota
	workspaceModeEditor
	workspaceModeRemote
)

var workspaceModeNames = []string{"Terminal", "Editor", "Remote"}

var (
	cyan        = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim         = lipgloss.NewStyle().Faint(true)
	green       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red         = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	bold        = lipgloss.NewStyle().Bold(true)
	activeTab   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Padding(0, 1)
)

func Run(popup bool) (string, error) {
	rows, err := loadRows(false, false)
	if err != nil {
		return "", err
	}
	restoreMouse := prepareCopyFriendlyTerminal()
	defer restoreMouse()
	m := model{rows: rows, filtered: rows, mode: "browse", width: 80, height: 24, selectedQueue: map[string]bool{}}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if m, ok := final.(model); ok {
		return m.chosen, nil
	}
	return "", nil
}

func RunQueue() (string, error) {
	rows, err := loadRows(true, false)
	if err != nil {
		return "", err
	}
	restoreMouse := prepareCopyFriendlyTerminal()
	defer restoreMouse()
	m := model{rows: rows, filtered: rows, mode: "browse", width: 80, height: 24, fullQueue: true, selectedQueue: map[string]bool{}}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if m, ok := final.(model); ok {
		return m.chosen, nil
	}
	return "", nil
}

func prepareCopyFriendlyTerminal() func() {
	// cmux does not use mouse input. Disabling inherited mouse-reporting and
	// tmux mouse mode while the selector is open keeps normal drag-to-select
	// copying available inside the popup/menu.
	fmt.Fprint(os.Stdout, resetMouseReporting)
	return tmux.SuspendMouse()
}

func loadRows(fullQueue bool, refreshLinear bool) ([]row, error) {
	_, _ = agent.ScanWithOptions(agent.ScanOptions{RefreshLinear: refreshLinear})
	tmuxSessions, err := tmux.List()
	if err != nil {
		return nil, err
	}
	agentSessions, _ := state.List()
	byTmux := map[string]row{}
	agentIDs := map[string]bool{}
	for _, s := range agentSessions {
		agentIDs[s.ID] = true
		byTmux[s.TmuxSession] = agentRow(s, false)
	}
	childByParent := map[string][]string{}
	activeTmux := map[string]bool{}
	for _, s := range tmuxSessions {
		activeTmux[s.Name] = true
		if s.Kind == "workspace" && s.ParentID != "" {
			childByParent[s.ParentID] = append(childByParent[s.ParentID], s.Name)
		}
	}
	var rows []row
	seen := map[string]bool{}
	for _, s := range tmuxSessions {
		if s.Kind == "workspace" && s.ParentID != "" && agentIDs[s.ParentID] {
			seen[s.Name] = true
			continue
		}
		if r, ok := byTmux[s.Name]; ok {
			r.active = true
			r.workspaceShells = childByParent[r.id]
			r.terminalPreview = tmux.Capture(s.Name, 8)
			if r.preview == "" {
				r.preview = r.terminalPreview
			}
			rows = append(rows, r)
			seen[s.Name] = true
			continue
		}
		rows = append(rows, rawTmuxRow(s))
		seen[s.Name] = true
	}
	for _, s := range agentSessions {
		if seen[s.TmuxSession] {
			continue
		}
		r := agentRow(s, false)
		r.workspaceShells = childByParent[r.id]
		r.status = "not-running"
		rows = append(rows, r)
	}
	rows = append(rows, queueRows(fullQueue, activeTmux)...)
	sortRows(rows)
	return rows, nil
}

func queueRows(fullQueue bool, activeTmux map[string]bool) []row {
	if !queue.Configured() {
		return []row{{
			id:      "linear-setup",
			title:   "Linear worklist not configured: set LINEAR_API_KEY and run cmux queue setup",
			status:  "setup",
			group:   "Linear worklist",
			updated: "",
			kind:    "notice",
			detail:  "Linear worklist needs LINEAR_API_KEY before cmux can fetch issues.",
		}}
	}
	limit := 8
	if fullQueue {
		limit = 50
	}
	fetchLimit := limit
	if !fullQueue {
		fetchLimit = 250
	}
	rows, preset, err := queue.Rows("", fetchLimit)
	if err != nil {
		return []row{{
			id:     "linear-error",
			title:  err.Error(),
			status: "error",
			group:  "Linear worklist",
			kind:   "notice",
			detail: "Queue preset could not be loaded or Linear returned an error.",
		}}
	}
	if len(rows) == 0 {
		return []row{{id: "linear-empty", title: "No matching Linear issues in " + preset.Name, status: "empty", group: "Linear worklist", kind: "notice"}}
	}
	return queueRowsFromLinearRows(rows, preset, fullQueue, activeTmux, limit)
}

func queueRowsFromLinearRows(rows []queue.Row, preset types.QueuePreset, fullQueue bool, activeTmux map[string]bool, displayLimit int) []row {
	out := make([]row, 0, len(rows))
	for rank, qr := range rows {
		if !fullQueue && qr.Started {
			continue
		}
		if !fullQueue && displayLimit > 0 && len(out) >= displayLimit {
			break
		}
		r := row{
			id:          qr.Issue.Identifier,
			title:       qr.Issue.Title,
			status:      string(qr.Status),
			group:       "Linear worklist",
			queueRank:   rank,
			repo:        preset.RepoPath,
			workspace:   preset.RepoPath,
			linear:      qr.Issue.URL,
			linearState: qr.Issue.State,
			branch:      qr.Issue.BranchName,
			kind:        "queue",
			queueIssue:  qr.Issue.Identifier,
			detail: strings.Join(nonEmpty([]string{
				"preset: " + preset.Name,
				"repo: " + preset.RepoPath,
				"linear: " + qr.Issue.URL,
				"team: " + valueOr(qr.Issue.TeamName, qr.Issue.TeamKey),
				"state: " + qr.Issue.State,
				"assignee: " + qr.Issue.AssigneeName,
			}), "\n"),
			preview: qr.Issue.Description,
		}
		if qr.Session != nil {
			sr := agentRow(*qr.Session, activeTmux[qr.Session.TmuxSession])
			sr.group = "Linear worklist"
			sr.kind = "queue"
			sr.queueIssue = qr.Issue.Identifier
			sr.queueRank = rank
			sr.linearState = qr.Issue.State
			sr.workspaceShells = r.workspaceShells
			if !sr.active {
				sr.status = "not-running"
			}
			r = sr
		}
		out = append(out, r)
	}
	return out
}

func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		gi, gj := groupRank(rows[i].group), groupRank(rows[j].group)
		if gi != gj {
			return gi < gj
		}
		if rows[i].group == "Linear worklist" && rows[i].queueRank != rows[j].queueRank {
			return rows[i].queueRank < rows[j].queueRank
		}
		return rows[i].updatedAt.After(rows[j].updatedAt)
	})
}

func groupRank(group string) int {
	switch group {
	case "Agent sessions":
		return 0
	case "Linear worklist":
		return 1
	case "Done / other":
		return 2
	default:
		return 3
	}
}

func rawTmuxRow(s tmux.Session) row {
	status := tmux.InferStatus(s.Name)
	preview := tmux.Capture(s.Name, 10)
	return row{
		id:              tmux.Child(s.Name),
		title:           displayName(s),
		status:          status,
		group:           dashboardGroup(types.AgentStatus(status)),
		updated:         format.AgeUnix(s.Created),
		updatedAt:       time.Unix(s.Created, 0),
		session:         s.Name,
		workspace:       s.Dir,
		agent:           valueOr(s.Agent, "shell"),
		preview:         preview,
		terminalPreview: preview,
		active:          true,
		kind:            "session",
	}
}

func agentRow(s types.AgentSession, active bool) row {
	notes := brief.Read(s.ID)
	preview := notes.Preview()
	if preview == "" {
		preview = s.LastSummary
	}
	detail := strings.Join(nonEmpty([]string{
		"repo: " + s.RepoPath,
		"workspace: " + valueOr(s.WorktreePath, s.RepoPath),
		"brief: " + valueOr(s.Brief.SourcePath, home.BriefPath(s.ID)),
		"linear: " + s.Linear.URL,
	}), "\n")
	return row{
		id:              s.ID,
		title:           s.Title,
		status:          string(s.Status),
		group:           dashboardGroup(s.Status),
		updated:         agentRowAge(s),
		updatedAt:       agentRowTime(s),
		session:         s.TmuxSession,
		repo:            s.RepoPath,
		workspace:       valueOr(s.WorktreePath, s.RepoPath),
		branch:          s.Branch,
		agent:           valueOr(s.AgentCommand, s.Provider),
		brief:           valueOr(s.Brief.SourcePath, home.BriefPath(s.ID)),
		linear:          s.Linear.URL,
		linearState:     s.Linear.State,
		profile:         string(s.Profile),
		briefState:      brief.State(s),
		lastSummary:     s.LastSummary,
		currentState:    notes.CurrentState,
		nextAction:      notes.NextAction,
		blockers:        notes.Blockers,
		testsRun:        notes.TestsRun,
		reviewSummary:   notes.ReviewSummary,
		preview:         preview,
		terminalPreview: s.Runtime.Preview,
		detail:          detail,
		active:          active,
		kind:            "session",
		structured:      true,
		queueIssue:      s.Linear.Identifier,
	}
}

func agentRowAge(s types.AgentSession) string {
	t := agentRowTime(s)
	if t.IsZero() {
		return "?"
	}
	return format.Age(t.UTC().Format(time.RFC3339))
}

func agentRowTime(s types.AgentSession) time.Time {
	switch s.Status {
	case types.StatusRunning, types.StatusIdle, types.StatusWaiting, types.StatusStale, types.StatusCrashed:
		if s.Runtime.LastActivityAt != "" {
			if t, err := time.Parse(time.RFC3339, s.Runtime.LastActivityAt); err == nil {
				return t
			}
		}
	}
	if t, err := time.Parse(time.RFC3339, s.LastUpdatedAt); err == nil {
		return t
	}
	return time.Time{}
}

func (m model) Init() tea.Cmd { return tea.ClearScreen }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case actionDoneMsg:
		if msg.err != nil {
			m.mode = "browse"
			m.message = msg.err.Error()
			return m, nil
		}
		switch msg.id {
		case "briefPublish":
			m.message = "Published brief"
		case "close":
			m.message = "Closed session"
		case "forget":
			m.message = "Forgot session"
		default:
			m.message = ""
		}
		m.mode = "browse"
		m.input = ""
		m.reloadRows(msg.selectID)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		key := msg.String()
		if m.mode == "help" {
			m.mode = "browse"
			return m, nil
		}
		if m.mode != "browse" && m.mode != "help" {
			if m.mode == "detail" {
				switch key {
				case "esc", "backspace", "ctrl+h", "i", "q":
					m.mode = "browse"
					m.message = ""
				case "enter":
					if r, ok := m.current(); ok && r.active {
						m.chosen = r.session
						return m, tea.Quit
					}
					m.message = "Session is not running"
				case ".":
					m.mode = "actionMenu"
					m.actionSelected = 0
				}
				return m, nil
			}
			if m.mode == "actionMenu" {
				switch key {
				case "esc", "backspace", "ctrl+h", ".", "q":
					m.mode = "browse"
				case "up", "k":
					m.moveAction(-1)
				case "down", "j", "tab":
					m.moveAction(1)
				case "enter":
					cmd, err := m.chooseAction()
					if err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					} else if cmd != nil {
						return m, cmd
					}
				}
				return m, nil
			}
			if m.mode == "busy" {
				return m, nil
			}
			if m.mode == "agentOpen" {
				switch key {
				case "esc", "backspace", "ctrl+h", "q":
					m.mode = "actionMenu"
					m.message = ""
				case "left", "h":
					m.moveAgentMode(-1)
				case "right", "l":
					m.moveAgentMode(1)
				case "up", "k":
					m.moveAgentAction(-1)
				case "down", "j", "tab":
					m.moveAgentAction(1)
				case "enter":
					if err := m.chooseAgentAction(); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "workspaceOpen" {
				switch key {
				case "esc", "backspace", "ctrl+h", "q":
					m.mode = "actionMenu"
					m.message = ""
				case "left", "h":
					m.moveWorkspaceMode(-1)
				case "right", "l":
					m.moveWorkspaceMode(1)
				case "up", "k":
					m.moveWorkspaceChoice(-1)
				case "down", "j", "tab":
					m.moveWorkspaceChoice(1)
				case "enter":
					if err := m.chooseWorkspace(); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "newMenu" {
				switch key {
				case "esc", "backspace", "ctrl+h", "a", "q":
					m.mode = "browse"
				case "up", "k":
					m.moveNew(-1)
				case "down", "j", "tab":
					m.moveNew(1)
				case "enter":
					m.chooseNew()
					if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "statusPick" {
				switch key {
				case "esc", "q":
					m.mode = "browse"
				case "1":
					m.pending = types.StatusRunning
					m.mode = "statusSummary"
					m.input = ""
				case "2":
					m.pending = types.StatusWaiting
					m.mode = "statusSummary"
					m.input = ""
				case "3":
					m.pending = types.StatusBlocked
					m.mode = "statusSummary"
					m.input = ""
				case "4":
					m.pending = types.StatusTestsFailed
					m.mode = "statusSummary"
					m.input = ""
				case "5":
					m.pending = types.StatusReadyForReview
					m.mode = "statusSummary"
					m.input = ""
				case "6":
					m.pending = types.StatusPROpened
					m.mode = "statusSummary"
					m.input = ""
				case "7":
					m.pending = types.StatusDone
					m.mode = "statusSummary"
					m.input = ""
				case "8":
					m.pending = types.StatusCrashed
					m.mode = "statusSummary"
					m.input = ""
				}
				return m, nil
			}
			if m.mode == "agentPick" {
				switch key {
				case "esc", "q":
					m.mode = "browse"
					m.input = ""
					m.create = ""
					m.createRepo = ""
					m.createAgent = ""
					m.createCustom = false
					m.target = ""
					m.message = ""
				case "up", "k":
					m.moveAgent(-1)
				case "down", "j", "tab":
					m.moveAgent(1)
				case "enter":
					if err := m.chooseAgent(); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "repoPick" {
				switch key {
				case "esc", "q":
					m.mode = "browse"
					m.input = ""
					m.create = ""
					m.createRepo = ""
					m.createAgent = ""
					m.createCustom = false
					m.target = ""
					m.message = ""
				case "up", "k":
					m.moveRepo(-1)
				case "down", "j", "tab":
					m.moveRepo(1)
				case "enter":
					if err := m.chooseRepo(); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "editorPick" {
				switch key {
				case "esc", "q":
					m.mode = "browse"
					m.input = ""
					m.message = ""
				case "up", "k":
					m.moveEditor(-1)
				case "down", "j", "tab":
					m.moveEditor(1)
				case "enter":
					if err := m.chooseEditor(); err != nil {
						m.message = err.Error()
					}
				}
				return m, nil
			}
			if isConfirmMode(m.mode) {
				switch key {
				case "esc", "backspace", "ctrl+h", "q", "n", "N":
					m.mode = "browse"
					m.input = ""
				case "enter", "y", "Y":
					selectedID := ""
					if r, ok := m.current(); ok {
						selectedID = r.id
					}
					m.message = ""
					if err := m.commitAction(); err != nil {
						m.message = err.Error()
					} else {
						m.message = ""
						m.mode = "browse"
						m.input = ""
						m.reloadRows(selectedID)
					}
				}
				return m, nil
			}
			switch key {
			case "esc":
				m.mode = "browse"
				m.input = ""
			case "enter":
				if m.mode == "filter" {
					m.mode = "browse"
					return m, nil
				}
				before := m.mode
				selectedID := ""
				if r, ok := m.current(); ok {
					selectedID = r.id
				}
				if err := m.commitAction(); err != nil {
					m.message = err.Error()
				} else if m.chosen != "" {
					return m, tea.Quit
				} else {
					if m.mode == before {
						m.mode = "browse"
						m.input = ""
						m.reloadRows(selectedID)
					}
				}
			case "backspace", "ctrl+h":
				if m.mode == "filter" {
					if len(m.filter) > 0 {
						m.filter = m.filter[:len(m.filter)-1]
					}
					m.applyFilter()
				} else if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
			default:
				if len(key) == 1 {
					if m.mode == "filter" {
						m.filter += key
						m.applyFilter()
					} else {
						m.input += key
					}
				}
			}
			return m, nil
		}
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.mode = "help"
		case "/":
			m.mode = "filter"
		case "a":
			m.mode = "newMenu"
			m.newSelected = 0
		case "i":
			if _, ok := m.current(); ok {
				m.mode = "detail"
				m.message = ""
			}
		case ".":
			if _, ok := m.current(); ok {
				m.mode = "actionMenu"
				m.actionSelected = 0
			}
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-5)
		case "pgdown":
			m.move(5)
		case "home":
			m.selected = 0
		case "end":
			if len(m.filtered) > 0 {
				m.selected = len(m.filtered) - 1
			}
		case "R":
			rows, err := loadRows(m.fullQueue, true)
			if err == nil {
				m.rows = rows
				m.applyFilter()
			} else {
				m.message = err.Error()
			}
		case "tab":
			m.fullQueue = !m.fullQueue
			m.filter = ""
			rows, err := loadRows(m.fullQueue, m.fullQueue)
			if err == nil {
				m.rows = rows
				m.applyFilter()
				m.message = ""
			} else {
				m.message = err.Error()
			}
		case " ":
			if m.fullQueue {
				m.toggleQueueSelection()
			}
		case "enter":
			if err := m.primaryAction(); err != nil {
				m.message = err.Error()
			} else if m.chosen != "" {
				return m, tea.Quit
			}
		}
		if isSpaceToggleKey(msg) && key != " " {
			if m.fullQueue {
				m.toggleQueueSelection()
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.mode == "help" {
		return helpView()
	}
	if m.mode == "detail" {
		return m.detailView()
	}
	var b strings.Builder
	count := fmt.Sprintf("%d sessions", len(m.filtered))
	if m.fullQueue {
		count = fmt.Sprintf("%d rows", len(m.filtered))
	}
	b.WriteString(bold.Render(cyan.Render("cmux")) + dim.Render("  "+count))
	if m.filter != "" || m.mode == "filter" {
		b.WriteString("  " + cyan.Render("/"+m.filter))
		if m.mode == "filter" {
			b.WriteString(cyan.Render("▌"))
		}
	}
	b.WriteString("\n\n")
	if len(m.filtered) == 0 {
		b.WriteString(dim.Render("  no sessions") + "\n")
	} else {
		lastGroup := ""
		start, end := m.visibleRange()
		if start > 0 {
			b.WriteString(dim.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
		}
		for i := start; i < end; i++ {
			r := m.filtered[i]
			if r.group != lastGroup {
				lastGroup = r.group
				b.WriteString(dim.Render("  "+r.group) + "\n")
			}
			mark := " "
			if r.kind == "queue" && !r.active && m.selectedQueue[r.queueIssue] {
				mark = "✓"
			}
			line := renderRowLine(r, mark, m.width)
			if i == m.selected {
				b.WriteString(green.Render("▌ "+line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
		if end < len(m.filtered) {
			b.WriteString(dim.Render(fmt.Sprintf("  ↓ %d more", len(m.filtered)-end)) + "\n")
		}
	}
	b.WriteString("\n" + dim.Render("── preview ─────────────────────────────────────────") + "\n")
	if len(m.filtered) > 0 {
		selected := m.filtered[m.selected]
		b.WriteString(m.previewPanel(selected))
	}
	b.WriteString("\n")
	prompt := m.prompt()
	if prompt != "" {
		b.WriteString(prompt)
	} else {
		if m.fullQueue {
			b.WriteString(dim.Render("↑↓/jk nav  enter start/open  space select  a new work  . actions  tab dashboard  / filter  R refresh  ? help  q quit"))
		} else {
			b.WriteString(dim.Render("↑↓/jk nav  enter open/start  a new work  . actions  tab Linear  / filter  R refresh  ? help  q quit"))
		}
	}
	if m.message != "" {
		if m.mode == "busy" {
			return b.String()
		}
		if prompt != "" {
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
		}
		b.WriteString(renderMessage(m.message))
	}
	return b.String()
}

func (m model) visibleRange() (int, int) {
	total := len(m.filtered)
	if total == 0 {
		return 0, 0
	}
	available := m.height - previewHeight(m.height) - 8
	if m.message != "" {
		available--
	}
	if available < 5 {
		available = 5
	}
	if available > total {
		available = total
	}
	if m.selected < available {
		return 0, available
	}
	start := m.selected - available/2
	if start+available > total {
		start = total - available
	}
	if start < 0 {
		start = 0
	}
	return start, start + available
}

func (m model) current() (row, bool) {
	if len(m.filtered) == 0 || m.selected < 0 || m.selected >= len(m.filtered) {
		return row{}, false
	}
	return m.filtered[m.selected], true
}

type previewLine struct {
	label string
	value string
	style string
}

func (m model) previewPanel(r row) string {
	maxLines := previewHeight(m.height)
	lines := previewContentLines(r, maxLines)
	if footer, ok := previewFooterLine(r); ok && len(lines) < maxLines && maxLines >= 6 {
		lines = append(lines, footer)
	}
	if len(lines) == 0 {
		lines = append(lines, previewLine{label: "status", value: "No preview available.", style: "dim"})
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(renderPreviewLine(line, m.width) + "\n")
	}
	return b.String()
}

func previewFooterLine(r row) (previewLine, bool) {
	switch {
	case r.kind == "queue" && !r.structured:
		parts := []string{
			valueOr(r.updated, ""),
			queueTeam(r),
			baseOrEmpty(r.repo),
		}
		value := compactJoin(parts)
		return previewLine{label: "meta", value: value, style: "footer"}, value != ""
	case r.structured:
		parts := []string{}
		if r.status == string(types.StatusCrashed) || r.status == string(types.StatusStale) || r.status == string(types.StatusWaiting) {
			parts = append(parts, r.status)
		}
		if r.profile != "" && r.profile != string(types.ProfileGeneral) {
			parts = append(parts, r.profile)
		}
		if r.briefState != "" {
			parts = append(parts, "brief "+r.briefState)
		}
		if r.workspace != "" {
			parts = append(parts, filepath.Base(r.workspace))
		}
		if r.branch != "" {
			parts = append(parts, r.branch)
		}
		if len(r.workspaceShells) > 0 {
			parts = append(parts, "⧉")
		}
		value := compactJoin(parts)
		return previewLine{label: "meta", value: value, style: "footer"}, value != ""
	default:
		parts := []string{}
		if r.workspace != "" {
			parts = append(parts, r.workspace)
		}
		if r.agent != "" {
			parts = append(parts, r.agent)
		}
		value := compactJoin(parts)
		return previewLine{label: "meta", value: value, style: "footer"}, value != ""
	}
}

func previewContentLines(r row, max int) []previewLine {
	if max <= 0 {
		return nil
	}
	switch {
	case r.kind == "queue" && !r.structured:
		return previewTextLines("desc", r.preview, max, false, "")
	case r.structured:
		termBudget := terminalPreviewBudget(max, r)
		briefBudget := max - termBudget
		briefLines := previewBriefLines(r, briefBudget)
		remaining := max - len(briefLines)
		if len(briefLines) > 0 && remaining > termBudget {
			remaining = termBudget
		}
		termLines := previewTextLines("term", r.terminalPreview, remaining, true, "dim")
		if len(briefLines) == 0 && len(termLines) == 0 {
			return []previewLine{{label: "summary", value: "No brief or terminal preview yet.", style: "dim"}}
		}
		return append(briefLines, termLines...)
	default:
		return previewTextLines("term", r.terminalPreview, max, true, "")
	}
}

func terminalPreviewBudget(max int, r row) int {
	if max <= 4 {
		return 1
	}
	if r.status == string(types.StatusCrashed) || r.status == string(types.StatusWaiting) {
		return min(5, max/2)
	}
	return min(4, max/3)
}

func previewBriefLines(r row, max int) []previewLine {
	lines := []previewLine{}
	add := func(label, value string) {
		if len(lines) >= max {
			return
		}
		value = cleanDetail(value)
		if value == "" {
			return
		}
		for _, line := range strings.Split(value, "\n") {
			line = compactPreviewText(line)
			if line == "" {
				continue
			}
			nextLabel := ""
			if len(lines) == 0 || label != "" {
				nextLabel = label
				label = ""
			}
			lines = append(lines, previewLine{label: nextLabel, value: line})
			if len(lines) >= max {
				return
			}
		}
	}
	add("current", r.currentState)
	add("next", r.nextAction)
	add("blocker", r.blockers)
	return lines
}

func previewTextLines(label, text string, max int, tailLines bool, style string) []previewLine {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	if tailLines {
		raw = tail(raw, max)
	} else if len(raw) > max {
		raw = raw[:max]
	}
	lines := []previewLine{}
	for _, line := range raw {
		line = compactPreviewText(line)
		if line == "" {
			continue
		}
		nextLabel := ""
		if len(lines) == 0 {
			nextLabel = label
		}
		lines = append(lines, previewLine{label: nextLabel, value: line, style: style})
		if len(lines) >= max {
			break
		}
	}
	return lines
}

func renderPreviewLine(line previewLine, width int) string {
	label := fmt.Sprintf("%-8s", line.label)
	value := compactPreviewText(line.value)
	if width > 24 {
		value = format.Trunc(value, width-13)
	}
	switch line.style {
	case "dim":
		value = dim.Render(value)
	case "footer":
		value = dim.Render(renderMetaValue(value))
	case "red":
		value = red.Render(value)
	}
	if strings.TrimSpace(line.label) == "" {
		return "         " + value
	}
	return renderPreviewLabel(label, line.style) + " " + value
}

func renderPreviewLabel(label, style string) string {
	switch style {
	case "dim", "footer":
		if strings.TrimSpace(label) == "term" || strings.TrimSpace(label) == "meta" {
			return cyan.Render(label)
		}
		return dim.Render(label)
	default:
		return cyan.Render(label)
	}
}

func renderMetaValue(value string) string {
	value = strings.ReplaceAll(value, "term ⧉", "term "+cyan.Render("⧉"))
	return value
}

func compactJoin(parts []string) string {
	return strings.Join(nonEmpty(parts), " · ")
}

func compactPreviewText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\t", " ")
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	return value
}

func renderRowLine(r row, mark string, width int) string {
	local := format.Trunc(localStateCluster(r), 20)
	linear := format.Trunc(linearStateLabel(r), 14)
	id := format.Trunc(r.id, 16)
	updated := strings.TrimSpace(r.updated)
	titleWidth := width - 2 - 1 - 17 - 21 - 15
	if updated != "" {
		titleWidth -= len(updated) + 2
	}
	if titleWidth < 20 {
		titleWidth = 20
	}
	title := format.Trunc(r.title, titleWidth)
	line := fmt.Sprintf("%s %-16s %-20s %-14s %s", mark, id, local, linear, title)
	if updated != "" {
		line += "  " + dim.Render(updated)
	}
	return line
}

func localStateCluster(r row) string {
	if r.kind == "queue" && !r.structured && !r.active {
		return "-"
	}
	status := localStatusLabel(r.status)
	parts := []string{}
	if status == "-" {
		parts = append(parts, "-")
	} else {
		parts = append(parts, strings.TrimSpace(glyph(r.status)+" "+status))
	}
	if len(r.workspaceShells) > 0 {
		parts = append(parts, "⧉")
	}
	return strings.Join(parts, " ")
}

func localStatusLabel(status string) string {
	switch status {
	case "not-running":
		return "stopped"
	case "stopped":
		return "stopped"
	case "waiting_for_input":
		return "waiting"
	case "ready_for_review":
		return "review"
	case "pr_opened":
		return "pr"
	case "":
		return "-"
	default:
		return status
	}
}

func linearStateLabel(r row) string {
	if strings.TrimSpace(r.linearState) != "" {
		return r.linearState
	}
	if r.linear != "" || r.queueIssue != "" {
		return "-"
	}
	return "-"
}

func queueTeam(r row) string {
	for _, line := range strings.Split(r.detail, "\n") {
		if strings.HasPrefix(line, "team: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "team: "))
		}
	}
	return "-"
}

func baseOrEmpty(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(path)
}

func (m *model) commitAction() error {
	r, hasRow := m.current()
	value := strings.TrimSpace(m.input)
	switch m.mode {
	case "scratch":
		if value == "" {
			value = "."
		}
		m.target = value
		m.create = "scratch"
		m.mode = "agentPick"
		m.agentSelected = 0
		m.input = ""
		return nil
	case "start":
		if value == "" {
			return fmt.Errorf("enter a Linear issue key or task title")
		}
		if issuePattern.MatchString(value) && os.Getenv("LINEAR_API_KEY") == "" {
			return fmt.Errorf("LINEAR_API_KEY is not set in this cmux process")
		}
		m.target = value
		m.create = "start"
		m.createRepo = defaultRepoPath()
		m.agentMode = agentModeImplementation
		m.mode = "repoPick"
		m.repoSelected = 0
		m.input = ""
		return nil
	case "repoPath":
		if value == "" {
			return fmt.Errorf("enter a repository path")
		}
		root, err := normalizeRepoPath(value)
		if err != nil {
			return err
		}
		if err := config.RememberRepo(root); err != nil {
			return err
		}
		m.createRepo = root
		m.mode = "agentPick"
		m.agentSelected = 0
		m.input = ""
		return nil
	case "agentCustom":
		if value == "" {
			return fmt.Errorf("enter an agent command")
		}
		return m.createWithAgent(value)
	case "editorCustom":
		if value == "" {
			return fmt.Errorf("enter an editor command")
		}
		return m.openEditor(value)
	case "sshTarget":
		if value == "" {
			return fmt.Errorf("enter an SSH target")
		}
		if err := config.RememberSSHTarget(value); err != nil {
			return err
		}
		m.mode = "workspaceOpen"
		m.input = ""
		pending := m.pendingAction
		m.pendingAction = ""
		if pending == "" || pending == "setSSH" {
			m.message = "SSH target: " + value
			return nil
		}
		return m.runWorkspaceAction(pending)
	case "rename":
		if !hasRow || !r.active || value == "" {
			return fmt.Errorf("no active session selected")
		}
		next, err := tmux.Rename(r.session, value)
		if err == nil && isAgentRow(r) {
			_, _ = state.Update(r.id, func(s types.AgentSession) types.AgentSession {
				s.TmuxSession = next
				return s
			})
		}
		return err
	case "title":
		if !hasRow || value == "" {
			return fmt.Errorf("no session selected")
		}
		if r.active {
			if err := tmux.SetTitle(r.session, value); err != nil {
				return err
			}
		} else if !isAgentRow(r) {
			return fmt.Errorf("session is not running")
		}
		if isAgentRow(r) && value != "" {
			_, _ = state.Update(r.id, func(s types.AgentSession) types.AgentSession {
				s.Title = value
				return s
			})
		}
		return nil
	case "linearMove":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		if value == "" {
			return fmt.Errorf("enter a Linear state name or id")
		}
		s, err := agent.MoveLinear(r.id, value)
		if err != nil {
			return err
		}
		m.message = "Linear: " + valueOr(s.Linear.State, s.Linear.StateID)
		return nil
	case "delete":
		if !hasRow {
			return fmt.Errorf("no session selected")
		}
		if isAgentRow(r) {
			return agent.Delete(r.id)
		}
		return tmux.Kill(r.session)
	case "close":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		return agent.Close(r.id)
	case "cleanup":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		force := value == r.id
		if !force && strings.ToLower(value) != "y" {
			return nil
		}
		return agent.CleanupWorktree(r.id, force)
	case "reset":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		if value != r.id {
			return fmt.Errorf("type %s to reset workspace", r.id)
		}
		return agent.ResetWorkspace(r.id)
	case "statusSummary":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		_, err := agent.SetStatus(r.id, m.pending, value)
		return err
	default:
		return nil
	}
}

func (m *model) createWithAgent(agentCommand string) error {
	switch m.create {
	case "scratch":
		target := m.target
		if target == "" {
			target = "."
		}
		abs, _ := filepath.Abs(target)
		title := filepath.Base(abs)
		_, err := agent.Start(agent.StartOptions{Cwd: abs, Scratch: true, Title: title, Agent: agentCommand, AgentSet: true, Profile: types.ProfileGeneral, ProfileSet: true})
		return err
	case "start":
		target := strings.TrimSpace(m.target)
		if target == "" {
			return fmt.Errorf("enter a Linear issue key or task title")
		}
		cwd := valueOr(m.createRepo, ".")
		profile := profileForAgentMode(m.agentMode)
		if issuePattern.MatchString(target) {
			_, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand, AgentSet: true, Profile: profile, ProfileSet: true})
			return err
		}
		_, err := agent.Start(agent.StartOptions{Cwd: cwd, Title: target, Worktree: true, Agent: agentCommand, AgentSet: true, Profile: profile, ProfileSet: true})
		return err
	case "queue":
		targets := splitTargets(m.target)
		if len(targets) == 0 {
			return fmt.Errorf("select a Linear issue")
		}
		if len(targets) > queue.BatchLimit {
			return fmt.Errorf("batch start is capped at %d issues", queue.BatchLimit)
		}
		cwd := valueOr(m.createRepo, ".")
		var first types.AgentSession
		for _, target := range targets {
			s, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand, AgentSet: true, Profile: profileForAgentMode(m.agentMode), ProfileSet: true})
			if err != nil {
				return err
			}
			if first.ID == "" {
				first = s
			}
		}
		if len(targets) == 1 && first.TmuxSession != "" {
			m.chosen = first.TmuxSession
		}
		return nil
	case "queueFresh":
		targets := splitTargets(m.target)
		if len(targets) == 0 {
			return fmt.Errorf("select a Linear issue")
		}
		if len(targets) > queue.BatchLimit {
			return fmt.Errorf("batch start is capped at %d issues", queue.BatchLimit)
		}
		cwd := valueOr(m.createRepo, ".")
		var first types.AgentSession
		for _, target := range targets {
			s, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand, AgentSet: true, Profile: profileForAgentMode(m.agentMode), ProfileSet: true, Fresh: true})
			if err != nil {
				return err
			}
			if first.ID == "" {
				first = s
			}
		}
		if len(targets) == 1 && first.TmuxSession != "" {
			m.chosen = first.TmuxSession
		}
		return nil
	case "queuePlan":
		targets := splitTargets(m.target)
		if len(targets) == 0 {
			return fmt.Errorf("select a Linear issue")
		}
		if len(targets) > queue.BatchLimit {
			return fmt.Errorf("batch start is capped at %d issues", queue.BatchLimit)
		}
		cwd := valueOr(m.createRepo, ".")
		var first types.AgentSession
		for _, target := range targets {
			s, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand, AgentSet: true, Profile: types.ProfilePlan, ProfileSet: true})
			if err != nil {
				return err
			}
			if first.ID == "" {
				first = s
			}
		}
		if len(targets) == 1 && first.TmuxSession != "" {
			m.chosen = first.TmuxSession
		}
		return nil
	case "freshExisting":
		target := strings.TrimSpace(m.target)
		if target == "" {
			return fmt.Errorf("select a structured agent session")
		}
		if _, err := agent.Fresh(target, agentCommand, profileForAgentMode(m.agentMode)); err != nil {
			return err
		}
		m.mode = "browse"
		m.message = "Started fresh agent"
		m.reloadRows(target)
		return nil
	default:
		return fmt.Errorf("no pending session creation")
	}
}

func (m *model) chooseAgent() error {
	if len(agentChoices) == 0 {
		return fmt.Errorf("no agent choices configured")
	}
	if m.agentSelected < 0 {
		m.agentSelected = 0
	}
	if m.agentSelected >= len(agentChoices) {
		m.agentSelected = len(agentChoices) - 1
	}
	choice := agentChoices[m.agentSelected]
	if choice.custom {
		m.mode = "agentCustom"
		m.input = ""
		m.message = ""
		return nil
	}
	if err := m.createWithAgent(choice.command); err != nil {
		return err
	}
	if m.chosen != "" {
		return nil
	}
	m.afterCreate()
	return nil
}

func (m *model) afterCreate() {
	m.mode = "browse"
	m.input = ""
	m.create = ""
	m.createRepo = ""
	m.createAgent = ""
	m.createCustom = false
	m.target = ""
	m.agentSelected = 0
	m.message = ""
	m.selectedQueue = map[string]bool{}
	rows, err := loadRows(m.fullQueue, false)
	if err == nil {
		m.rows = rows
		m.applyFilter()
	}
}

func (m *model) reloadRows(selectID string) {
	rows, err := loadRows(m.fullQueue, false)
	if err != nil {
		m.message = err.Error()
		return
	}
	m.rows = rows
	m.applyFilter()
	if selectID != "" {
		m.selectRow(selectID)
	}
}

func (m *model) selectRow(id string) {
	for i, row := range m.filtered {
		if row.id == id || row.queueIssue == id {
			m.selected = i
			return
		}
	}
}

func (m model) agentPicker() string {
	var b strings.Builder
	b.WriteString(cyan.Render("agent") + dim.Render("  ↑↓/jk select · enter create · esc cancel") + "\n")
	for i, choice := range agentChoices {
		line := fmt.Sprintf("%-8s %s", choice.label, dim.Render(choice.description))
		if i == m.agentSelected {
			b.WriteString(green.Render("▌ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) actionMenu() string {
	r, ok := m.current()
	if !ok {
		return dim.Render("no row selected")
	}
	choices := actionsFor(r, m.fullQueue)
	var b strings.Builder
	b.WriteString(cyan.Render("actions") + dim.Render("  "+r.id+" · ↑↓/jk select · enter run · esc cancel") + "\n")
	lastSection := ""
	for i, choice := range choices {
		section := actionSection(choice.id)
		if section != "" && section != lastSection {
			lastSection = section
			b.WriteString(dim.Render("  "+section) + "\n")
		}
		line := fmt.Sprintf("%-22s %s", choice.label, dim.Render(choice.description))
		if i == m.actionSelected {
			b.WriteString(green.Render("▌ "+line) + "\n")
		} else if choice.danger {
			b.WriteString("  " + red.Render(line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func actionSection(id string) string {
	switch id {
	case "agentOpen", "startSelected":
		return "Agent"
	case "workspaceOpen":
		return "Workspace"
	case "briefPublish":
		return "Brief"
	case "linearMove":
		return "Linear"
	case "detail":
		return "Inspect"
	case "close", "forget", "title", "rename":
		return "Session"
	case "deleteWorktree", "delete":
		return "Recovery"
	default:
		return ""
	}
}

func (m model) newMenu() string {
	choices := newChoices()
	var b strings.Builder
	b.WriteString(cyan.Render("new work") + dim.Render("  ↑↓/jk select · enter continue · esc cancel") + "\n")
	for i, choice := range choices {
		line := fmt.Sprintf("%-18s %s", choice.label, dim.Render(choice.description))
		if i == m.newSelected {
			b.WriteString(green.Render("▌ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) repoPicker() string {
	choices := repoChoices(m.createRepo)
	var b strings.Builder
	b.WriteString(cyan.Render("repository") + dim.Render("  ↑↓/jk select · enter continue · esc cancel") + "\n")
	for i, choice := range choices {
		desc := choice.description
		if choice.path != "" {
			desc = strings.TrimSpace(strings.Join(nonEmpty([]string{desc, choice.path}), " · "))
		}
		line := fmt.Sprintf("%-18s %s", choice.label, dim.Render(desc))
		if i == m.repoSelected {
			b.WriteString(green.Render("▌ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) editorPicker() string {
	choices := editorChoices()
	var b strings.Builder
	b.WriteString(cyan.Render("editor") + dim.Render("  ↑↓/jk select · enter open · esc cancel") + "\n")
	for i, choice := range choices {
		command := choice.command
		if command == "" {
			command = "type command"
		}
		line := fmt.Sprintf("%-12s %-16s %s", choice.label, command, dim.Render(choice.description))
		if i == m.editorSelected {
			b.WriteString(green.Render("▌ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) agentOpenPanel() string {
	r, ok := m.current()
	if !ok {
		return dim.Render("no row selected")
	}
	choices := agentActionChoices(r, m.agentMode)
	selected := m.agentPick
	if selected < 0 || selected >= len(choices) {
		selected = 0
	}
	var b strings.Builder
	b.WriteString(cyan.Render("agent") + dim.Render("  "+r.id+" · ←/→/h/l mode · ↑↓/jk select · enter open/start · esc cancel") + "\n")
	b.WriteString("  " + renderAgentTabs(r, m.agentMode) + "\n")
	descWidth := m.width - 32
	if descWidth < 24 {
		descWidth = 24
	}
	for i, choice := range choices {
		line := fmt.Sprintf("%-18s %s", choice.label, dim.Render(format.Trunc(choice.description, descWidth)))
		switch {
		case i == selected && choice.disabled:
			b.WriteString(red.Render("▌ "+line) + "\n")
		case i == selected:
			b.WriteString(green.Render("▌ "+line) + "\n")
		case choice.disabled:
			b.WriteString("  " + dim.Render(line) + "\n")
		default:
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderAgentTabs(r row, selected int) string {
	tabs := make([]string, 0, len(agentModeNames))
	for i, name := range agentModeNames {
		mode := agentModeValues[i]
		if mode == selected {
			tabs = append(tabs, activeTab.Render(name))
		} else {
			tabs = append(tabs, inactiveTab.Render(name))
		}
	}
	return strings.Join(tabs, " ")
}

func (m model) workspaceOpenPanel() string {
	r, ok := m.current()
	if !ok {
		return dim.Render("no row selected")
	}
	choices := workspaceChoices(r, m.workspaceMode)
	selected := m.workspacePick
	if selected < 0 || selected >= len(choices) {
		selected = 0
	}
	var b strings.Builder
	b.WriteString(cyan.Render("open workspace") + dim.Render("  "+r.id+" · ←/→/h/l mode · ↑↓/jk select · enter run/show · esc cancel") + "\n")
	b.WriteString("  " + renderWorkspaceTabs(m.workspaceMode) + "\n")
	descWidth := m.width - 32
	if descWidth < 24 {
		descWidth = 24
	}
	for i, choice := range choices {
		line := fmt.Sprintf("%-22s %s", choice.label, dim.Render(format.Trunc(choice.description, descWidth)))
		switch {
		case i == selected && choice.disabled:
			b.WriteString(red.Render("▌ "+line) + "\n")
		case i == selected:
			b.WriteString(green.Render("▌ "+line) + "\n")
		case choice.disabled:
			b.WriteString("  " + dim.Render(line) + "\n")
		default:
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderWorkspaceTabs(selected int) string {
	tabs := make([]string, 0, len(workspaceModeNames))
	for i, name := range workspaceModeNames {
		if i == selected {
			tabs = append(tabs, activeTab.Render(name))
		} else {
			tabs = append(tabs, inactiveTab.Render(name))
		}
	}
	return strings.Join(tabs, " ")
}

func (m model) detailView() string {
	r, ok := m.current()
	if !ok {
		return dim.Render("no session selected")
	}
	var b strings.Builder
	b.WriteString(bold.Render(cyan.Render(r.id)) + "  " + r.title + "\n")
	b.WriteString(dim.Render(r.group+" · "+r.status+" · updated "+r.updated) + "\n\n")
	for _, item := range detailItems(r) {
		label := fmt.Sprintf("%-10s", item[0]+":")
		b.WriteString(cyan.Render(label) + item[1] + "\n")
	}
	b.WriteString("\n" + dim.Render("── brief ───────────────────────────────────────────") + "\n")
	writeDetailSection(&b, "Current", r.currentState)
	writeDetailSection(&b, "Next", r.nextAction)
	writeDetailSection(&b, "Blockers", r.blockers)
	writeDetailSection(&b, "Tests", r.testsRun)
	writeDetailSection(&b, "Review", r.reviewSummary)
	if len(r.workspaceShells) > 0 {
		b.WriteString("\n" + cyan.Render("Workspace terminals:") + "\n")
		for _, name := range r.workspaceShells {
			b.WriteString("  " + name + "\n")
		}
	}
	if r.preview != "" {
		b.WriteString("\n" + dim.Render("── recent terminal ─────────────────────────────────") + "\n")
		for _, line := range tail(strings.Split(r.preview, "\n"), previewHeight(m.height)) {
			b.WriteString("▎ " + line + "\n")
		}
	}
	b.WriteString("\n" + dim.Render("enter open  . actions  esc back"))
	if m.message != "" {
		b.WriteString("\n" + renderMessage(m.message))
	}
	return b.String()
}

func (m model) prompt() string {
	switch m.mode {
	case "filter":
		return renderPromptInput("filter", m.filter) + dim.Render("enter apply · esc cancel")
	case "actionMenu":
		return m.actionMenu()
	case "newMenu":
		return m.newMenu()
	case "scratch":
		return renderPromptInput("scratch path", m.input) + dim.Render("enter create · esc cancel")
	case "start":
		return renderPromptInput("issue/task", m.input) + dim.Render("REB-123 starts Linear work · other text creates task worktree")
	case "agentPick":
		return m.agentPicker()
	case "repoPick":
		return m.repoPicker()
	case "repoPath":
		return renderPromptInput("repo path", m.input) + dim.Render("queue worktrees will be created from this repo")
	case "agentOpen":
		return m.agentOpenPanel()
	case "workspaceOpen":
		return m.workspaceOpenPanel()
	case "busy":
		return renderMessage(m.message)
	case "editorPick":
		return m.editorPicker()
	case "editorCustom":
		return renderPromptInput("editor command", m.input) + dim.Render("examples: cursor, code, zed, open -a Cursor")
	case "sshTarget":
		return renderPromptInput("ssh target", m.input) + dim.Render("saved for remote workspace commands · enter continue · esc cancel")
	case "agentCustom":
		return renderPromptInput("agent command", m.input) + dim.Render("enter create · esc cancel")
	case "rename":
		return renderPromptInput("session name", m.input) + dim.Render("enter save · esc cancel")
	case "title":
		return renderPromptInput("display name", m.input) + dim.Render("enter save · esc cancel")
	case "linearMove":
		return renderPromptInput("Linear state", m.input) + dim.Render("state name or id · enter move · esc cancel")
	case "delete":
		if r, ok := m.current(); ok && !isAgentRow(r) {
			return m.confirmPrompt("Kill session", []string{"Close this unmanaged tmux session."})
		}
		return m.confirmPrompt("Forget session", []string{"Stop the agent session.", "Keep the workspace and branch.", "Do not change Linear."})
	case "close":
		return m.confirmPrompt("Stop and clean up", []string{"Stop the agent session.", "Remove a clean cmux-owned worktree.", "Refuse if workspace is dirty or brief has unpublished changes."})
	case "cleanup":
		return red.Render("delete worktree? enter y if clean, or type session id to force  ") + dim.Render("dirty worktrees are refused unless forced · esc cancel")
	case "reset":
		if r, ok := m.current(); ok {
			return red.Render("reset workspace? type "+r.id+" then enter  ") + dim.Render("runs git reset --hard and git clean -fd · esc cancel")
		}
		return red.Render("reset workspace? type session id then enter  ") + dim.Render("esc cancel")
	case "statusPick":
		return strings.Join([]string{
			cyan.Render("set status"),
			"  1 running",
			"  2 waiting_for_input",
			"  3 blocked",
			"  4 tests_failed",
			"  5 ready_for_review",
			"  6 pr_opened",
			"  7 done",
			"  8 crashed",
			dim.Render("press 1-8 · esc cancel"),
		}, "\n")
	case "statusSummary":
		return renderPromptInput("summary for "+string(m.pending), m.input) + dim.Render("enter save · esc cancel")
	default:
		return ""
	}
}

func renderPromptInput(label, value string) string {
	return cyan.Render(label+"  "+value) + cyan.Render("▌  ")
}

func (m model) confirmPrompt(title string, lines []string) string {
	r, ok := m.current()
	target := ""
	if ok {
		target = "  " + dim.Render(r.id)
	}
	var b strings.Builder
	b.WriteString(red.Render(title) + target + "\n")
	for _, line := range lines {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString(dim.Render("enter/y confirm · esc/n cancel"))
	return strings.TrimRight(b.String(), "\n")
}

func isConfirmMode(mode string) bool {
	switch mode {
	case "delete", "close":
		return true
	default:
		return false
	}
}

func (m *model) move(delta int) {
	if len(m.filtered) == 0 {
		m.selected = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
}

func (m *model) moveAgent(delta int) {
	if len(agentChoices) == 0 {
		m.agentSelected = 0
		return
	}
	m.agentSelected += delta
	if m.agentSelected < 0 {
		m.agentSelected = len(agentChoices) - 1
	}
	if m.agentSelected >= len(agentChoices) {
		m.agentSelected = 0
	}
}

func (m *model) moveRepo(delta int) {
	choices := repoChoices(m.createRepo)
	if len(choices) == 0 {
		m.repoSelected = 0
		return
	}
	m.repoSelected += delta
	if m.repoSelected < 0 {
		m.repoSelected = len(choices) - 1
	}
	if m.repoSelected >= len(choices) {
		m.repoSelected = 0
	}
}

func (m *model) moveEditor(delta int) {
	choices := editorChoices()
	if len(choices) == 0 {
		m.editorSelected = 0
		return
	}
	m.editorSelected += delta
	if m.editorSelected < 0 {
		m.editorSelected = len(choices) - 1
	}
	if m.editorSelected >= len(choices) {
		m.editorSelected = 0
	}
}

func (m *model) moveAgentMode(delta int) {
	index := agentModeIndex(m.agentMode) + delta
	if index < 0 {
		index = len(agentModeValues) - 1
	}
	if index >= len(agentModeValues) {
		index = 0
	}
	m.agentMode = agentModeValues[index]
	m.agentPick = 0
	m.message = ""
}

func (m *model) moveAgentAction(delta int) {
	r, ok := m.current()
	if !ok {
		m.agentPick = 0
		return
	}
	choices := agentActionChoices(r, m.agentMode)
	if len(choices) == 0 {
		m.agentPick = 0
		return
	}
	m.agentPick += delta
	if m.agentPick < 0 {
		m.agentPick = len(choices) - 1
	}
	if m.agentPick >= len(choices) {
		m.agentPick = 0
	}
}

func (m *model) moveWorkspaceMode(delta int) {
	m.workspaceMode += delta
	if m.workspaceMode < 0 {
		m.workspaceMode = len(workspaceModeNames) - 1
	}
	if m.workspaceMode >= len(workspaceModeNames) {
		m.workspaceMode = 0
	}
	m.workspacePick = 0
	m.message = ""
}

func (m *model) moveWorkspaceChoice(delta int) {
	r, ok := m.current()
	if !ok {
		m.workspacePick = 0
		return
	}
	choices := workspaceChoices(r, m.workspaceMode)
	if len(choices) == 0 {
		m.workspacePick = 0
		return
	}
	m.workspacePick += delta
	if m.workspacePick < 0 {
		m.workspacePick = len(choices) - 1
	}
	if m.workspacePick >= len(choices) {
		m.workspacePick = 0
	}
}

func (m *model) moveAction(delta int) {
	r, ok := m.current()
	if !ok {
		m.actionSelected = 0
		return
	}
	choices := actionsFor(r, m.fullQueue)
	if len(choices) == 0 {
		m.actionSelected = 0
		return
	}
	m.actionSelected += delta
	if m.actionSelected < 0 {
		m.actionSelected = len(choices) - 1
	}
	if m.actionSelected >= len(choices) {
		m.actionSelected = 0
	}
}

func (m *model) moveNew(delta int) {
	choices := newChoices()
	m.newSelected += delta
	if m.newSelected < 0 {
		m.newSelected = len(choices) - 1
	}
	if m.newSelected >= len(choices) {
		m.newSelected = 0
	}
}

func (m *model) chooseNew() {
	choices := newChoices()
	if m.newSelected < 0 || m.newSelected >= len(choices) {
		m.newSelected = 0
	}
	switch choices[m.newSelected].id {
	case "scratch":
		m.mode = "scratch"
		wd, _ := os.Getwd()
		m.input = wd
	case "issueTask":
		m.mode = "start"
		m.input = ""
	case "workspace":
		name, err := m.openWorkspaceSession(true)
		if err != nil {
			m.message = err.Error()
			m.mode = "browse"
			return
		}
		m.chosen = name
	}
}

func (m *model) chooseAction() (tea.Cmd, error) {
	r, ok := m.current()
	if !ok {
		return nil, fmt.Errorf("no row selected")
	}
	choices := actionsFor(r, m.fullQueue)
	if len(choices) == 0 {
		return nil, fmt.Errorf("no actions available")
	}
	if m.actionSelected < 0 || m.actionSelected >= len(choices) {
		m.actionSelected = 0
	}
	switch choices[m.actionSelected].id {
	case "agentOpen":
		m.mode = "agentOpen"
		m.agentMode = autoAgentMode(r)
		m.agentPick = 0
		m.message = ""
	case "startSelected":
		return nil, m.startQueueFlow(true, false)
	case "detail":
		m.mode = "detail"
	case "path":
		m.showWorkspacePath()
		m.mode = "browse"
	case "workspaceOpen":
		workspace := valueOr(r.workspace, r.repo)
		if workspace == "" {
			return nil, fmt.Errorf("no workspace path recorded")
		}
		m.mode = "workspaceOpen"
		m.workspaceMode = workspaceModeTerminal
		m.workspacePick = 0
		m.message = ""
	case "briefPublish":
		if !isAgentRow(r) {
			return nil, fmt.Errorf("select a structured agent session")
		}
		m.mode = "busy"
		m.message = "Publishing brief..."
		return m.runLongAction("briefPublish", r), nil
	case "close":
		if !isAgentRow(r) {
			return nil, fmt.Errorf("select a structured agent session")
		}
		m.mode = "close"
		m.input = ""
	case "forget":
		m.mode = "delete"
		m.input = ""
	case "rename":
		if !r.active {
			return nil, fmt.Errorf("session is not running")
		}
		m.mode = "rename"
		m.input = tmux.Child(r.session)
	case "title":
		if !r.active && !isAgentRow(r) {
			return nil, fmt.Errorf("session is not running")
		}
		m.mode = "title"
		m.input = r.title
	case "linearMove":
		if !isAgentRow(r) || r.linear == "" {
			return nil, fmt.Errorf("select a Linear-backed session")
		}
		m.mode = "linearMove"
		m.input = ""
	case "delete":
		m.mode = "delete"
		m.input = ""
	case "deleteWorktree":
		m.mode = "cleanup"
		m.input = ""
	}
	return nil, nil
}

func (m *model) runLongAction(id string, r row) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch id {
		case "briefPublish":
			_, _, err = agent.PublishBrief(r.id)
		case "close":
			err = agent.Close(r.id)
		default:
			err = fmt.Errorf("unknown action: %s", id)
		}
		return actionDoneMsg{id: id, selectID: r.id, err: err}
	}
}

func preflightSubmitClose(r row) error {
	if r.workspace != "" {
		dirty, summary := gitx.StatusSummary(r.workspace)
		if dirty {
			return agent.WorktreeDirtyError{Path: r.workspace, Summary: summary}
		}
	}
	if r.workspace != "" && r.repo != "" && r.workspace != r.repo && !isCmuxOwnedSeparateWorktree(r) {
		return fmt.Errorf("worktree is outside cmux worktrees; close will not remove it: %s", r.workspace)
	}
	return nil
}

func isCmuxOwnedSeparateWorktree(r row) bool {
	if strings.TrimSpace(r.workspace) == "" || r.workspace == r.repo {
		return false
	}
	rel, err := filepath.Rel(home.WorktreesDir(), r.workspace)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func (m *model) primaryAction() error {
	r, ok := m.current()
	if !ok {
		return nil
	}
	if r.kind == "queue" && !r.active && !r.structured {
		m.mode = "agentOpen"
		m.agentMode = autoAgentMode(r)
		m.agentPick = 0
		m.message = ""
		return nil
	}
	if !r.active || !tmux.Has(r.session) {
		m.mode = "actionMenu"
		m.actionSelected = 0
		return nil
	}
	m.chosen = r.session
	return nil
}

func (m *model) startQueueFlow(useSelected bool, scoping bool) error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no row selected")
	}
	selected := []string{}
	if useSelected {
		selected = m.selectedQueueIssues()
	}
	if len(selected) == 0 && r.queueIssue != "" {
		selected = []string{r.queueIssue}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no queue issue selected")
	}
	m.create = "queue"
	if scoping {
		m.create = "queuePlan"
		m.agentMode = agentModePlan
	} else {
		m.agentMode = agentModeImplementation
	}
	m.createRepo = r.repo
	m.target = strings.Join(selected, ",")
	m.mode = "repoPick"
	m.repoSelected = 0
	m.message = ""
	return nil
}

func (m *model) chooseRepo() error {
	choices := repoChoices(m.createRepo)
	if len(choices) == 0 {
		m.mode = "repoPath"
		m.input = defaultRepoPath()
		return nil
	}
	if m.repoSelected < 0 {
		m.repoSelected = 0
	}
	if m.repoSelected >= len(choices) {
		m.repoSelected = len(choices) - 1
	}
	choice := choices[m.repoSelected]
	if choice.custom {
		m.mode = "repoPath"
		m.input = defaultRepoPath()
		return nil
	}
	root, err := normalizeRepoPath(choice.path)
	if err != nil {
		return err
	}
	m.createRepo = root
	if m.createCustom {
		m.createCustom = false
		m.mode = "agentCustom"
		m.input = ""
		m.message = ""
		return nil
	}
	if m.createAgent != "" {
		command := m.createAgent
		m.createAgent = ""
		if err := m.createWithAgent(command); err != nil {
			return err
		}
		if m.chosen == "" {
			m.afterCreate()
		}
		return nil
	}
	m.mode = "agentPick"
	m.agentSelected = 0
	m.input = ""
	return nil
}

func (m *model) chooseEditor() error {
	choices := editorChoices()
	if len(choices) == 0 {
		m.mode = "editorCustom"
		m.input = ""
		return nil
	}
	if m.editorSelected < 0 {
		m.editorSelected = 0
	}
	if m.editorSelected >= len(choices) {
		m.editorSelected = len(choices) - 1
	}
	choice := choices[m.editorSelected]
	if choice.custom {
		m.mode = "editorCustom"
		m.input = ""
		m.message = ""
		return nil
	}
	return m.openEditor(choice.command)
}

func (m *model) chooseAgentAction() error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no row selected")
	}
	choices := agentActionChoices(r, m.agentMode)
	if len(choices) == 0 {
		return fmt.Errorf("no agent actions available")
	}
	if m.agentPick < 0 {
		m.agentPick = 0
	}
	if m.agentPick >= len(choices) {
		m.agentPick = len(choices) - 1
	}
	choice := choices[m.agentPick]
	if choice.disabled {
		return fmt.Errorf("%s", choice.description)
	}
	if choice.custom {
		if r.structured {
			m.create = "freshExisting"
			m.target = r.id
			m.mode = "agentCustom"
			m.input = ""
			m.message = ""
			return nil
		}
		if err := m.prepareAgentStart(); err != nil {
			return err
		}
		m.createCustom = true
		m.mode = "repoPick"
		m.repoSelected = 0
		m.message = ""
		return nil
	}
	switch choice.id {
	case "openExisting":
		if !r.active || !tmux.Has(r.session) {
			return fmt.Errorf("session is not running")
		}
		m.chosen = r.session
	case "stopExisting":
		return m.stopCurrent()
	case "restartExisting":
		return m.restartCurrent()
	case "startFreshExisting":
		return m.startFreshCurrent(choice.command)
	case "startAgent":
		return m.startAgentFromPanel(choice.command)
	default:
		return fmt.Errorf("unknown agent action: %s", choice.id)
	}
	return nil
}

func (m *model) startAgentFromPanel(command string) error {
	if err := m.prepareAgentStart(); err != nil {
		return err
	}
	m.createAgent = command
	m.mode = "repoPick"
	m.repoSelected = 0
	m.message = ""
	return nil
}

func (m *model) startFreshCurrent(command string) error {
	r, ok := m.current()
	if !ok || !isAgentRow(r) {
		return fmt.Errorf("select a structured agent session")
	}
	_, err := agent.Fresh(r.id, command, profileForAgentMode(m.agentMode))
	if err != nil {
		return err
	}
	m.mode = "browse"
	m.message = "Started fresh agent"
	m.reloadRows(r.id)
	return nil
}

func (m *model) prepareAgentStart() error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no row selected")
	}
	if r.kind != "queue" || r.structured {
		return fmt.Errorf("custom agent start is only available before a session exists")
	}
	issue := r.queueIssue
	if issue == "" {
		issue = r.id
	}
	if issue == "" {
		return fmt.Errorf("no Linear issue selected")
	}
	mode := resolvedAgentMode(r, m.agentMode)
	m.agentMode = mode
	m.create = "queue"
	if mode == agentModeFresh {
		m.create = "queueFresh"
	} else if mode == agentModePlan {
		m.create = "queuePlan"
	} else {
		m.create = "queue"
	}
	m.createRepo = valueOr(r.repo, ".")
	m.target = issue
	return nil
}

func (m *model) chooseWorkspace() error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no row selected")
	}
	choices := workspaceChoices(r, m.workspaceMode)
	if len(choices) == 0 {
		return fmt.Errorf("no workspace actions available")
	}
	if m.workspacePick < 0 {
		m.workspacePick = 0
	}
	if m.workspacePick >= len(choices) {
		m.workspacePick = len(choices) - 1
	}
	choice := choices[m.workspacePick]
	if choice.disabled && choice.id != "setSSH" {
		m.pendingAction = choice.id
		m.mode = "sshTarget"
		m.input = defaultSSHTarget()
		m.message = ""
		return nil
	}
	return m.runWorkspaceAction(choice.id)
}

func (m *model) runWorkspaceAction(id string) error {
	if strings.HasPrefix(id, "editorCommand:") {
		return m.openEditor(strings.TrimPrefix(id, "editorCommand:"))
	}
	switch id {
	case "workspaceCurrent":
		name, err := m.openWorkspaceSession(false)
		if err != nil {
			return err
		}
		m.chosen = name
	case "workspaceNew":
		name, err := m.openWorkspaceSession(true)
		if err != nil {
			return err
		}
		m.chosen = name
	case "copyCD":
		path, err := m.workspacePath()
		if err != nil {
			return err
		}
		m.message = "Command: cd " + shellQuote(path)
	case "editorCustom":
		m.mode = "editorCustom"
		m.input = ""
		m.message = ""
	case "remoteCursor", "remoteCode", "remoteSSH":
		target := defaultSSHTarget()
		if target == "" {
			m.pendingAction = id
			m.mode = "sshTarget"
			m.input = ""
			m.message = ""
			return nil
		}
		path, err := m.workspacePath()
		if err != nil {
			return err
		}
		m.message = "Command: " + remoteWorkspaceCommand(id, target, path)
	case "setSSH":
		m.pendingAction = id
		m.mode = "sshTarget"
		m.input = defaultSSHTarget()
		m.message = ""
	default:
		return fmt.Errorf("unknown workspace action: %s", id)
	}
	return nil
}

type repoChoice struct {
	label       string
	path        string
	description string
	custom      bool
}

func repoChoices(preferred string) []repoChoice {
	choices := []repoChoice{}
	add := func(label, path, description string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		for _, choice := range choices {
			if choice.path == path {
				return
			}
		}
		choices = append(choices, repoChoice{label: label, path: path, description: description})
	}
	if preferred != "" {
		add(filepath.Base(preferred), preferred, "preset default")
	}
	if cfg, err := config.LoadOrDefault(); err == nil {
		for _, repo := range cfg.Repos {
			add(valueOr(repo.Name, filepath.Base(repo.Path)), repo.Path, "previously used")
		}
		add(filepath.Base(cfg.DefaultRepoPath), cfg.DefaultRepoPath, "default")
	}
	current := defaultRepoPath()
	add(filepath.Base(current), current, "current directory")
	choices = append(choices, repoChoice{label: "Custom path", custom: true})
	return choices
}

func editorChoices() []editorChoice {
	choices := []editorChoice{}
	add := func(label, command, description string) {
		command = strings.TrimSpace(command)
		if command == "" {
			return
		}
		for _, choice := range choices {
			if choice.command == command {
				return
			}
		}
		choices = append(choices, editorChoice{label: label, command: command, description: description})
	}
	if cfg, err := config.LoadOrDefault(); err == nil {
		add(editorLabel(cfg.DefaultEditorCommand), cfg.DefaultEditorCommand, "default")
		for _, command := range cfg.EditorCommands {
			add(editorLabel(command), command, "previously used")
		}
	}
	add("CMUX_EDITOR", os.Getenv("CMUX_EDITOR"), "environment")
	add("Cursor", "cursor", "open in Cursor")
	add("VS Code", "code", "open in Visual Studio Code")
	add("Zed", "zed", "open in Zed")
	choices = append(choices, editorChoice{label: "Custom", description: "type command or executable path", custom: true})
	return choices
}

func editorLabel(command string) string {
	command = strings.TrimSpace(command)
	switch filepath.Base(command) {
	case "cursor":
		return "Cursor"
	case "code":
		return "VS Code"
	case "zed":
		return "Zed"
	default:
		return format.Trunc(command, 12)
	}
}

func agentActionChoices(r row, mode int) []agentActionChoice {
	if r.structured {
		resolved := agentModeLabel(resolvedAgentMode(r, mode))
		agentCommand := valueOr(r.agent, "claude")
		freshChoices := freshAgentChoices(agentCommand, resolved)
		if r.active {
			choices := []agentActionChoice{
				{id: "openExisting", label: "Open agent", description: "attach to running agent"},
				{id: "stopExisting", label: "Stop agent", description: "stop tmux agent, keep session and workspace"},
				{id: "restartExisting", label: "Restart agent", description: "restart in the existing workspace"},
			}
			return append(choices, freshChoices...)
		}
		choices := []agentActionChoice{
			{id: "restartExisting", label: "Restart agent", description: "restart in the existing workspace"},
		}
		return append(choices, freshChoices...)
	}
	if r.kind != "queue" {
		if r.active {
			return []agentActionChoice{{id: "openExisting", label: "Current session", description: "attach to running session"}}
		}
		return []agentActionChoice{{id: "openExisting", label: "Current session", description: "session is not running", disabled: true}}
	}
	if r.queueIssue == "" {
		return []agentActionChoice{{id: "startAgent", label: "Agent", description: "no Linear issue selected", disabled: true}}
	}
	resolved := agentModeLabel(resolvedAgentMode(r, mode))
	choices := make([]agentActionChoice, 0, len(agentChoices))
	for _, choice := range agentChoices {
		if choice.custom {
			choices = append(choices, agentActionChoice{id: "startAgent", label: "Custom", description: "type command · " + resolved, custom: true})
			continue
		}
		choices = append(choices, agentActionChoice{
			id:          "startAgent",
			label:       choice.label,
			command:     choice.command,
			description: choice.command + " · " + resolved,
		})
	}
	return choices
}

func freshAgentChoices(currentCommand, resolved string) []agentActionChoice {
	choices := []agentActionChoice{{
		id:          "startFreshExisting",
		label:       "Fresh current",
		command:     currentCommand,
		description: currentCommand + " · " + resolved,
	}}
	seen := map[string]bool{strings.TrimSpace(currentCommand): true}
	for _, choice := range agentChoices {
		if choice.custom {
			choices = append(choices, agentActionChoice{id: "startFreshExisting", label: "Fresh custom", description: "type command · " + resolved, custom: true})
			continue
		}
		command := strings.TrimSpace(choice.command)
		if command == "" || seen[command] {
			continue
		}
		seen[command] = true
		choices = append(choices, agentActionChoice{
			id:          "startFreshExisting",
			label:       "Fresh " + choice.label,
			command:     command,
			description: command + " · " + resolved,
		})
	}
	return choices
}

func resolvedAgentMode(r row, selected int) int {
	if selected == agentModeFresh || selected == agentModePlan || selected == agentModeImplementation || selected == agentModeDebug || selected == agentModeReview {
		return selected
	}
	return autoAgentMode(r)
}

func autoAgentMode(r row) int {
	switch types.AgentProfile(r.profile) {
	case types.ProfilePlan:
		return agentModePlan
	case types.ProfileDebug:
		return agentModeDebug
	case types.ProfileReview:
		return agentModeReview
	case types.ProfileImplement:
		return agentModeImplementation
	}
	return agentModeImplementation
}

func agentModeIndex(mode int) int {
	for i, value := range agentModeValues {
		if value == mode {
			return i
		}
	}
	return 0
}

func agentModeLabel(mode int) string {
	switch mode {
	case agentModeFresh:
		return "Fresh"
	case agentModePlan:
		return "Plan"
	case agentModeImplementation:
		return "Implement"
	case agentModeDebug:
		return "Debug"
	case agentModeReview:
		return "Review"
	default:
		return "Auto"
	}
}

func profileForAgentMode(mode int) types.AgentProfile {
	switch mode {
	case agentModePlan:
		return types.ProfilePlan
	case agentModeDebug:
		return types.ProfileDebug
	case agentModeReview:
		return types.ProfileReview
	default:
		return types.ProfileImplement
	}
}

func workspaceChoices(r row, mode int) []workspaceChoice {
	target := defaultSSHTarget()
	switch mode {
	case workspaceModeTerminal:
		choices := []workspaceChoice{}
		if len(r.workspaceShells) > 0 {
			choices = append(choices, workspaceChoice{id: "workspaceCurrent", label: "Current shell", description: r.workspaceShells[0]})
		}
		choices = append(choices,
			workspaceChoice{id: "workspaceNew", label: "New shell", description: "create a tmux shell in this workspace"},
			workspaceChoice{id: "copyCD", label: "CD command", description: "cd " + shellQuote(valueOr(r.workspace, r.repo))},
		)
		return choices
	case workspaceModeEditor:
		choices := []workspaceChoice{}
		for _, choice := range editorChoices() {
			if choice.custom {
				choices = append(choices, workspaceChoice{id: "editorCustom", label: "Custom editor", description: "type command"})
				continue
			}
			choices = append(choices, workspaceChoice{
				id:          "editorCommand:" + choice.command,
				label:       choice.label,
				description: choice.command + " " + shellQuote(valueOr(r.workspace, r.repo)),
			})
		}
		return choices
	case workspaceModeRemote:
		missing := target == ""
		desc := func(id string) string {
			if missing {
				return "needs SSH target"
			}
			return remoteWorkspaceCommand(id, target, valueOr(r.workspace, r.repo))
		}
		setDesc := "set once for remote commands"
		if target != "" {
			setDesc = "currently " + target
		}
		return []workspaceChoice{
			{id: "remoteCursor", label: "Cursor remote", description: desc("remoteCursor"), disabled: missing},
			{id: "remoteCode", label: "VS Code remote", description: desc("remoteCode"), disabled: missing},
			{id: "remoteSSH", label: "SSH cd command", description: desc("remoteSSH"), disabled: missing},
			{id: "setSSH", label: "Set SSH target", description: setDesc},
		}
	default:
		return nil
	}
}

func actionsFor(r row, fullQueue bool) []menuChoice {
	if r.kind == "queue" && !r.structured {
		actions := []menuChoice{
			{id: "agentOpen", label: "Agent...", description: "start from Linear state with optional mode override"},
		}
		if fullQueue {
			actions = append(actions, menuChoice{id: "startSelected", label: "Start selected", description: "batch up to 3 selected issues"})
		}
		return append(actions,
			menuChoice{id: "detail", label: "Details", description: "show issue details"},
			menuChoice{id: "path", label: "Show URL/path", description: "show Linear URL or workspace path"},
		)
	}
	if r.structured {
		actions := []menuChoice{}
		agentDesc := "attach to running agent"
		if !r.active {
			agentDesc = "restart agent in workspace"
		}
		actions = append(actions, menuChoice{id: "agentOpen", label: "Agent...", description: agentDesc})
		if r.workspace != "" || r.repo != "" {
			actions = append(actions, menuChoice{id: "workspaceOpen", label: "Open workspace...", description: "terminal, editor, and remote commands"})
		}
		actions = append(actions,
			menuChoice{id: "briefPublish", label: "Publish brief", description: briefPublishDescription(r)},
			menuChoice{id: "linearMove", label: "Move issue to...", description: "explicit Linear status change"},
			menuChoice{id: "detail", label: "Details", description: "show brief and output"},
			menuChoice{id: "title", label: "Rename session", description: "change display name"},
			menuChoice{id: "close", label: "Close session", description: "safe close without changing Linear"},
			menuChoice{id: "forget", label: "Forget session", description: "forget local state and keep workspace", danger: true},
		)
		if r.workspace != "" && r.repo != "" && r.workspace != r.repo {
			actions = append(actions, menuChoice{id: "deleteWorktree", label: "Delete worktree", description: "remove the cmux worktree only", danger: true})
		}
		return actions
	}
	actions := []menuChoice{}
	if r.active {
		actions = append(actions, menuChoice{id: "agentOpen", label: "Agent...", description: "attach to running session"})
	}
	if r.workspace != "" {
		actions = append(actions, menuChoice{id: "workspaceOpen", label: "Open workspace...", description: "terminal, editor, and remote commands"})
	}
	return append(actions,
		menuChoice{id: "rename", label: "Rename session", description: "change session name"},
		menuChoice{id: "delete", label: "Kill session", description: "close unmanaged session", danger: true},
	)
}

func briefPublishDescription(r row) string {
	if r.briefState == "" {
		return "publish without moving Linear or closing"
	}
	return r.briefState
}

func newChoices() []menuChoice {
	return []menuChoice{
		{id: "scratch", label: "Scratch session", description: "start an agent in a local path"},
		{id: "issueTask", label: "Issue or task", description: "Linear issue key or task title"},
		{id: "workspace", label: "Workspace shell", description: "open shell in selected/current workspace"},
	}
}

func (m *model) toggleQueueSelection() {
	r, ok := m.current()
	if !ok || r.kind != "queue" || r.active || r.structured || r.queueIssue == "" {
		return
	}
	if m.selectedQueue == nil {
		m.selectedQueue = map[string]bool{}
	}
	if m.selectedQueue[r.queueIssue] {
		delete(m.selectedQueue, r.queueIssue)
		m.message = ""
		return
	}
	if len(m.selectedQueue) >= queue.BatchLimit {
		m.message = fmt.Sprintf("Batch start is capped at %d issues", queue.BatchLimit)
		return
	}
	m.selectedQueue[r.queueIssue] = true
	m.message = ""
}

func (m model) selectedQueueIssues() []string {
	var ids []string
	for _, r := range m.rows {
		if r.kind == "queue" && r.queueIssue != "" && m.selectedQueue[r.queueIssue] {
			ids = append(ids, r.queueIssue)
		}
	}
	return ids
}

func (m *model) showWorkspacePath() {
	if r, ok := m.current(); ok && r.kind == "queue" && !r.structured && r.linear != "" {
		m.message = "Linear: " + r.linear
		return
	}
	path, err := m.workspacePath()
	if err != nil {
		m.message = err.Error()
		return
	}
	m.message = "Workspace: " + path
}

func (m *model) workspacePath() (string, error) {
	r, ok := m.current()
	if !ok {
		return "", fmt.Errorf("no session selected")
	}
	path := valueOr(r.workspace, r.repo)
	if path == "" {
		return "", fmt.Errorf("no workspace path recorded")
	}
	return path, nil
}

func (m *model) openWorkspaceSession(createNew bool) (string, error) {
	r, ok := m.current()
	if !ok {
		return "", fmt.Errorf("no session selected")
	}
	if !createNew && len(r.workspaceShells) > 0 {
		return r.workspaceShells[0], nil
	}
	workspace := valueOr(r.workspace, r.repo)
	if workspace == "" {
		return "", fmt.Errorf("no workspace path recorded")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	name, err := tmux.GenerateSessionName(abs, "")
	if err != nil {
		return "", err
	}
	title := "workspace: " + valueOr(r.id, filepath.Base(abs))
	options := tmux.CreateOptions{Name: name, Dir: abs, Title: title}
	if r.structured {
		options.Kind = "workspace"
		options.ParentID = r.id
	}
	if err := tmux.Create(options); err != nil {
		return "", err
	}
	return name, nil
}

func (m *model) openEditor(command string) error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no session selected")
	}
	workspace := valueOr(r.workspace, r.repo)
	if workspace == "" {
		return fmt.Errorf("no workspace path recorded")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace is not a directory: %s", abs)
	}
	if err := startEditorCommand(command, abs); err != nil {
		return err
	}
	if err := config.RememberEditor(command); err != nil {
		return err
	}
	m.mode = "browse"
	m.input = ""
	m.message = "Opened in editor: " + abs
	return nil
}

func startEditorCommand(command, path string) error {
	parts, err := splitEditorCommand(command)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("enter an editor command")
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	cmd := exec.Command(parts[0], args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func splitEditorCommand(command string) ([]string, error) {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
	}
	for _, r := range strings.TrimSpace(command) {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in editor command")
	}
	flush()
	return parts, nil
}

func isSpaceToggleKey(msg tea.KeyMsg) bool {
	key := msg.String()
	if key == " " || key == "shift+space" || key == "shift+ " {
		return true
	}
	if msg.Type == tea.KeySpace {
		return true
	}
	if len(msg.Runes) == 1 && msg.Runes[0] == '\u00a0' {
		return true
	}
	return false
}

func (m *model) restartCurrent() error {
	r, ok := m.current()
	if !ok || !isAgentRow(r) {
		return fmt.Errorf("select a structured agent session")
	}
	s, err := agent.Restart(r.id)
	if err != nil {
		return err
	}
	m.mode = "browse"
	m.message = "Restarted agent"
	rows, err := loadRows(m.fullQueue, false)
	if err == nil {
		m.rows = rows
		m.applyFilter()
		for i, row := range m.filtered {
			if row.id == s.ID {
				m.selected = i
				break
			}
		}
	}
	return nil
}

func (m *model) stopCurrent() error {
	r, ok := m.current()
	if !ok || !isAgentRow(r) {
		return fmt.Errorf("select a structured agent session")
	}
	if err := agent.Kill(r.id); err != nil {
		return err
	}
	m.message = "Stopped agent"
	m.mode = "browse"
	m.reloadRows(r.id)
	return nil
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.rows
	} else {
		needle := strings.ToLower(m.filter)
		var filtered []row
		for _, r := range m.rows {
			hay := strings.ToLower(r.id + " " + r.title + " " + r.status + " " + r.group)
			if strings.Contains(hay, needle) {
				filtered = append(filtered, r)
			}
		}
		m.filtered = filtered
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func displayName(s tmux.Session) string {
	if s.Title != "" {
		return s.Title
	}
	return tmux.Child(s.Name)
}

func helpView() string {
	return strings.Join([]string{
		bold.Render(cyan.Render("CMUX Keys")),
		"",
		cyan.Render("↑↓ j k") + "        navigate",
		cyan.Render("home end") + "      first / last",
		cyan.Render("pgup pgdn") + "     move by 5",
		cyan.Render("enter") + "         open session or start Linear issue",
		cyan.Render(".") + "             actions for selected row",
		cyan.Render("a") + "             new work menu",
		cyan.Render("tab") + "           dashboard / Linear worklist",
		cyan.Render("i") + "             show selected session details",
		cyan.Render("/") + "             filter",
		cyan.Render("space") + "         select Linear issue in full worklist",
		cyan.Render("R") + "             scan sessions and refresh",
		cyan.Render("q esc") + "         quit",
		"",
		dim.Render("press any key to return"),
	}, "\n")
}

func isAgentRow(r row) bool {
	return r.structured
}

func dashboardGroup(status types.AgentStatus) string {
	switch status {
	case types.StatusDone, types.StatusStale:
		return "Done / other"
	default:
		return "Agent sessions"
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nonEmpty(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasSuffix(value, ":") {
			continue
		}
		out = append(out, value)
	}
	return out
}

func detailItems(r row) [][2]string {
	values := [][2]string{
		{"session", r.session},
		{"agent", r.agent},
		{"profile", r.profile},
		{"branch", valueOr(r.branch, "current")},
		{"workspace", r.workspace},
		{"repo", r.repo},
		{"brief", r.brief},
		{"brief state", r.briefState},
		{"linear", r.linear},
		{"summary", r.lastSummary},
	}
	out := [][2]string{}
	for _, item := range values {
		if strings.TrimSpace(item[1]) != "" {
			out = append(out, item)
		}
	}
	return out
}

func writeDetailSection(b *strings.Builder, label, value string) {
	value = cleanDetail(value)
	if value == "" {
		return
	}
	b.WriteString(cyan.Render(label+":") + "\n")
	for _, line := range strings.Split(value, "\n") {
		b.WriteString("  " + line + "\n")
	}
}

func cleanDetail(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "- None.", "- None yet.", "- Not run yet.", "- Not ready for review yet.":
		return ""
	default:
		return value
	}
}

func renderMessage(message string) string {
	if strings.HasPrefix(message, "Workspace:") || strings.HasPrefix(message, "Opened in editor:") || strings.HasPrefix(message, "Command:") || strings.HasPrefix(message, "SSH target:") || strings.HasPrefix(message, "Linear:") || strings.HasPrefix(message, "Published") || strings.HasPrefix(message, "Closed") || strings.HasPrefix(message, "Forgot") || strings.HasPrefix(message, "Stopped") || strings.HasPrefix(message, "Restarted") || strings.HasPrefix(message, "Started") || strings.HasPrefix(message, "Title") {
		return cyan.Render(message)
	}
	return red.Render("Error: " + message)
}

func glyph(status string) string {
	switch status {
	case "running":
		return "●"
	case "waiting_for_input", "blocked", "tests_failed", "crashed":
		return "▲"
	case "ready_for_review":
		return "◆"
	case "done":
		return "✓"
	default:
		return "○"
	}
}

func previewHeight(h int) int {
	switch {
	case h < 20:
		return 4
	case h < 30:
		return 6
	case h < 40:
		return 9
	case h < 55:
		return 12
	default:
		return 16
	}
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func splitTargets(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultSSHTarget() string {
	cfg, err := config.LoadOrDefault()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.DefaultSSHTarget)
}

func remoteWorkspaceCommand(id, target, path string) string {
	target = strings.TrimSpace(target)
	switch id {
	case "remoteCursor":
		return "cursor --remote ssh-remote+" + target + " " + shellQuote(path)
	case "remoteCode":
		return "code --remote ssh-remote+" + target + " " + shellQuote(path)
	case "remoteSSH":
		return "ssh -t " + shellQuote(target) + " " + shellQuote("cd "+shellQuote(path)+" && exec $SHELL -l")
	default:
		return ""
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if isShellSafe(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isShellSafe(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '/', '.', '_', '-', '+', ':', '@', '=':
			continue
		default:
			return false
		}
	}
	return value != ""
}

func defaultRepoPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return wd
	}
	if root, err := gitx.Root(abs); err == nil && root != "" {
		return root
	}
	return abs
}

func normalizeRepoPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("enter a repository path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository path is not a directory: %s", abs)
	}
	root, err := gitx.Root(abs)
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s; choose a repo for Linear work or use Scratch session for non-git folders", abs)
	}
	return root, nil
}
