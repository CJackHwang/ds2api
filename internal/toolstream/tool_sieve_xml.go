package toolstream

import (
	"ds2api/internal/toolcall"
	"regexp"
	"strings"
)

// --- XML tool call support for the streaming sieve ---

//nolint:unused // kept as explicit tag inventory for future XML sieve refinements.
var xmlToolCallClosingTags = []string{"</tool_calls>", "</|dsml|tool_calls>", "</dsml|tool_calls>"}
var xmlToolCallOpeningTags = []string{"<tool_calls", "<invoke", "<|dsml|tool_calls", "<|dsml|invoke", "<dsml|tool_calls", "<dsml|invoke"}

// xmlToolCallTagPairs maps each opening tag to its expected closing tag.
// Order matters: longer/wrapper tags must be checked first.
var xmlToolCallTagPairs = []struct{ open, close string }{
	{"<tool_calls", "</tool_calls>"},
	{"<|dsml|tool_calls", "</|dsml|tool_calls>"},
	{"<dsml|tool_calls", "</dsml|tool_calls>"},
}

// xmlToolCallBlockPattern matches a complete canonical XML tool call block.
//
//nolint:unused // reserved for future fast-path XML block detection.
var xmlToolCallBlockPattern = regexp.MustCompile(`(?is)(<tool_calls\b[^>]*>\s*(?:.*?)\s*</tool_calls>|<\|dsml\|tool_calls\b[^>]*>\s*(?:.*?)\s*</\|dsml\|tool_calls>|<dsml\|tool_calls\b[^>]*>\s*(?:.*?)\s*</dsml\|tool_calls>)`)

// xmlToolTagsToDetect is the set of XML tag prefixes used by findToolSegmentStart.
var xmlToolTagsToDetect = []string{"<tool_calls>", "<tool_calls\n", "<tool_calls ", "<invoke ", "<invoke\n", "<invoke\t", "<invoke\r", "<|dsml|tool_calls>", "<|dsml|tool_calls\n", "<|dsml|tool_calls ", "<|dsml|invoke ", "<|dsml|invoke\n", "<|dsml|invoke\t", "<|dsml|invoke\r", "<dsml|tool_calls>", "<dsml|tool_calls\n", "<dsml|tool_calls ", "<dsml|invoke ", "<dsml|invoke\n", "<dsml|invoke\t", "<dsml|invoke\r"}

// consumeXMLToolCapture tries to extract complete XML tool call blocks from captured text.
func consumeXMLToolCapture(captured string, toolNames []string) (prefix string, calls []toolcall.ParsedToolCall, suffix string, ready bool) {
	lower := strings.ToLower(captured)
	// Find the FIRST matching open/close pair for the canonical wrapper.
	for _, pair := range xmlToolCallTagPairs {
		openIdx := strings.Index(lower, pair.open)
		if openIdx < 0 {
			continue
		}
		// Find the matching closing tag outside CDATA. Long write-file tool
		// calls often contain XML examples in CDATA, including </tool_calls>.
		closeIdx := findXMLCloseOutsideCDATA(captured, pair.close, openIdx+len(pair.open))
		if closeIdx < 0 {
			// Opening tag is present but its specific closing tag hasn't arrived.
			// Return not-ready so we keep buffering until the canonical wrapper closes.
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
	if !strings.Contains(lower, "<tool_calls") && !strings.Contains(lower, "<|dsml|tool_calls") && !strings.Contains(lower, "<dsml|tool_calls") {
		invokeIdx := strings.Index(lower, "<invoke")
		closeIdx := findXMLCloseOutsideCDATA(captured, "</tool_calls>", invokeIdx)
		if invokeIdx >= 0 && closeIdx > invokeIdx {
			closeEnd := closeIdx + len("</tool_calls>")
			xmlBlock := "<tool_calls>" + captured[invokeIdx:closeIdx] + "</tool_calls>"
			prefixPart := captured[:invokeIdx]
			suffixPart := captured[closeEnd:]
			parsed := toolcall.ParseToolCalls(xmlBlock, toolNames)
			if len(parsed) > 0 {
				prefixPart, suffixPart = trimWrappingJSONFence(prefixPart, suffixPart)
				return prefixPart, parsed, suffixPart, true
			}
			return prefixPart + captured[invokeIdx:closeEnd], nil, suffixPart, true
		}
		dsmlInvokeIdx := strings.Index(lower, "<|dsml|invoke")
		dsmlCloseIdx := findXMLCloseOutsideCDATA(captured, "</|dsml|tool_calls>", dsmlInvokeIdx)
		if dsmlInvokeIdx >= 0 && dsmlCloseIdx > dsmlInvokeIdx {
			closeEnd := dsmlCloseIdx + len("</|dsml|tool_calls>")
			xmlBlock := "<|DSML|tool_calls>" + captured[dsmlInvokeIdx:dsmlCloseIdx] + "</|DSML|tool_calls>"
			prefixPart := captured[:dsmlInvokeIdx]
			suffixPart := captured[closeEnd:]
			parsed := toolcall.ParseToolCalls(xmlBlock, toolNames)
			if len(parsed) > 0 {
				prefixPart, suffixPart = trimWrappingJSONFence(prefixPart, suffixPart)
				return prefixPart, parsed, suffixPart, true
			}
			return prefixPart + captured[dsmlInvokeIdx:closeEnd], nil, suffixPart, true
		}
		looseDSMLInvokeIdx := strings.Index(lower, "<dsml|invoke")
		looseDSMLCloseIdx := findXMLCloseOutsideCDATA(captured, "</dsml|tool_calls>", looseDSMLInvokeIdx)
		if looseDSMLInvokeIdx >= 0 && looseDSMLCloseIdx > looseDSMLInvokeIdx {
			closeEnd := looseDSMLCloseIdx + len("</dsml|tool_calls>")
			xmlBlock := "<|DSML|tool_calls>" + captured[looseDSMLInvokeIdx:looseDSMLCloseIdx] + "</|DSML|tool_calls>"
			prefixPart := captured[:looseDSMLInvokeIdx]
			suffixPart := captured[closeEnd:]
			parsed := toolcall.ParseToolCalls(xmlBlock, toolNames)
			if len(parsed) > 0 {
				prefixPart, suffixPart = trimWrappingJSONFence(prefixPart, suffixPart)
				return prefixPart, parsed, suffixPart, true
			}
			return prefixPart + captured[looseDSMLInvokeIdx:closeEnd], nil, suffixPart, true
		}
	}
	return "", nil, "", false
}

// hasOpenXMLToolTag returns true if captured text contains an XML tool opening tag
// whose SPECIFIC closing tag has not appeared yet.
func hasOpenXMLToolTag(captured string) bool {
	lower := strings.ToLower(captured)
	for _, pair := range xmlToolCallTagPairs {
		openIdx := strings.Index(lower, pair.open)
		if openIdx >= 0 {
			if findXMLCloseOutsideCDATA(captured, pair.close, openIdx+len(pair.open)) < 0 {
				return true
			}
		}
	}
	return false
}

func findXMLCloseOutsideCDATA(s, closeTag string, start int) int {
	if s == "" || closeTag == "" {
		return -1
	}
	if start < 0 {
		start = 0
	}
	lower := strings.ToLower(s)
	target := strings.ToLower(closeTag)
	for i := start; i < len(s); {
		switch {
		case strings.HasPrefix(lower[i:], "<![cdata["):
			end := strings.Index(lower[i+len("<![cdata["):], "]]>")
			if end < 0 {
				return -1
			}
			i += len("<![cdata[") + end + len("]]>")
		case strings.HasPrefix(lower[i:], "<!--"):
			end := strings.Index(lower[i+len("<!--"):], "-->")
			if end < 0 {
				return -1
			}
			i += len("<!--") + end + len("-->")
		case strings.HasPrefix(lower[i:], target):
			return i
		default:
			i++
		}
	}
	return -1
}

// findPartialXMLToolTagStart checks if the string ends with a partial canonical
// XML wrapper tag (e.g., "<too") and returns the position of the '<'.
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
