package chat

import (
	"regexp"
	"strings"
)

var (
	reasoningEffortPattern = regexp.MustCompile(`(?s)Reasoning Effort:.*?(?:\n\n|\z)`)
	roleMarkerPattern      = regexp.MustCompile(`^\d+\.\s*(ASSISTANT|USER|SYSTEM|TOOL)\s*$`)
)

func extractNewHistoryContent(oldText, newText string) string {
	oldClean := removeReasoningEffortBlock(strings.TrimSpace(oldText))
	newClean := removeReasoningEffortBlock(strings.TrimSpace(newText))

	if oldClean == "" {
		return newClean
	}

	oldLines := splitLines(oldClean)
	newLines := splitLines(newClean)

	commonPrefixLen := 0
	minLen := min(len(oldLines), len(newLines))
	for commonPrefixLen < minLen && strings.TrimSpace(oldLines[commonPrefixLen]) == strings.TrimSpace(newLines[commonPrefixLen]) {
		commonPrefixLen++
	}

	if commonPrefixLen >= len(newLines) {
		return ""
	}

	newPartLines := newLines[commonPrefixLen:]
	return strings.Join(newPartLines, "\n")
}

func removeReasoningEffortBlock(text string) string {
	loc := reasoningEffortPattern.FindStringIndex(text)
	if loc == nil {
		return text
	}
	return strings.TrimSpace(text[:loc[0]])
}

func isUserContentLine(line string) bool {
	if line == "" {
		return false
	}
	if roleMarkerPattern.MatchString(line) {
		return false
	}
	return true
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func BuildSupplementContent(baseContext, newUserInput string) string {
	var b strings.Builder

	if strings.TrimSpace(baseContext) != "" {
		b.WriteString(baseContext)
		if !strings.HasSuffix(baseContext, "\n") {
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String())
}
