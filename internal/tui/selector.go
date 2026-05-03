package tui

import (
	"fmt"
	"strings"

	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"

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
	mode     string
	digits   string
	message  string
	chosen   string
	width    int
	height   int
}

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
		if m.mode == "filter" {
			switch key {
			case "esc", "enter":
				m.mode = "browse"
			case "backspace", "ctrl+h":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
				m.applyFilter()
			default:
				if len(key) == 1 {
					m.filter += key
					m.applyFilter()
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
	if m.mode == "filter" {
		b.WriteString(cyan.Render("filter  "+m.filter+"_  ") + dim.Render("enter apply · esc cancel"))
	} else {
		b.WriteString(dim.Render("↑↓/jk nav  1-9 jump  home/end  pgup/pgdn  enter open  / filter  R refresh  ? help  q quit"))
	}
	if m.digits != "" {
		b.WriteString("  " + cyan.Render("→ "+m.digits))
	}
	if m.message != "" {
		b.WriteString("  " + red.Render(m.message))
	}
	return b.String()
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
		cyan.Render("R") + "             refresh",
		cyan.Render("q esc") + "         quit",
		"",
		dim.Render("press any key to return"),
	}, "\n")
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
