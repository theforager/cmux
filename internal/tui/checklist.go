package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ChecklistItem struct {
	ID          string
	Label       string
	Description string
}

type ChecklistOptions struct {
	Title    string
	Help     string
	Items    []ChecklistItem
	Selected []string
	Ordered  bool
}

type checklistModel struct {
	title    string
	help     string
	items    []ChecklistItem
	cursor   int
	selected map[string]bool
	order    []string
	ordered  bool
	done     bool
	canceled bool
}

func RunChecklist(o ChecklistOptions) ([]string, error) {
	m := checklistModel{
		title:    o.Title,
		help:     o.Help,
		items:    o.Items,
		selected: map[string]bool{},
		ordered:  o.Ordered,
	}
	for _, id := range o.Selected {
		if id == "" || m.selected[id] {
			continue
		}
		m.selected[id] = true
		m.order = append(m.order, id)
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	next, ok := final.(checklistModel)
	if !ok || next.canceled {
		return nil, nil
	}
	return next.values(), nil
}

func (m checklistModel) Init() tea.Cmd { return nil }

func (m checklistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if isSpaceToggleKey(msg) {
			m.toggle()
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "a":
			m.selectAll()
		case "x":
			m.clear()
		}
	}
	return m, nil
}

func (m checklistModel) View() string {
	var b strings.Builder
	b.WriteString(bold.Render(cyan.Render(m.title)) + "\n")
	if m.help != "" {
		b.WriteString(dim.Render(m.help) + "\n")
	}
	b.WriteString(dim.Render("space toggle  enter accept  a all  x none  esc cancel") + "\n\n")
	for i, item := range m.items {
		box := "[ ]"
		if m.selected[item.ID] {
			box = "[x]"
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "▌ "
		}
		line := fmt.Sprintf("%s %s", box, item.Label)
		if item.Description != "" {
			line += "  " + dim.Render(item.Description)
		}
		if i == m.cursor {
			b.WriteString(green.Render(prefix+line) + "\n")
		} else {
			b.WriteString(prefix + line + "\n")
		}
	}
	if m.ordered && len(m.order) > 0 {
		labels := []string{}
		for _, id := range m.order {
			if m.selected[id] {
				labels = append(labels, m.label(id))
			}
		}
		if len(labels) > 0 {
			b.WriteString("\n" + cyan.Render("order: ") + strings.Join(labels, " -> ") + "\n")
		}
	}
	return b.String()
}

func (m *checklistModel) toggle() {
	if len(m.items) == 0 {
		return
	}
	id := m.items[m.cursor].ID
	if m.selected[id] {
		delete(m.selected, id)
		m.removeFromOrder(id)
		return
	}
	m.selected[id] = true
	m.order = append(m.order, id)
}

func (m *checklistModel) selectAll() {
	for _, item := range m.items {
		if m.selected[item.ID] {
			continue
		}
		m.selected[item.ID] = true
		m.order = append(m.order, item.ID)
	}
}

func (m *checklistModel) clear() {
	m.selected = map[string]bool{}
	m.order = nil
}

func (m *checklistModel) removeFromOrder(id string) {
	next := []string{}
	for _, value := range m.order {
		if value != id {
			next = append(next, value)
		}
	}
	m.order = next
}

func (m checklistModel) values() []string {
	if m.ordered {
		out := []string{}
		for _, id := range m.order {
			if m.selected[id] {
				out = append(out, id)
			}
		}
		return out
	}
	out := []string{}
	for _, item := range m.items {
		if m.selected[item.ID] {
			out = append(out, item.ID)
		}
	}
	return out
}

func (m checklistModel) label(id string) string {
	for _, item := range m.items {
		if item.ID == id {
			return item.Label
		}
	}
	return id
}
