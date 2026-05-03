package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type row struct {
	id      string
	title   string
	status  string
	group   string
	updated string
	session string
	preview string
	active  bool
}

type model struct {
	rows     []row
	filtered []row
	selected int
	filter   string
	input    string
	pending  types.AgentStatus
	mode     string
	digits   string
	message  string
	chosen   string
	width    int
	height   int
}

var issuePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)

var (
	cyan  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim   = lipgloss.NewStyle().Faint(true)
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	bold  = lipgloss.NewStyle().Bold(true)
)

func Run(popup bool) (string, error) {
	rows, err := loadRows()
	if err != nil {
		return "", err
	}
	m := model{rows: rows, filtered: rows, mode: "browse", width: 80, height: 24}
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

func loadRows() ([]row, error) {
	tmuxSessions, err := tmux.List()
	if err != nil {
		return nil, err
	}
	agentSessions, _ := state.List()
	byTmux := map[string]row{}
	for _, s := range agentSessions {
		byTmux[s.TmuxSession] = row{id: s.ID, title: s.Title, status: string(s.Status), group: string(s.Type), updated: format.Age(s.LastUpdatedAt), session: s.TmuxSession, preview: s.LastSummary, active: false}
	}
	var rows []row
	seen := map[string]bool{}
	for _, s := range tmuxSessions {
		if r, ok := byTmux[s.Name]; ok {
			r.active = true
			r.preview = tmux.Capture(s.Name, 10)
			rows = append(rows, r)
			seen[s.Name] = true
			continue
		}
		rows = append(rows, row{id: tmux.Child(s.Name), title: displayName(s), status: tmux.InferStatus(s.Name), group: tmux.Parent(s.Name), updated: format.AgeUnix(s.Created), session: s.Name, preview: tmux.Capture(s.Name, 10), active: true})
		seen[s.Name] = true
	}
	for _, s := range agentSessions {
		if seen[s.TmuxSession] {
			continue
		}
		rows = append(rows, row{id: s.ID, title: s.Title + " (not running)", status: string(s.Status), group: string(s.Type), updated: format.Age(s.LastUpdatedAt), session: s.TmuxSession, preview: s.LastSummary, active: false})
	}
	return rows, nil
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
			switch key {
			case "esc":
				m.mode = "browse"
				m.input = ""
			case "enter":
				if m.mode == "filter" {
					m.mode = "browse"
					return m, nil
				}
				if err := m.commitAction(); err != nil {
					m.message = err.Error()
				} else {
					m.message = ""
					m.mode = "browse"
					m.input = ""
					rows, err := loadRows()
					if err == nil {
						m.rows = rows
						m.applyFilter()
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
		case "n":
			m.mode = "scratch"
			wd, _ := os.Getwd()
			m.input = wd
		case "a":
			m.mode = "start"
			m.input = ""
		case "r":
			if r, ok := m.current(); ok && r.active {
				m.mode = "rename"
				m.input = tmux.Child(r.session)
			}
		case "t":
			if r, ok := m.current(); ok && r.active {
				m.mode = "title"
				m.input = r.title
			}
		case "d":
			if _, ok := m.current(); ok {
				m.mode = "delete"
				m.input = ""
			}
		case "s":
			if r, ok := m.current(); ok && isAgentRow(r) {
				m.mode = "statusPick"
				m.input = ""
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
			rows, err := loadRows()
			if err == nil {
				m.rows = rows
				m.applyFilter()
			} else {
				m.message = err.Error()
			}
		case "enter":
			if len(m.filtered) == 0 {
				return m, nil
			}
			r := m.filtered[m.selected]
			if !r.active {
				m.message = "Session is not running: " + r.id
				return m, nil
			}
			m.chosen = r.session
			return m, tea.Quit
		default:
			if len(key) == 1 && key >= "1" && key <= "9" {
				m.digits += key
				target := atoi(m.digits)
				if target >= 1 && target <= len(m.filtered) {
					m.selected = target - 1
				} else {
					single := atoi(key)
					m.digits = ""
					if single >= 1 && single <= len(m.filtered) {
						m.selected = single - 1
						m.digits = key
					}
				}
			} else {
				m.digits = ""
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.mode == "help" {
		return helpView()
	}
	var b strings.Builder
	count := fmt.Sprintf("%d sessions", len(m.filtered))
	b.WriteString(bold.Render(cyan.Render("cmux")) + dim.Render("  "+count))
	if m.filter != "" || m.mode == "filter" {
		b.WriteString("  " + cyan.Render("/"+m.filter))
		if m.mode == "filter" {
			b.WriteString(cyan.Render("_"))
		}
	}
	b.WriteString("\n\n")
	if len(m.filtered) == 0 {
		b.WriteString(dim.Render("  no sessions") + "\n")
	} else {
		lastGroup := ""
		for i, r := range m.filtered {
			if r.group != lastGroup {
				lastGroup = r.group
				b.WriteString(dim.Render("  "+r.group) + "\n")
			}
			line := fmt.Sprintf("%s %-3d %-16s %-17s %s  %s", glyph(r.status), i+1, format.Trunc(r.id, 16), format.Trunc(r.status, 17), r.title, dim.Render(r.updated))
			if i == m.selected {
				b.WriteString(green.Render("▌ "+line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
	}
	b.WriteString("\n" + dim.Render("── preview ─────────────────────────────────────────") + "\n")
	if len(m.filtered) > 0 {
		preview := m.filtered[m.selected].preview
		if preview == "" {
			preview = "(no recent activity)"
		}
		for _, line := range tail(strings.Split(preview, "\n"), previewHeight(m.height)) {
			b.WriteString("▎ " + line + "\n")
		}
	}
	b.WriteString("\n")
	if prompt := m.prompt(); prompt != "" {
		b.WriteString(prompt)
	} else {
		b.WriteString(dim.Render("↑↓/jk nav  1-9 jump  home/end  pgup/pgdn  enter open  / filter  n scratch  a issue/task  r rename  t title  d delete  s status  R refresh  ? help  q quit"))
	}
	if m.digits != "" {
		b.WriteString("  " + cyan.Render("→ "+m.digits))
	}
	if m.message != "" {
		b.WriteString("  " + red.Render(m.message))
	}
	return b.String()
}

func (m model) current() (row, bool) {
	if len(m.filtered) == 0 || m.selected < 0 || m.selected >= len(m.filtered) {
		return row{}, false
	}
	return m.filtered[m.selected], true
}

func (m *model) commitAction() error {
	r, hasRow := m.current()
	value := strings.TrimSpace(m.input)
	switch m.mode {
	case "scratch":
		if value == "" {
			value = "."
		}
		abs, _ := filepath.Abs(value)
		title := filepath.Base(abs)
		_, err := agent.Start(agent.StartOptions{Cwd: abs, Scratch: true, Title: title})
		return err
	case "start":
		if value == "" {
			return fmt.Errorf("enter a Linear issue key or task title")
		}
		if issuePattern.MatchString(value) {
			_, err := agent.Start(agent.StartOptions{Cwd: ".", IssueKey: value})
			return err
		}
		_, err := agent.Start(agent.StartOptions{Cwd: ".", Title: value, Worktree: true})
		return err
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
		if !hasRow || !r.active {
			return fmt.Errorf("no active session selected")
		}
		if err := tmux.SetTitle(r.session, value); err != nil {
			return err
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
			return agent.Kill(r.id)
		}
		return tmux.Kill(r.session)
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

func (m model) prompt() string {
	switch m.mode {
	case "filter":
		return cyan.Render("filter  "+m.filter+"_  ") + dim.Render("enter apply · esc cancel")
	case "scratch":
		return cyan.Render("scratch path  "+m.input+"_  ") + dim.Render("enter create · esc cancel")
	case "start":
		return cyan.Render("issue/task  "+m.input+"_  ") + dim.Render("REB-123 starts Linear work · other text creates task worktree")
	case "rename":
		return cyan.Render("rename  "+m.input+"_  ") + dim.Render("enter save · esc cancel")
	case "title":
		return cyan.Render("title  "+m.input+"_  ") + dim.Render("enter save · esc cancel")
	case "delete":
		return red.Render("delete? type y then enter  ") + dim.Render("esc cancel")
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
		return cyan.Render("summary for "+string(m.pending)+"  "+m.input+"_  ") + dim.Render("enter save · esc cancel")
	default:
		return ""
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
		cyan.Render("1-9 digits") + "    jump to row number",
		cyan.Render("enter") + "         open selected session",
		cyan.Render("/") + "             filter",
		cyan.Render("n") + "             create scratch session",
		cyan.Render("a") + "             start Linear issue or task-backed agent",
		cyan.Render("r") + "             rename selected tmux session",
		cyan.Render("t") + "             set selected title",
		cyan.Render("d") + "             delete selected session",
		cyan.Render("s") + "             set structured agent status",
		cyan.Render("R") + "             refresh",
		cyan.Render("q esc") + "         quit",
		"",
		dim.Render("press any key to return"),
	}, "\n")
}

func isAgentRow(r row) bool {
	return r.group == string(types.TypeIssueBacked) || r.group == string(types.TypeTaskBacked) || r.group == string(types.TypeScratch)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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
	if h < 20 {
		return 4
	}
	if h > 45 {
		return 16
	}
	return 8
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
