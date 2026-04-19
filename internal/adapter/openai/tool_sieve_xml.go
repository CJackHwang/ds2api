package openai

import (
	"ds2api/internal/toolcall"
	"regexp"
	"strings"
)

// --- XML tool call support for the streaming sieve ---

//nolint:unused // kept as explicit tag inventory for future XML sieve refinements.
var xmlToolCallClosingTags = []string{"</tool_calls>", "</tool_call>", "</invoke>", "</function_call>", "</function_calls>", "</tool_use>",
	// Agent-style XML tags (Roo Code, Cline, etc.)
	"</attempt_completion>", "</ask_followup_question>", "</new_task>", "</result>"}
var xmlToolCallOpeningTags = []string{"<tool_calls", "<tool_call", "<invoke", "<function_call", "<function_calls", "<tool_use",
	// Agent-style XML tags
	"<attempt_completion", "<ask_followup_question", "<new_task", "<result"}

// xmlToolCallTagPairs maps each opening tag to its expected closing tag.
// Order matters: longer/wrapper tags must be checked first.
var xmlToolCallTagPairs = []struct{ open, close string }{
	{"<tool_calls", "</tool_calls>"},
	{"<tool_call", "</tool_call>"},
	{"<function_calls", "</function_calls>"},
	{"<function_call", "</function_call>"},
	{"<invoke", "</invoke>"},
	{"<tool_use", "</tool_use>"},
	// Agent-style: these are XML "tool call" patterns from coding agents.
	// They get captured → parsed. If parsing fails, the raw XML is preserved
	// so the caller can still see the original text.
	{"<attempt_completion", "</attempt_completion>"},
	{"<ask_followup_question", "</ask_followup_question>"},
	{"<new_task", "</new_task>"},
}

// xmlToolCallBlockPattern matches a complete XML tool call block (wrapper or standalone).
//
//nolint:unused // reserved for future fast-path XML block detection.
var xmlToolCallBlockPattern = regexp.MustCompile(`(?is)(<tool_calls>\s*(?:.*?)\s*</tool_calls>|<tool_call>\s*(?:.*?)\s*</tool_call>|<invoke\b[^>]*>(?:.*?)</invoke>|<function_calls?\b[^>]*>(?:.*?)</function_calls?>|<tool_use>(?:.*?)</tool_use>|<attempt_completion>(?:.*?)</attempt_completion>|<ask_followup_question>(?:.*?)</ask_followup_question>|<new_task>(?:.*?)</new_task>)`)

// xmlToolTagsToDetect is the set of XML tag prefixes used by findToolSegmentStart.
var xmlToolTagsToDetect = []string{"<tool_calls>", "<tool_calls\n", "<tool_call>", "<tool_call\n",
	"<invoke ", "<invoke>", "<function_call", "<function_calls", "<tool_use>",
	// Agent-style tags
	"<attempt_completion>", "<ask_followup_question>", "<new_task>"}

// consumeXMLToolCapture tries to extract complete XML tool call blocks from captured text.
func consumeXMLToolCapture(captured string, toolNames []string) (prefix string, calls []toolcall.ParsedToolCall, suffix string, ready bool) {
	lower := strings.ToLower(captured)
	// Find the FIRST matching open/close pair, preferring wrapper tags.
	// Tag pairs are ordered longest-first (e.g. <tool_calls before <tool_call)
	// so wrapper tags are checked before inner tags.
	for _, pair := range xmlToolCallTagPairs {
		openIdx := strings.Index(lower, pair.open)
		if openIdx < 0 {
			continue
		}
		// Find the LAST occurrence of the specific closing tag to get the outermost block.
		closeIdx := strings.LastIndex(lower, pair.close)
		if closeIdx < openIdx {
			// Opening tag is present but its specific closing tag hasn't arrived.
			// Return not-ready so we keep buffering — do NOT fall through to
			// try inner pairs (e.g. <tool_call inside <tool_calls).
			return "", nil, "", false
		}
		closeEnd := closeIdx + len(pair.close)

		xmlBlock := captured[openIdx:closeEnd]
		prefixPart := captured[:openIdx]
		suffixPart := captured[closeEnd:]
		parsed := toolcall.ParseToolCalls(xmlBlock, toolNames)
		if len(parsed) > 0 {
			prefixPart, suffixPart = trimWrappingJSONFence(prefixPart, suffixPart)
			return prefixPart, parsed, suffixPart, true
		}
		// If this block failed to become a tool call, pass it through as text.
		return prefixPart + xmlBlock, nil, suffixPart, true
	}
	return "", nil, "", false
}

// hasOpenXMLToolTag returns true if captured text contains an XML tool opening tag
// whose SPECIFIC closing tag has not appeared yet.
func hasOpenXMLToolTag(captured string) bool {
	lower := strings.ToLower(captured)
	for _, pair := range xmlToolCallTagPairs {
		if strings.Contains(lower, pair.open) {
			if !strings.Contains(lower, pair.close) {
				return true
			}
		}
	}
	return false
}

// findPartialXMLToolTagStart checks if the string ends with a partial XML tool tag
// (e.g., "<tool_ca" or "<inv") and returns the position of the '<'.
func findPartialXMLToolTagStart(s string) int {
	lastLT := strings.LastIndex(s, "<")
	if lastLT < 0 {
		return -1
	}
	tail := s[lastLT:]
	// If there's a '>' in the tail, the tag is closed — not partial.
	if strings.Contains(tail, ">") {
		return -1
	}
	lowerTail := strings.ToLower(tail)
	// Check if the tail is a prefix of any known XML tool tag.
	for _, tag := range xmlToolCallOpeningTags {
		tagWithLT := tag
		if !strings.HasPrefix(tagWithLT, "<") {
			tagWithLT = "<" + tagWithLT
		}
		if strings.HasPrefix(tagWithLT, lowerTail) {
			return lastLT
		}
	}
	return -1
}
