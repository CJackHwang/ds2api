package toolcall

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"regexp"
	"strings"
)

var xmlToolCallPattern = regexp.MustCompile(`(?is)<tool_call>\s*(.*?)\s*</tool_call>`)
var functionCallPattern = regexp.MustCompile(`(?is)<function_call>\s*([^<]+?)\s*</function_call>`)
var functionParamPattern = regexp.MustCompile(`(?is)<function\s+parameter\s+name="([^"]+)"\s*>\s*(.*?)\s*</function\s+parameter>`)
var antmlFunctionCallPattern = regexp.MustCompile(`(?is)<(?:[a-z0-9_]+:)?function_call[^>]*(?:name|function)="([^"]+)"[^>]*>\s*(.*?)\s*</(?:[a-z0-9_]+:)?function_call>`)
var antmlArgumentPattern = regexp.MustCompile(`(?is)<(?:[a-z0-9_]+:)?argument\s+name="([^"]+)"\s*>\s*(.*?)\s*</(?:[a-z0-9_]+:)?argument>`)
var invokeCallPattern = regexp.MustCompile(`(?is)<invoke\s+name="([^"]+)"\s*>(.*?)</invoke>`)
var invokeParamPattern = regexp.MustCompile(`(?is)<parameter\s+name="([^"]+)"\s*>\s*(.*?)\s*</parameter>`)
var toolUseFunctionPattern = regexp.MustCompile(`(?is)<tool_use>\s*<function\s+name="([^"]+)"\s*>(.*?)</function>\s*</tool_use>`)
var toolUseNameParametersPattern = regexp.MustCompile(`(?is)<tool_use>\s*<tool_name>\s*([^<]+?)\s*</tool_name>\s*<parameters>\s*(.*?)\s*</parameters>\s*</tool_use>`)
var toolUseFunctionNameParametersPattern = regexp.MustCompile(`(?is)<tool_use>\s*<function_name>\s*([^<]+?)\s*</function_name>\s*<parameters>\s*(.*?)\s*</parameters>\s*</tool_use>`)
var toolUseToolNameBodyPattern = regexp.MustCompile(`(?is)<tool_use>\s*<tool_name>\s*([^<]+?)\s*</tool_name>\s*(.*?)\s*</tool_use>`)
var xmlToolNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?tool_name\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?tool_name>`),
	regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?function_name\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?function_name>`),
}

func parseXMLToolCalls(text string) []ParsedToolCall {
	matches := xmlToolCallPattern.FindAllString(text, -1)
	out := make([]ParsedToolCall, 0, len(matches)+1)
	for _, block := range matches {
		call, ok := parseSingleXMLToolCall(block)
		if !ok {
			continue
		}
		out = append(out, call)
	}
	if len(out) > 0 {
		return out
	}
	if call, ok := parseFunctionCallTagStyle(text); ok {
		return []ParsedToolCall{call}
	}
	if calls := parseAntmlFunctionCallStyles(text); len(calls) > 0 {
		return calls
	}
	if call, ok := parseInvokeFunctionCallStyle(text); ok {
		return []ParsedToolCall{call}
	}
	if call, ok := parseToolUseFunctionStyle(text); ok {
		return []ParsedToolCall{call}
	}
	if call, ok := parseToolUseNameParametersStyle(text); ok {
		return []ParsedToolCall{call}
	}
	if call, ok := parseToolUseFunctionNameParametersStyle(text); ok {
		return []ParsedToolCall{call}
	}
	if call, ok := parseToolUseToolNameBodyStyle(text); ok {
		return []ParsedToolCall{call}
	}
	return nil
}

func parseSingleXMLToolCall(block string) (ParsedToolCall, bool) {
	inner := strings.TrimSpace(block)
	inner = strings.TrimPrefix(inner, "<tool_call>")
	inner = strings.TrimSuffix(inner, "</tool_call>")
	inner = strings.TrimSpace(inner)
	if strings.HasPrefix(inner, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(inner), &payload); err == nil {
			name := strings.TrimSpace(asString(payload["tool"]))
			if name == "" {
				name = strings.TrimSpace(asString(payload["tool_name"]))
			}
			if name != "" {
				input := map[string]any{}
				if params, ok := payload["params"].(map[string]any); ok {
					input = params
				} else if params, ok := payload["parameters"].(map[string]any); ok {
					input = params
				}
				return ParsedToolCall{Name: name, Input: input}, true
			}
		}
	}

	name := ""
	params := extractXMLToolParamsByRegex(inner)
	dec := xml.NewDecoder(strings.NewReader(block))
	inTool := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			tag := strings.ToLower(t.Name.Local)
			switch tag {
			case "tool":
				inTool = true
				for _, attr := range t.Attr {
					if strings.EqualFold(strings.TrimSpace(attr.Name.Local), "name") && strings.TrimSpace(name) == "" {
						name = strings.TrimSpace(attr.Value)
					}
				}
			case "parameters":
				var node struct {
					Inner string `xml:",innerxml"`
				}
				if err := dec.DecodeElement(&node, &t); err == nil {
					inner := strings.TrimSpace(node.Inner)
					if inner != "" {
						extracted := extractRawTagValue(inner)
						if parsed := parseStructuredToolCallInput(extracted); len(parsed) > 0 {
							for k, vv := range parsed {
								params[k] = vv
							}
						}
					}
				}
			case "tool_name", "function_name", "name":
				var v string
				if err := dec.DecodeElement(&v, &t); err == nil && strings.TrimSpace(v) != "" {
					name = strings.TrimSpace(v)
				}
			case "input", "arguments", "argument", "args", "params":
				var v string
				if err := dec.DecodeElement(&v, &t); err == nil && strings.TrimSpace(v) != "" {
					if parsed := parseStructuredToolCallInput(strings.TrimSpace(v)); len(parsed) > 0 {
						for k, vv := range parsed {
							params[k] = vv
						}
					}
				}
			default:
				if inTool {
					var v string
					if err := dec.DecodeElement(&v, &t); err == nil {
						params[t.Name.Local] = strings.TrimSpace(html.UnescapeString(v))
					}
				}
			}
		case xml.EndElement:
			tag := strings.ToLower(t.Name.Local)
			if tag == "tool" {
				inTool = false
			}
		}
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(html.UnescapeString(extractXMLToolNameByRegex(stripTopLevelXMLParameters(inner))))
	}
	if strings.TrimSpace(name) == "" {
		return ParsedToolCall{}, false
	}
	return ParsedToolCall{Name: strings.TrimSpace(html.UnescapeString(name)), Input: params}, true
}

func stripTopLevelXMLParameters(inner string) string {
	out := strings.TrimSpace(inner)
	for {
		idx := strings.Index(strings.ToLower(out), "<parameters")
		if idx < 0 {
			return out
		}
		segment := out[idx:]
		segmentLower := strings.ToLower(segment)
		openEnd := strings.Index(segmentLower, ">")
		if openEnd < 0 {
			return out
		}
		closeIdx := strings.Index(segmentLower, "</parameters>")
		if closeIdx < 0 {
			return out[:idx]
		}
		end := idx + closeIdx + len("</parameters>")
		out = out[:idx] + out[end:]
	}
}

func extractXMLToolNameByRegex(inner string) string {
	for _, pattern := range xmlToolNamePatterns {
		if m := pattern.FindStringSubmatch(inner); len(m) >= 2 {
			if v := strings.TrimSpace(stripTagText(m[1])); v != "" {
				return v
			}
		}
	}
	return ""
}

func extractXMLToolParamsByRegex(inner string) map[string]any {
	raw := findMarkupTagValue(inner, toolCallMarkupArgsTagNames, toolCallMarkupArgsPatternByTag)
	if raw == "" {
		return map[string]any{}
	}
	parsed := parseMarkupInput(raw)
	if parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func parseFunctionCallTagStyle(text string) (ParsedToolCall, bool) {
	m := functionCallPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	input := map[string]any{}
	for _, pm := range functionParamPattern.FindAllStringSubmatch(text, -1) {
		if len(pm) < 3 {
			continue
		}
		key := strings.TrimSpace(pm[1])
		val := extractRawTagValue(pm[2])
		if key != "" {
			if parsed := parseStructuredToolCallInput(val); len(parsed) > 0 {
				if isOnlyRawValue(parsed, val) {
					input[key] = val
				} else {
					input[key] = parsed
				}
			}
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseAntmlFunctionCallStyles(text string) []ParsedToolCall {
	matches := antmlFunctionCallPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]ParsedToolCall, 0, len(matches))
	for _, m := range matches {
		if call, ok := parseSingleAntmlFunctionCallMatch(m); ok {
			out = append(out, call)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseSingleAntmlFunctionCallMatch(m []string) (ParsedToolCall, bool) {
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	body := strings.TrimSpace(m[2])
	input := map[string]any{}
	if strings.HasPrefix(body, "{") {
		if err := json.Unmarshal([]byte(body), &input); err == nil {
			return ParsedToolCall{Name: name, Input: input}, true
		}
	}
	for _, am := range antmlArgumentPattern.FindAllStringSubmatch(body, -1) {
		if len(am) < 3 {
			continue
		}
		k := strings.TrimSpace(am[1])
		v := extractRawTagValue(am[2])
		if k != "" {
			input[k] = v
		}
	}
	if len(input) > 0 {
		return ParsedToolCall{Name: name, Input: input}, true
	}
	if paramsRaw := findMarkupTagValue(body, toolCallMarkupArgsTagNames, toolCallMarkupArgsPatternByTag); paramsRaw != "" {
		if parsed := parseMarkupInput(paramsRaw); len(parsed) > 0 {
			return ParsedToolCall{Name: name, Input: parsed}, true
		}
	}
	if strings.Contains(body, "<") {
		if parsed := parseStructuredToolCallInput(body); len(parsed) > 0 && !isOnlyRawValue(parsed, body) {
			return ParsedToolCall{Name: name, Input: parsed}, true
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseInvokeFunctionCallStyle(text string) (ParsedToolCall, bool) {
	m := invokeCallPattern.FindStringSubmatch(text)
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	input := map[string]any{}
	for _, pm := range invokeParamPattern.FindAllStringSubmatch(m[2], -1) {
		if len(pm) < 3 {
			continue
		}
		k := strings.TrimSpace(pm[1])
		v := extractRawTagValue(pm[2])
		if k != "" {
			if parsed := parseStructuredToolCallInput(v); len(parsed) > 0 {
				if isOnlyRawValue(parsed, v) {
					input[k] = v
				} else {
					input[k] = parsed
				}
			}
		}
	}
	if len(input) == 0 {
		if argsRaw := findMarkupTagValue(m[2], toolCallMarkupArgsTagNames, toolCallMarkupArgsPatternByTag); argsRaw != "" {
			input = parseMarkupInput(argsRaw)
		} else if kv := parseMarkupKVObject(m[2]); len(kv) > 0 {
			input = kv
		} else if parsed := parseStructuredToolCallInput(m[2]); len(parsed) > 0 && !isOnlyRawValue(parsed, strings.TrimSpace(html.UnescapeString(m[2]))) {
			input = parsed
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseToolUseFunctionStyle(text string) (ParsedToolCall, bool) {
	m := toolUseFunctionPattern.FindStringSubmatch(text)
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	body := m[2]
	input := map[string]any{}
	for _, pm := range invokeParamPattern.FindAllStringSubmatch(body, -1) {
		if len(pm) < 3 {
			continue
		}
		k := strings.TrimSpace(pm[1])
		v := extractRawTagValue(pm[2])
		if k != "" {
			if parsed := parseStructuredToolCallInput(v); len(parsed) > 0 {
				if isOnlyRawValue(parsed, v) {
					input[k] = v
				} else {
					input[k] = parsed
				}
			}
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseToolUseNameParametersStyle(text string) (ParsedToolCall, bool) {
	m := toolUseNameParametersPattern.FindStringSubmatch(text)
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	raw := strings.TrimSpace(m[2])
	input := map[string]any{}
	if raw != "" {
		if parsed := parseStructuredToolCallInput(raw); len(parsed) > 0 {
			input = parsed
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseToolUseFunctionNameParametersStyle(text string) (ParsedToolCall, bool) {
	m := toolUseFunctionNameParametersPattern.FindStringSubmatch(text)
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	raw := strings.TrimSpace(m[2])
	input := map[string]any{}
	if raw != "" {
		if parsed := parseStructuredToolCallInput(raw); len(parsed) > 0 {
			input = parsed
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseToolUseToolNameBodyStyle(text string) (ParsedToolCall, bool) {
	m := toolUseToolNameBodyPattern.FindStringSubmatch(text)
	if len(m) < 3 {
		return ParsedToolCall{}, false
	}
	name := strings.TrimSpace(html.UnescapeString(m[1]))
	if name == "" {
		return ParsedToolCall{}, false
	}
	body := strings.TrimSpace(m[2])
	input := map[string]any{}
	if body != "" {
		if kv := parseXMLChildKV(body); len(kv) > 0 {
			input = kv
		} else if kv := parseMarkupKVObject(body); len(kv) > 0 {
			input = kv
		} else if parsed := parseStructuredToolCallInput(body); len(parsed) > 0 {
			input = parsed
		}
	}
	return ParsedToolCall{Name: name, Input: input}, true
}

func parseXMLChildKV(body string) map[string]any {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return nil
	}
	parsed := parseStructuredToolCallInput(trimmed)
	if len(parsed) == 0 {
		return nil
	}
	return parsed
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
