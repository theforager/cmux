package runbook

import (
	"os"
	"regexp"
	"strings"

	"github.com/theforager/cmux/internal/home"
)

type Summary struct {
	Goal          string
	CurrentState  string
	DecisionsMade string
	Blockers      string
	TestsRun      string
	NextAction    string
	ReviewSummary string
}

func Read(sessionID string) Summary {
	b, err := os.ReadFile(home.RunbookPath(sessionID))
	if err != nil {
		return Summary{}
	}
	text := string(b)
	return Summary{
		Goal:          section(text, "Goal"),
		CurrentState:  section(text, "Current state"),
		DecisionsMade: section(text, "Decisions made"),
		Blockers:      section(text, "Blockers"),
		TestsRun:      section(text, "Tests run"),
		NextAction:    section(text, "Next action"),
		ReviewSummary: section(text, "Review summary"),
	}
}

func (s Summary) Preview() string {
	lines := []string{}
	add := func(label, value string) {
		value = clean(value)
		if value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("Current", s.CurrentState)
	add("Next", s.NextAction)
	add("Blockers", s.Blockers)
	add("Tests", s.TestsRun)
	add("Review", s.ReviewSummary)
	return strings.Join(lines, "\n")
}

func section(text, heading string) string {
	pattern := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(heading) + `\s*$`)
	loc := pattern.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	rest := text[loc[1]:]
	next := regexp.MustCompile(`(?m)^## `).FindStringIndex(rest)
	if next != nil {
		rest = rest[:next[0]]
	}
	return strings.TrimSpace(rest)
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	if value == "- None." || value == "- None yet." || value == "- Not run yet." || value == "- Not ready for review yet." {
		return ""
	}
	return strings.ReplaceAll(value, "\n", " ")
}
