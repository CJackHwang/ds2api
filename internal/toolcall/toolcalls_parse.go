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
	// SourceText is the raw text actually parsed to produce this result.
	// It may differ from the primary rawText when the parser fell back to
	// the thinking block. RunShadowDiff uses this to ensure both sides
	// compare against the same input.
	SourceText string
	// AvailableNames is the tool-name list passed to the parser. Stored here
	// so RunShadowDiff can replay buildParseCandidate with the same input
	// and obtain a valid nameWhitelistHit confidence signal.
	AvailableNames []string
}

func ParseToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseToolCallsDetailed(text, availableToolNames).Calls
}

func ParseToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	return parseToolCallsDetailedXMLOnly(text, availableToolNames)
}

func ParseStandaloneToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseStandaloneToolCallsDetailed(text, availableToolNames).Calls
}

func ParseStandaloneToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	return parseToolCallsDetailedXMLOnly(text, availableToolNames)
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

// parseCandidate is the private intermediate representation produced by the
// parser pipeline. It is kept separate from ToolCallParseResult so that M2
// can attach additional confidence signals without modifying the public API.
type parseCandidate struct {
	sawToolCallSyntax bool
	calls             []ParsedToolCall
	rejectedToolNames []string
	rejectedByPolicy  bool
	// M2 confidence signals – internal only, surfaced via structured logs.
	parsePath        string // see parsePathXxx constants in toolcalls_candidates.go
	ambiguous        bool   // true when both DSML and canonical wrapper syntax coexist
	nameWhitelistHit bool   // true when ≥1 parsed call name is in availableNames
}

func (c parseCandidate) toResult() ToolCallParseResult {
	return ToolCallParseResult{
		Calls:             c.calls,
		SawToolCallSyntax: c.sawToolCallSyntax,
		RejectedByPolicy:  c.rejectedByPolicy,
		RejectedToolNames: c.rejectedToolNames,
	}
}

func parseToolCallsDetailedXMLOnly(text string, availableNames []string) ToolCallParseResult {
	r := buildParseCandidate(text, availableNames).toResult()
	r.SourceText = text
	r.AvailableNames = availableNames
	return r
}

func buildParseCandidate(text string, availableNames []string) parseCandidate {
	cand := parseCandidate{}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		cand.parsePath = parsePathEmpty
		return cand
	}
	trimmed = stripFencedCodeBlocks(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		cand.parsePath = parsePathStrippedEmpty
		return cand
	}

	normalized, ok := normalizeDSMLToolCallMarkup(trimmed)
	if !ok {
		cand.parsePath = parsePathNormalizeFailed
		return cand
	}
	cand.sawToolCallSyntax = looksLikeToolCallSyntax(normalized) || hasRepairableXMLToolCallsWrapper(normalized)
	hasDSML, hasCanonical := ContainsToolCallWrapperSyntaxOutsideIgnored(normalized)
	cand.ambiguous = hasDSML && hasCanonical
	parsed := parseXMLToolCalls(normalized)
	usedCDATARecover := false
	if len(parsed) == 0 && indexToolCDATAOpen(normalized, 0) >= 0 {
		recovered := SanitizeLooseCDATA(normalized)
		if recovered != normalized {
			parsed = parseXMLToolCalls(recovered)
			if len(parsed) > 0 {
				usedCDATARecover = true
			}
		}
	}
	if len(parsed) == 0 {
		cand.parsePath = parsePathXMLFailed
		return cand
	}

	cand.sawToolCallSyntax = true
	cand.calls, cand.rejectedToolNames = filterToolCallsDetailed(parsed)
	cand.rejectedByPolicy = len(cand.rejectedToolNames) > 0 && len(cand.calls) == 0
	if usedCDATARecover {
		cand.parsePath = parsePathXMLCDATARecover
	} else {
		cand.parsePath = parsePathXMLDirect
	}
	cand.nameWhitelistHit = namesHitWhitelist(cand.calls, availableNames)
	return cand
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
	hasDSML, hasCanonical := ContainsToolCallWrapperSyntaxOutsideIgnored(text)
	return hasDSML || hasCanonical
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
	cdataFenceMarker := ""
	// Track builder length when a fence opens so we can preserve content
	// collected before the unclosed fence.
	beforeFenceLen := 0
	for _, line := range lines {
		if inCDATA || cdataStartsBeforeFence(line) {
			b.WriteString(line)
			inCDATA, cdataFenceMarker = updateCDATAStateForStrip(inCDATA, cdataFenceMarker, line)
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if marker, ok := parseFenceOpen(trimmed); ok {
				inFence = true
				fenceMarker = marker
				beforeFenceLen = b.Len()
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
		// Unclosed fence: preserve content that was collected before the
		// fence started rather than dropping everything.
		result := b.String()
		if beforeFenceLen > 0 && beforeFenceLen <= len(result) {
			return result[:beforeFenceLen]
		}
		return ""
	}
	return b.String()
}

func markdownCodeSpanEnd(text string, start int) (int, bool) {
	if start < 0 || start >= len(text) || text[start] != '`' {
		return start, false
	}
	count := countLeadingFenceChars(text[start:], '`')
	if count == 0 {
		return start, false
	}
	search := start + count
	for search < len(text) {
		if text[search] != '`' {
			search++
			continue
		}
		run := countLeadingFenceChars(text[search:], '`')
		if run == count {
			return search + run, true
		}
		search += run
	}
	return start, false
}

func cdataStartsBeforeFence(line string) bool {
	cdataIdx := indexToolCDATAOpen(line, 0)
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

func updateCDATAStateForStrip(inCDATA bool, cdataFenceMarker, line string) (bool, string) {
	pos := 0
	state := inCDATA
	fenceMarker := cdataFenceMarker
	lineForFence := line
	if !state {
		start := indexToolCDATAOpen(line, pos)
		if start < 0 {
			return false, ""
		}
		pos = start + toolCDATAOpenLenAt(line, start)
		if pos > len(line) {
			pos = len(line)
		}
		state = true
		lineForFence = line[pos:]
	}
	if !state {
		return false, ""
	}

	trimmed := strings.TrimLeft(lineForFence, " \t")
	if fenceMarker == "" {
		if marker, ok := parseFenceOpen(trimmed); ok {
			fenceMarker = marker
		}
	} else if isFenceClose(trimmed, fenceMarker) {
		fenceMarker = ""
	}

	for pos < len(line) {
		endPos := -1
		closeLen := 0
		for search := pos; search < len(line); search++ {
			if foundLen := toolCDATACloseLenAt(line, search); foundLen > 0 {
				endPos = search
				closeLen = foundLen
				break
			}
		}
		if endPos < 0 {
			return true, fenceMarker
		}
		pos = endPos + closeLen
		if pos > len(line) {
			pos = len(line)
		}
		if fenceMarker != "" {
			continue
		}
		if cdataEndLooksStructural(line, pos) || strings.TrimSpace(line[pos:]) == "" {
			state = false
			for pos < len(line) {
				start := indexToolCDATAOpen(line, pos)
				if start < 0 {
					return false, ""
				}
				pos = start + toolCDATAOpenLenAt(line, start)
				if pos > len(line) {
					pos = len(line)
				}
				state = true
				trimmedTail := strings.TrimLeft(line[pos:], " \t")
				if marker, ok := parseFenceOpen(trimmedTail); ok {
					fenceMarker = marker
				} else {
					fenceMarker = ""
				}
				break
			}
			continue
		}
	}
	return state, fenceMarker
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
