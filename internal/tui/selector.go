package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/config"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/queue"
	"github.com/theforager/cmux/internal/runbook"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type row struct {
	id              string
	title           string
	status          string
	group           string
	updated         string
	queueRank       int
	session         string
	repo            string
	workspace       string
	branch          string
	agent           string
	runbook         string
	linear          string
	phase           string
	lastSummary     string
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
	agentSelected  int
	repoSelected   int
	actionSelected int
	newSelected    int
	filter         string
	input          string
	create         string
	createRepo     string
	target         string
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

var (
	cyan  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim   = lipgloss.NewStyle().Faint(true)
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	bold  = lipgloss.NewStyle().Bold(true)
)

func Run(popup bool) (string, error) {
	rows, err := loadRows(false)
	if err != nil {
		return "", err
	}
	m := model{rows: rows, filtered: rows, mode: "browse", width: 80, height: 24, selectedQueue: map[string]bool{}}
	opts := []tea.ProgramOption{}
	if !popup {
		opts = []tea.ProgramOption{tea.WithAltScreen()}
	}
	p := tea.NewProgram(m, opts...)
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
	rows, err := loadRows(true)
	if err != nil {
		return "", err
	}
	m := model{rows: rows, filtered: rows, mode: "browse", width: 80, height: 24, fullQueue: true, selectedQueue: map[string]bool{}}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if m, ok := final.(model); ok {
		return m.chosen, nil
	}
	return "", nil
}

func loadRows(fullQueue bool) ([]row, error) {
	_, _ = agent.Scan()
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
			group:   "Linear",
			updated: "",
			kind:    "notice",
			detail:  "Linear worklist needs LINEAR_API_KEY before cmux can fetch issues.",
		}}
	}
	limit := 8
	if fullQueue {
		limit = 50
	}
	rows, preset, err := queue.Rows("", limit)
	if err != nil {
		return []row{{
			id:     "linear-error",
			title:  err.Error(),
			status: "error",
			group:  "Linear",
			kind:   "notice",
			detail: "Queue preset could not be loaded or Linear returned an error.",
		}}
	}
	if len(rows) == 0 {
		return []row{{id: "linear-empty", title: "No matching Linear issues in " + preset.Name, status: "empty", group: "Linear", kind: "notice"}}
	}
	displayLimit := limit
	out := make([]row, 0, len(rows))
	for rank, qr := range rows {
		if !fullQueue && qr.Started {
			continue
		}
		if !fullQueue && displayLimit > 0 && len(out) >= displayLimit {
			break
		}
		r := row{
			id:         qr.Issue.Identifier,
			title:      qr.Issue.Title,
			status:     string(qr.Status),
			group:      "Linear",
			queueRank:  rank,
			repo:       preset.RepoPath,
			workspace:  preset.RepoPath,
			linear:     qr.Issue.URL,
			branch:     qr.Issue.BranchName,
			kind:       "queue",
			queueIssue: qr.Issue.Identifier,
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
			sr.group = "Linear"
			sr.kind = "queue"
			sr.queueIssue = qr.Issue.Identifier
			sr.queueRank = rank
			sr.workspaceShells = r.workspaceShells
			sr.updated = format.Age(qr.Session.LastUpdatedAt)
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
		if rows[i].group == "Linear" && rows[i].queueRank != rows[j].queueRank {
			return rows[i].queueRank < rows[j].queueRank
		}
		return rows[i].updated > rows[j].updated
	})
}

func groupRank(group string) int {
	switch group {
	case "Needs attention":
		return 0
	case "Active":
		return 1
	case "Ready for review":
		return 2
	case "Linear":
		return 3
	case "Stale":
		return 4
	case "Done/Other":
		return 5
	default:
		return 6
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
	notes := runbook.Read(s.ID)
	preview := notes.Preview()
	if preview == "" {
		preview = s.LastSummary
	}
	detail := strings.Join(nonEmpty([]string{
		"repo: " + s.RepoPath,
		"workspace: " + valueOr(s.WorktreePath, s.RepoPath),
		"runbook: " + home.RunbookPath(s.ID),
		"linear: " + s.Linear.URL,
	}), "\n")
	return row{
		id:              s.ID,
		title:           s.Title,
		status:          string(s.Status),
		group:           dashboardGroup(s.Status),
		updated:         format.Age(s.LastUpdatedAt),
		session:         s.TmuxSession,
		repo:            s.RepoPath,
		workspace:       valueOr(s.WorktreePath, s.RepoPath),
		branch:          s.Branch,
		agent:           valueOr(s.AgentCommand, s.Provider),
		runbook:         home.RunbookPath(s.ID),
		linear:          s.Linear.URL,
		phase:           s.Phase,
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
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
				case "esc", "backspace", "ctrl+h", ".":
					m.mode = "browse"
				case "up", "k":
					m.moveAction(-1)
				case "down", "j", "tab":
					m.moveAction(1)
				case "enter":
					if err := m.chooseAction(); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.mode == "newMenu" {
				switch key {
				case "esc", "backspace", "ctrl+h", "a":
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
				case "esc":
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
					m.pending = types.StatusStale
					m.mode = "statusSummary"
					m.input = ""
				case "9":
					m.pending = types.StatusCrashed
					m.mode = "statusSummary"
					m.input = ""
				}
				return m, nil
			}
			if m.mode == "agentPick" {
				switch key {
				case "esc":
					m.mode = "browse"
					m.input = ""
					m.create = ""
					m.createRepo = ""
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
				case "1":
					if err := m.createWithAgent("claude"); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					} else {
						m.afterCreate()
					}
				case "2":
					if err := m.createWithAgent("codex"); err != nil {
						m.message = err.Error()
					} else if m.chosen != "" {
						return m, tea.Quit
					} else {
						m.afterCreate()
					}
				case "3":
					m.mode = "agentCustom"
					m.input = ""
					m.message = ""
				}
				return m, nil
			}
			if m.mode == "repoPick" {
				switch key {
				case "esc":
					m.mode = "browse"
					m.input = ""
					m.create = ""
					m.createRepo = ""
					m.target = ""
					m.message = ""
				case "up", "k":
					m.moveRepo(-1)
				case "down", "j", "tab":
					m.moveRepo(1)
				case "enter":
					if err := m.chooseRepo(); err != nil {
						m.message = err.Error()
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
					m.message = ""
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
			rows, err := loadRows(m.fullQueue)
			if err == nil {
				m.rows = rows
				m.applyFilter()
			} else {
				m.message = err.Error()
			}
		case "tab":
			m.fullQueue = !m.fullQueue
			m.filter = ""
			rows, err := loadRows(m.fullQueue)
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
			status := r.status
			if len(r.workspaceShells) > 0 {
				status += " ⧉"
			}
			line := fmt.Sprintf("%s%s %-16s %-20s %s  %s", glyph(r.status), mark, format.Trunc(r.id, 16), format.Trunc(status, 20), r.title, dim.Render(r.updated))
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
		if r.phase != "" && r.phase != types.PhaseWork {
			parts = append(parts, r.phase)
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
		runbookBudget := max - termBudget
		runbookLines := previewRunbookLines(r, runbookBudget)
		remaining := max - len(runbookLines)
		if len(runbookLines) > 0 && remaining > termBudget {
			remaining = termBudget
		}
		termLines := previewTextLines("term", r.terminalPreview, remaining, true, "dim")
		if len(runbookLines) == 0 && len(termLines) == 0 {
			return []previewLine{{label: "summary", value: "No runbook or terminal preview yet.", style: "dim"}}
		}
		return append(runbookLines, termLines...)
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

func previewRunbookLines(r row, max int) []previewLine {
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
		m.mode = "agentPick"
		m.agentSelected = 0
		m.input = ""
		return nil
	case "repoPath":
		if value == "" {
			return fmt.Errorf("enter a repository path")
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			return err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("repository path is not a directory: %s", abs)
		}
		if _, err := gitx.Root(abs); err != nil {
			return fmt.Errorf("not a git repository: %s; choose a repo for Linear work or use Scratch session for non-git folders", abs)
		}
		if err := config.RememberRepo(abs); err != nil {
			return err
		}
		m.createRepo = abs
		m.mode = "agentPick"
		m.agentSelected = 0
		m.input = ""
		return nil
	case "agentCustom":
		if value == "" {
			return fmt.Errorf("enter an agent command")
		}
		return m.createWithAgent(value)
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
	case "delete":
		if !hasRow {
			return fmt.Errorf("no session selected")
		}
		if strings.ToLower(value) != "y" {
			return nil
		}
		if isAgentRow(r) {
			return agent.Delete(r.id)
		}
		return tmux.Kill(r.session)
	case "close":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		if strings.ToLower(value) != "y" {
			return nil
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
	case "scopedSummary":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		_, err := agent.MarkScoped(r.id, value)
		return err
	case "needsReviewSummary":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		_, err := agent.MarkNeedsReview(r.id, value)
		return err
	case "doneSummary":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		_, err := agent.Complete(r.id, value)
		return err
	case "abandonSummary":
		if !hasRow || !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		_, err := agent.Abandon(r.id, value)
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
		_, err := agent.Start(agent.StartOptions{Cwd: abs, Scratch: true, Title: title, Agent: agentCommand})
		return err
	case "start":
		target := strings.TrimSpace(m.target)
		if target == "" {
			return fmt.Errorf("enter a Linear issue key or task title")
		}
		if issuePattern.MatchString(target) {
			_, err := agent.Start(agent.StartOptions{Cwd: ".", IssueKey: target, Agent: agentCommand})
			return err
		}
		_, err := agent.Start(agent.StartOptions{Cwd: ".", Title: target, Worktree: true, Agent: agentCommand})
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
			s, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand})
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
	case "queueScope":
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
			s, err := agent.Start(agent.StartOptions{Cwd: cwd, IssueKey: target, Agent: agentCommand, Scoping: true})
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
	m.target = ""
	m.agentSelected = 0
	m.message = ""
	m.selectedQueue = map[string]bool{}
	rows, err := loadRows(m.fullQueue)
	if err == nil {
		m.rows = rows
		m.applyFilter()
	}
}

func (m *model) reloadRows(selectID string) {
	rows, err := loadRows(m.fullQueue)
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
	b.WriteString(cyan.Render("agent") + dim.Render("  ↑↓/jk select · enter create · 1-3 shortcut · esc cancel") + "\n")
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
	case "open", "restart", "workspace", "workspaceNew":
		return "Open"
	case "startQueue", "startScope", "startSelected":
		return "Start"
	case "markScoped", "needsReview", "done", "abandon":
		return "Workflow"
	case "detail", "path", "status":
		return "Inspect"
	case "title", "rename":
		return "Name"
	case "close":
		return "Cleanup"
	case "delete", "cleanup", "reset":
		return "Advanced"
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
	b.WriteString("\n" + dim.Render("── runbook ─────────────────────────────────────────") + "\n")
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
	case "agentCustom":
		return renderPromptInput("agent command", m.input) + dim.Render("enter create · esc cancel")
	case "rename":
		return renderPromptInput("session name", m.input) + dim.Render("enter save · esc cancel")
	case "title":
		return renderPromptInput("display name", m.input) + dim.Render("enter save · esc cancel")
	case "delete":
		return red.Render("forget session? type y then enter  ") + dim.Render("agent stops; worktree and branch are kept · esc cancel")
	case "close":
		return red.Render("close session? type y then enter  ") + dim.Render("clean cmux worktree is removed; dirty workspaces are refused · esc cancel")
	case "cleanup":
		return red.Render("cleanup worktree? type y if clean, or type session id to force  ") + dim.Render("dirty worktrees are refused unless forced · esc cancel")
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
			"  8 stale",
			"  9 crashed",
			dim.Render("press 1-9 · esc cancel"),
		}, "\n")
	case "statusSummary":
		return renderPromptInput("summary for "+string(m.pending), m.input) + dim.Render("enter save · esc cancel")
	case "scopedSummary":
		return renderPromptInput("scope summary", m.input) + dim.Render("moves Linear to ready state · esc cancel")
	case "needsReviewSummary":
		return renderPromptInput("review summary", m.input) + dim.Render("adds needs-review in Linear · esc cancel")
	case "doneSummary":
		return renderPromptInput("completion summary", m.input) + dim.Render("moves Linear issue to Done · esc cancel")
	case "abandonSummary":
		return renderPromptInput("abandon summary", m.input) + dim.Render("moves Linear back to original queue state · esc cancel")
	default:
		return ""
	}
}

func renderPromptInput(label, value string) string {
	return cyan.Render(label+"  "+value) + cyan.Render("▌  ")
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

func (m *model) chooseAction() error {
	r, ok := m.current()
	if !ok {
		return fmt.Errorf("no row selected")
	}
	choices := actionsFor(r, m.fullQueue)
	if len(choices) == 0 {
		return fmt.Errorf("no actions available")
	}
	if m.actionSelected < 0 || m.actionSelected >= len(choices) {
		m.actionSelected = 0
	}
	switch choices[m.actionSelected].id {
	case "open":
		if !r.active || !tmux.Has(r.session) {
			return fmt.Errorf("session is not running")
		}
		m.chosen = r.session
	case "startQueue":
		return m.startQueueFlow(false, false)
	case "startScope":
		return m.startQueueFlow(false, true)
	case "startSelected":
		return m.startQueueFlow(true, false)
	case "detail":
		m.mode = "detail"
	case "path":
		m.showWorkspacePath()
		m.mode = "browse"
	case "workspace", "workspaceNew":
		name, err := m.openWorkspaceSession(choices[m.actionSelected].id == "workspaceNew")
		if err != nil {
			return err
		}
		m.chosen = name
	case "restart":
		s, err := m.restartCurrent()
		if err != nil {
			return err
		}
		m.chosen = s.TmuxSession
	case "status":
		if !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		m.mode = "statusPick"
		m.input = ""
	case "markScoped":
		if !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		m.mode = "scopedSummary"
		m.input = ""
	case "needsReview":
		if !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		m.mode = "needsReviewSummary"
		m.input = r.lastSummary
	case "done":
		if !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		m.mode = "doneSummary"
		m.input = r.lastSummary
	case "abandon":
		if !isAgentRow(r) {
			return fmt.Errorf("select a structured agent session")
		}
		m.mode = "abandonSummary"
		m.input = ""
	case "rename":
		if !r.active {
			return fmt.Errorf("session is not running")
		}
		m.mode = "rename"
		m.input = tmux.Child(r.session)
	case "title":
		if !r.active && !isAgentRow(r) {
			return fmt.Errorf("session is not running")
		}
		m.mode = "title"
		m.input = r.title
	case "delete":
		m.mode = "delete"
		m.input = ""
	case "close":
		m.mode = "close"
		m.input = ""
	case "cleanup":
		m.mode = "cleanup"
		m.input = ""
	case "reset":
		m.mode = "reset"
		m.input = ""
	}
	return nil
}

func (m *model) primaryAction() error {
	r, ok := m.current()
	if !ok {
		return nil
	}
	if r.kind == "queue" && !r.active && !r.structured {
		return m.startQueueFlow(false, false)
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
		m.create = "queueScope"
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
	m.createRepo = choice.path
	m.mode = "agentPick"
	m.agentSelected = 0
	m.input = ""
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

func actionsFor(r row, fullQueue bool) []menuChoice {
	if r.kind == "queue" && !r.structured {
		actions := []menuChoice{
			{id: "startQueue", label: "Start work", description: "move to active work and launch agent"},
			{id: "startScope", label: "Start scoping", description: "move to scoping and launch planning agent"},
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
		if r.active {
			actions = append(actions, menuChoice{id: "open", label: "Open agent", description: "open session"})
		} else {
			actions = append(actions, menuChoice{id: "restart", label: "Restart agent", description: "start agent again in workspace"})
		}
		actions = append(actions, workspaceAction(r))
		if len(r.workspaceShells) > 0 {
			actions = append(actions, menuChoice{id: "workspaceNew", label: "New workspace terminal", description: "create another attached terminal"})
		}
		if r.phase == types.PhaseScoping {
			actions = append(actions, menuChoice{id: "markScoped", label: "Mark scoped", description: "move Linear issue to ready queue"})
		}
		if r.linear != "" {
			actions = append(actions, menuChoice{id: "needsReview", label: "Mark needs review", description: "add Linear needs-review label"})
			actions = append(actions, menuChoice{id: "done", label: "Mark done", description: "move Linear issue to Done"})
			actions = append(actions, menuChoice{id: "abandon", label: "Abandon work", description: "move Linear issue back to original queue state", danger: true})
		}
		return append(actions,
			menuChoice{id: "detail", label: "Details", description: "show runbook and output"},
			menuChoice{id: "status", label: "Set status", description: "advanced status override"},
			menuChoice{id: "title", label: "Rename session", description: "change display name"},
			menuChoice{id: "close", label: "Close session", description: closeDescription(r), danger: true},
			menuChoice{id: "delete", label: "Forget session", description: "stop agent and keep workspace", danger: true},
			menuChoice{id: "reset", label: "Reset workspace", description: "git reset --hard and clean", danger: true},
		)
	}
	actions := []menuChoice{}
	if r.active {
		actions = append(actions, menuChoice{id: "open", label: "Open session", description: "open unmanaged session"})
	}
	if r.workspace != "" {
		actions = append(actions, menuChoice{id: "workspace", label: "Workspace terminal", description: "open shell in path"})
	}
	return append(actions,
		menuChoice{id: "rename", label: "Rename session", description: "change session name"},
		menuChoice{id: "delete", label: "Kill session", description: "close unmanaged session", danger: true},
	)
}

func workspaceAction(r row) menuChoice {
	if len(r.workspaceShells) > 0 {
		return menuChoice{id: "workspace", label: "Open workspace terminal", description: "open attached terminal"}
	}
	return menuChoice{id: "workspace", label: "New workspace terminal", description: "create attached terminal"}
}

func closeDescription(r row) string {
	if r.workspace != "" && r.repo != "" && r.workspace != r.repo {
		return "remove clean worktree and hide session"
	}
	return "stop agent and hide session"
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
	r, ok := m.current()
	if !ok {
		m.message = "No session selected"
		return
	}
	path := valueOr(r.workspace, r.repo)
	if path == "" {
		m.message = "No workspace path recorded"
		return
	}
	m.message = "Workspace: " + path
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

func (m *model) restartCurrent() (types.AgentSession, error) {
	r, ok := m.current()
	if !ok || !isAgentRow(r) {
		return types.AgentSession{}, fmt.Errorf("select a structured agent session")
	}
	s, err := agent.Restart(r.id)
	if err != nil {
		return s, err
	}
	rows, err := loadRows(m.fullQueue)
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
	return s, nil
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
	case types.StatusBlocked, types.StatusTestsFailed, types.StatusWaiting, types.StatusCrashed:
		return "Needs attention"
	case types.StatusRunning, types.StatusIdle:
		return "Active"
	case types.StatusReadyForReview, types.StatusPROpened:
		return "Ready for review"
	case types.StatusStale:
		return "Stale"
	default:
		return "Done/Other"
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
		{"phase", r.phase},
		{"branch", valueOr(r.branch, "current")},
		{"workspace", r.workspace},
		{"repo", r.repo},
		{"runbook", r.runbook},
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
	if strings.HasPrefix(message, "Workspace:") {
		return cyan.Render(message)
	}
	return red.Render("Error: " + message)
}

func glyph(status string) string {
	switch status {
	case "running":
		return "●"
	case "waiting_for_input":
		return "◐"
	case "blocked":
		return "!"
	case "tests_failed", "crashed":
		return "✕"
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

func defaultRepoPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return wd
	}
	return abs
}
