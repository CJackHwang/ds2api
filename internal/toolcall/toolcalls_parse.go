package toolcall

import (
	"strings"
)

type ParsedToolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolCallParseResult struct {
	Calls             []ParsedToolCall
	SawToolCallSyntax bool
	RejectedByPolicy  bool
	RejectedToolNames []string
}

func ParseToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseToolCallsDetailed(text, availableToolNames).Calls
}

func ParseToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	return parseToolCallsDetailedXMLOnly(text)
}

func ParseStandaloneToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseStandaloneToolCallsDetailed(text, availableToolNames).Calls
}

func ParseStandaloneToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	return parseToolCallsDetailedXMLOnly(text)
}

func ParseAssistantToolCallsDetailed(text, thinking string, availableToolNames []string) ToolCallParseResult {
	textParsed := ParseStandaloneToolCallsDetailed(text, availableToolNames)
	if len(textParsed.Calls) > 0 {
		return textParsed
	}
	if strings.TrimSpace(text) != "" {
		return textParsed
	}
	thinkingParsed := ParseStandaloneToolCallsDetailed(thinking, availableToolNames)
	if len(thinkingParsed.Calls) > 0 {
		return thinkingParsed
	}
	return textParsed
}

func parseToolCallsDetailedXMLOnly(text string) ToolCallParseResult {
	result := ToolCallParseResult{}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return result
	}
	result.SawToolCallSyntax = looksLikeToolCallSyntax(trimmed)
	trimmed = normalizeDSMLToolCallSyntax(trimmed)
	trimmed = stripFencedCodeBlocks(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return result
	}

	parsed := parseXMLToolCalls(trimmed)
	if len(parsed) == 0 {
		return result
	}

	result.SawToolCallSyntax = true
	calls, rejectedNames := filterToolCallsDetailed(parsed)
	result.Calls = calls
	result.RejectedToolNames = rejectedNames
	result.RejectedByPolicy = len(rejectedNames) > 0 && len(calls) == 0
	return result
}

func filterToolCallsDetailed(parsed []ParsedToolCall) ([]ParsedToolCall, []string) {
	out := make([]ParsedToolCall, 0, len(parsed))
	for _, tc := range parsed {
		if tc.Name == "" {
			continue
		}
		if tc.Input == nil {
			tc.Input = map[string]any{}
		}
		out = append(out, tc)
	}
	return out, nil
}

func looksLikeToolCallSyntax(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "<|dsml|tool_calls") || strings.Contains(lower, "<tool_calls")
}

func stripFencedCodeBlocks(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text))

	lines := strings.SplitAfter(text, "\n")
	inFence := false
	fenceMarker := ""
	inCDATA := false
	for _, line := range lines {
		if inCDATA || cdataStartsBeforeFence(line) {
			b.WriteString(line)
			inCDATA = updateCDATAState(inCDATA, line)
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if marker, ok := parseFenceOpen(trimmed); ok {
				inFence = true
				fenceMarker = marker
				continue
			}
			b.WriteString(line)
			continue
		}

		if isFenceClose(trimmed, fenceMarker) {
			inFence = false
			fenceMarker = ""
		}
	}

	if inFence {
		return ""
	}
	return b.String()
}

func cdataStartsBeforeFence(line string) bool {
	cdataIdx := strings.Index(strings.ToLower(line), "<![cdata[")
	if cdataIdx < 0 {
		return false
	}
	fenceIdx := firstFenceMarkerIndex(line)
	return fenceIdx < 0 || cdataIdx < fenceIdx
}

func firstFenceMarkerIndex(line string) int {
	idxBacktick := strings.Index(line, "```")
	idxTilde := strings.Index(line, "~~~")
	switch {
	case idxBacktick < 0:
		return idxTilde
	case idxTilde < 0:
		return idxBacktick
	case idxBacktick < idxTilde:
		return idxBacktick
	default:
		return idxTilde
	}
}

func updateCDATAState(inCDATA bool, line string) bool {
	lower := strings.ToLower(line)
	pos := 0
	state := inCDATA
	for pos < len(lower) {
		if state {
			end := strings.Index(lower[pos:], "]]>")
			if end < 0 {
				return true
			}
			pos += end + len("]]>")
			state = false
			continue
		}
		start := strings.Index(lower[pos:], "<![cdata[")
		if start < 0 {
			return false
		}
		pos += start + len("<![cdata[")
		state = true
	}
	return state
}

func parseFenceOpen(line string) (string, bool) {
	if len(line) < 3 {
		return "", false
	}
	ch := line[0]
	if ch != '`' && ch != '~' {
		return "", false
	}
	count := countLeadingFenceChars(line, ch)
	if count < 3 {
		return "", false
	}
	return strings.Repeat(string(ch), count), true
}

func isFenceClose(line, marker string) bool {
	if marker == "" {
		return false
	}
	ch := marker[0]
	if line == "" || line[0] != ch {
		return false
	}
	count := countLeadingFenceChars(line, ch)
	if count < len(marker) {
		return false
	}
	rest := strings.TrimSpace(line[count:])
	return rest == ""
}

func countLeadingFenceChars(line string, ch byte) int {
	count := 0
	for count < len(line) && line[count] == ch {
		count++
	}
	return count
}
