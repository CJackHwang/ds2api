package toolcall

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

var xmlAttrPattern = regexp.MustCompile(`(?is)\b([a-z0-9_:-]+)\s*=\s*("([^"]*)"|'([^']*)')`)
var xmlToolCallsClosePattern = regexp.MustCompile(`(?is)</tool_calls>`)
var xmlInvokeStartPattern = regexp.MustCompile(`(?is)<invoke\b[^>]*\bname\s*=\s*("([^"]*)"|'([^']*)')`)

func parseXMLToolCalls(text string) []ParsedToolCall {
	wrappers := findXMLElementBlocks(text, "tool_calls")
	if len(wrappers) == 0 {
		repaired := repairMissingXMLToolCallsOpeningWrapper(text)
		if repaired != text {
			wrappers = findXMLElementBlocks(repaired, "tool_calls")
		}
	}
	if len(wrappers) == 0 {
		return nil
	}
	out := make([]ParsedToolCall, 0, len(wrappers))
	for _, wrapper := range wrappers {
		for _, block := range findXMLElementBlocks(wrapper.Body, "invoke") {
			call, ok := parseSingleXMLToolCall(block)
			if !ok {
				continue
			}
			out = append(out, call)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func repairMissingXMLToolCallsOpeningWrapper(text string) string {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<tool_calls") {
		return text
	}

	closeMatches := xmlToolCallsClosePattern.FindAllStringIndex(text, -1)
	if len(closeMatches) == 0 {
		return text
	}
	invokeLoc := xmlInvokeStartPattern.FindStringIndex(text)
	if invokeLoc == nil {
		return text
	}
	closeLoc := closeMatches[len(closeMatches)-1]
	if invokeLoc[0] >= closeLoc[0] {
		return text
	}

	return text[:invokeLoc[0]] + "<tool_calls>" + text[invokeLoc[0]:closeLoc[0]] + "</tool_calls>" + text[closeLoc[1]:]
}

func parseSingleXMLToolCall(block xmlElementBlock) (ParsedToolCall, bool) {
	attrs := parseXMLTagAttributes(block.Attrs)
	name := strings.TrimSpace(html.UnescapeString(attrs["name"]))
	if name == "" {
		return ParsedToolCall{}, false
	}

	inner := strings.TrimSpace(block.Body)
	if strings.HasPrefix(inner, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(inner), &payload); err == nil {
			input := map[string]any{}
			if params, ok := payload["input"].(map[string]any); ok {
				input = params
			}
			if len(input) == 0 {
				if params, ok := payload["parameters"].(map[string]any); ok {
					input = params
				}
			}
			return ParsedToolCall{Name: name, Input: input}, true
		}
	}

	input := map[string]any{}
	for _, paramMatch := range findXMLElementBlocks(inner, "parameter") {
		paramAttrs := parseXMLTagAttributes(paramMatch.Attrs)
		paramName := strings.TrimSpace(html.UnescapeString(paramAttrs["name"]))
		if paramName == "" {
			continue
		}
		value := parseInvokeParameterValueWithStringHint(paramMatch.Body, paramAttrs["string"])
		appendMarkupValue(input, paramName, value)
	}

	if len(input) == 0 {
		if strings.TrimSpace(inner) != "" {
			return ParsedToolCall{}, false
		}
		return ParsedToolCall{Name: name, Input: map[string]any{}}, true
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

type xmlElementBlock struct {
	Attrs string
	Body  string
	Start int
	End   int
}

func findXMLElementBlocks(text, tag string) []xmlElementBlock {
	if text == "" || tag == "" {
		return nil
	}
	var out []xmlElementBlock
	pos := 0
	for pos < len(text) {
		start, bodyStart, attrs, ok := findXMLStartTagOutsideCDATA(text, tag, pos)
		if !ok {
			break
		}
		closeStart, closeEnd, ok := findMatchingXMLEndTagOutsideCDATA(text, tag, bodyStart)
		if !ok {
			break
		}
		out = append(out, xmlElementBlock{
			Attrs: attrs,
			Body:  text[bodyStart:closeStart],
			Start: start,
			End:   closeEnd,
		})
		pos = closeEnd
	}
	return out
}

func findXMLStartTagOutsideCDATA(text, tag string, from int) (start, bodyStart int, attrs string, ok bool) {
	lower := strings.ToLower(text)
	target := "<" + strings.ToLower(tag)
	for i := maxInt(from, 0); i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(lower, i)
		if blocked {
			return -1, -1, "", false
		}
		if advanced {
			i = next
			continue
		}
		if strings.HasPrefix(lower[i:], target) && hasXMLTagBoundary(text, i+len(target)) {
			end := findXMLTagEnd(text, i+len(target))
			if end < 0 {
				return -1, -1, "", false
			}
			return i, end + 1, text[i+len(target) : end], true
		}
		i++
	}
	return -1, -1, "", false
}

func findMatchingXMLEndTagOutsideCDATA(text, tag string, from int) (closeStart, closeEnd int, ok bool) {
	lower := strings.ToLower(text)
	openTarget := "<" + strings.ToLower(tag)
	closeTarget := "</" + strings.ToLower(tag)
	depth := 1
	for i := maxInt(from, 0); i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(lower, i)
		if blocked {
			return -1, -1, false
		}
		if advanced {
			i = next
			continue
		}
		if strings.HasPrefix(lower[i:], closeTarget) && hasXMLTagBoundary(text, i+len(closeTarget)) {
			end := findXMLTagEnd(text, i+len(closeTarget))
			if end < 0 {
				return -1, -1, false
			}
			depth--
			if depth == 0 {
				return i, end + 1, true
			}
			i = end + 1
			continue
		}
		if strings.HasPrefix(lower[i:], openTarget) && hasXMLTagBoundary(text, i+len(openTarget)) {
			end := findXMLTagEnd(text, i+len(openTarget))
			if end < 0 {
				return -1, -1, false
			}
			if !isSelfClosingXMLTag(text[:end]) {
				depth++
			}
			i = end + 1
			continue
		}
		i++
	}
	return -1, -1, false
}

func skipXMLIgnoredSection(lower string, i int) (next int, advanced bool, blocked bool) {
	switch {
	case strings.HasPrefix(lower[i:], "<![cdata["):
		end := strings.Index(lower[i+len("<![cdata["):], "]]>")
		if end < 0 {
			return 0, false, true
		}
		return i + len("<![cdata[") + end + len("]]>"), true, false
	case strings.HasPrefix(lower[i:], "<!--"):
		end := strings.Index(lower[i+len("<!--"):], "-->")
		if end < 0 {
			return 0, false, true
		}
		return i + len("<!--") + end + len("-->"), true, false
	default:
		return i, false, false
	}
}

func findXMLTagEnd(text string, from int) int {
	quote := byte(0)
	for i := maxInt(from, 0); i < len(text); i++ {
		ch := text[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '>' {
			return i
		}
	}
	return -1
}

func hasXMLTagBoundary(text string, idx int) bool {
	if idx >= len(text) {
		return true
	}
	switch text[idx] {
	case ' ', '\t', '\n', '\r', '>', '/':
		return true
	default:
		return false
	}
}

func isSelfClosingXMLTag(startTag string) bool {
	return strings.HasSuffix(strings.TrimSpace(startTag), "/")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseXMLTagAttributes(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, m := range xmlAttrPattern.FindAllStringSubmatch(raw, -1) {
		if len(m) < 5 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		if key == "" {
			continue
		}
		value := m[3]
		if value == "" {
			value = m[4]
		}
		out[key] = value
	}
	return out
}

func parseInvokeParameterValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if value, ok := extractStandaloneCDATA(trimmed); ok {
		return value
	}
	if parsed := parseStructuredToolCallInput(trimmed); len(parsed) > 0 {
		if len(parsed) == 1 {
			if rawValue, ok := parsed["_raw"].(string); ok {
				return rawValue
			}
		}
		return parsed
	}
	return html.UnescapeString(extractRawTagValue(trimmed))
}

func parseInvokeParameterValueWithStringHint(raw, stringAttr string) any {
	trimmedAttr := strings.TrimSpace(strings.ToLower(stringAttr))
	switch trimmedAttr {
	case "true":
		return html.UnescapeString(extractRawTagValue(strings.TrimSpace(raw)))
	case "false":
		trimmed := strings.TrimSpace(html.UnescapeString(extractRawTagValue(raw)))
		if trimmed == "" {
			return ""
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed
		}
	}
	return parseInvokeParameterValue(raw)
}
