package brief

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/types"
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
	ChangesMade      string
	RisksFollowUp    string
	BranchPR         string
}

type Section struct {
	Heading string
	Body    string
}

func Read(sessionID string) Summary {
	b, err := os.ReadFile(home.BriefPath(sessionID))
	if err != nil {
		return Summary{}
	}
	text := string(b)
	return Summary{
		Goal:             firstSection(text, "Goal"),
		CurrentState:     firstSection(text, "Current state", "Current understanding", "Findings", "Symptom", "Summary"),
		DecisionsMade:    firstSection(text, "Decisions", "Decisions made", "Key decisions"),
		Blockers:         firstSection(text, "Open questions", "Open questions / risks", "Risks / follow-up", "Risks"),
		TestsRun:         firstSection(text, "Tests run", "Verification", "Research / tests"),
		NextAction:       firstSection(text, "Next steps", "Next action", "Next coding steps"),
		UserConfirmation: section(text, "User confirmation"),
		ReviewSummary:    firstSection(text, "Reviewer notes", "Review notes", "Review summary"),
		ChangesMade:      firstSection(text, "Changes made"),
		RisksFollowUp:    firstSection(text, "Risks / follow-up"),
		BranchPR:         firstSection(text, "Branch / PR"),
	}
}

func DefaultContent(goal string, kind types.BriefKind) string {
	switch kind {
	case types.BriefPlan:
		return "# Session Brief\n\n## Goal\n" + goal + "\n\n## Current understanding\n- Not started.\n\n## Decisions\n- None.\n\n## Plan\n- Not started.\n\n## Open questions\n- None.\n\n## Next steps\n- Pick the first concrete implementation step.\n"
	case types.BriefImplementation:
		return "# Session Brief\n\n## Summary\n- Not started.\n\n## Changes made\n- None.\n\n## Tests run\n- Not run.\n\n## Risks / follow-up\n- None.\n\n## Reviewer notes\n- Not ready.\n\n## Branch / PR\n- None.\n"
	case types.BriefDebug:
		return "# Session Brief\n\n## Symptom\n- Not started.\n\n## Findings\n- Not started.\n\n## Root cause\n- Not found.\n\n## Fix\n- Not started.\n\n## Verification\n- Not run.\n"
	case types.BriefReview:
		return "# Session Brief\n\n## Summary\n- Not started.\n\n## Findings\n- Not started.\n\n## Changes requested\n- None.\n\n## Tests run\n- Not run.\n\n## Reviewer notes\n- Not ready.\n"
	default:
		return "# Session Brief\n\n## Goal\n" + goal + "\n\n## Current state\n- Not started.\n\n## Decisions\n- None.\n\n## Tests run\n- Not run.\n\n## Next steps\n- Pick the first concrete implementation step.\n\n## Reviewer notes\n- Not ready.\n"
	}
}

func Render(sessionID string) (string, error) {
	b, err := os.ReadFile(home.BriefPath(sessionID))
	if err != nil {
		return "", err
	}
	return renderMarkdown(string(b)), nil
}

func Hash(rendered string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rendered)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ReadSections(sessionID string) []Section {
	b, err := os.ReadFile(home.BriefPath(sessionID))
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

func State(session types.AgentSession) string {
	rendered, err := Render(session.ID)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	if session.Brief.LastPublishError != "" {
		return "publish failed"
	}
	hash := Hash(rendered)
	if session.Brief.PublishedHash == "" {
		return "not published"
	}
	if hash != session.Brief.PublishedHash {
		return "changed since publish"
	}
	return "published"
}

func (s Summary) Preview() string {
	lines := []string{}
	add := func(label, value string) {
		value = clean(value)
		if value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("Summary", s.CurrentState)
	add("Changes", s.ChangesMade)
	add("Next", s.NextAction)
	add("Risks", firstNonEmpty(s.RisksFollowUp, s.Blockers))
	add("Tests", s.TestsRun)
	add("Review", s.ReviewSummary)
	return strings.Join(lines, "\n")
}

func WrapPublishedBlock(sessionID string, kind types.BriefKind, hash, rendered string) string {
	kindValue := string(kind)
	if kindValue == "" {
		kindValue = string(types.BriefGeneral)
	}
	return fmt.Sprintf("<!-- cmux:begin session=%s kind=%s hash=%s -->\n%s\n<!-- cmux:end -->", sessionID, kindValue, hash, strings.TrimSpace(rendered))
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

func renderMarkdown(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# Session Brief") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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
	case "- None.", "- None yet.", "- Not started.", "- Not found.", "- Not run.", "- Not run yet.", "- Not ready.", "- Not ready for review yet.", "- Not confirmed.", "- Start implementation.", "- Pick the first concrete implementation step.":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
