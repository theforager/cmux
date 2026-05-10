package runbook

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/theforager/cmux/internal/home"
)

type Summary struct {
	Goal             string
	CurrentState     string
	DecisionsMade    string
	Blockers         string
	TestsRun         string
	NextAction       string
	UserConfirmation string
	ReviewSummary    string
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
		Goal:             firstSection(text, "Goal"),
		CurrentState:     firstSection(text, "Current state", "Current understanding"),
		DecisionsMade:    firstSection(text, "Decisions made", "Key decisions"),
		Blockers:         firstSection(text, "Blockers", "Open questions / risks"),
		TestsRun:         firstSection(text, "Tests run", "Research / tests"),
		NextAction:       firstSection(text, "Next action", "Next coding steps"),
		UserConfirmation: section(text, "User confirmation"),
		ReviewSummary:    firstSection(text, "Review summary", "Review notes"),
	}
}

func DefaultContent(goal, phase string) string {
	if phase == "scoping" {
		return "# Agent Runbook\n\n## Goal\n" + goal + "\n\n## Current understanding\n- Not started.\n\n## Key decisions\n- None.\n\n## Proposed plan\n- Not started.\n\n## Acceptance criteria\n- Not started.\n\n## Open questions / risks\n- None.\n\n## User confirmation\n- Not confirmed.\n\n## Next coding steps\n- Pick the first concrete implementation step.\n"
	}
	return "# Agent Runbook\n\n## Goal\n" + goal + "\n\n## Current state\n- Not started.\n\n## Decisions made\n- None.\n\n## Tests run\n- Not run.\n\n## Next action\n- Pick the first concrete implementation step.\n\n## Review notes\n- Not ready.\n"
}

func ValidateScopedHandoff(sessionID string) error {
	notes := Read(sessionID)
	if CleanBlock(notes.UserConfirmation) == "" {
		return fmt.Errorf("scoping handoff needs user confirmation before publishing to Linear; walk through key decisions, open questions, and the proposed plan with the user, then record approval in RUNBOOK.md under ## User confirmation")
	}
	return nil
}

const ScopedStartMarker = "<!-- cmux:scoped:start -->"
const ScopedEndMarker = "<!-- cmux:scoped:end -->"

func ScopedHandoff(sessionID, summary string) string {
	return scopedHandoffBlock(summary, ReadSections(sessionID))
}

func ReplaceScopedHandoff(description, block string) string {
	wrapped := ScopedStartMarker + "\n" + block + "\n" + ScopedEndMarker
	description = strings.TrimSpace(description)
	start := strings.Index(description, ScopedStartMarker)
	end := strings.Index(description, ScopedEndMarker)
	if start >= 0 && end >= start {
		end += len(ScopedEndMarker)
		return strings.TrimSpace(description[:start] + wrapped + description[end:])
	}
	if description == "" {
		return wrapped
	}
	return description + "\n\n" + wrapped
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

func firstSection(text string, headings ...string) string {
	for _, heading := range headings {
		if value := section(text, heading); value != "" {
			return value
		}
	}
	return ""
}

func scopedHandoffBlock(summary string, sections []Section) string {
	lines := []string{"## cmux scoped handoff"}
	add := func(label, value string) {
		value = CleanBlock(value)
		if value != "" {
			lines = append(lines, "", "### "+label, value)
		}
	}
	add("Summary", summary)
	for _, section := range sections {
		heading := strings.TrimSpace(section.Heading)
		if skipScopedHandoffSection(heading) {
			continue
		}
		add(heading, section.Body)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func skipScopedHandoffSection(heading string) bool {
	switch strings.ToLower(strings.TrimSpace(heading)) {
	case "goal", "review summary", "review notes", "tests run":
		return true
	default:
		return false
	}
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
	case "- None.", "- None yet.", "- Not started.", "- Not run.", "- Not run yet.", "- Not ready.", "- Not ready for review yet.", "- Not confirmed.", "- Start implementation.", "- Pick the first concrete implementation step.":
		return true
	default:
		return false
	}
}
