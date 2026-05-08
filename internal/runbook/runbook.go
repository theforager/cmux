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

type Section struct {
	Heading string
	Body    string
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

func ReadSections(sessionID string) []Section {
	b, err := os.ReadFile(home.RunbookPath(sessionID))
	if err != nil {
		return nil
	}
	return sections(string(b))
}

func Clean(value string) string {
	return clean(value)
}

func CleanBlock(value string) string {
	value = strings.TrimSpace(value)
	if isPlaceholder(value) {
		return ""
	}
	return value
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

func sections(text string) []Section {
	pattern := regexp.MustCompile(`(?m)^## (.+?)\s*$`)
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]Section, 0, len(matches))
	for i, match := range matches {
		heading := strings.TrimSpace(text[match[2]:match[3]])
		start := match[1]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		body := strings.TrimSpace(text[start:end])
		out = append(out, Section{Heading: heading, Body: body})
	}
	return out
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	if isPlaceholder(value) {
		return ""
	}
	return strings.ReplaceAll(value, "\n", " ")
}

func isPlaceholder(value string) bool {
	switch strings.TrimSpace(value) {
	case "- None.", "- None yet.", "- Not started.", "- Not run.", "- Not run yet.", "- Not ready.", "- Not ready for review yet.", "- Start implementation.", "- Pick the first concrete implementation step.":
		return true
	default:
		return false
	}
}
