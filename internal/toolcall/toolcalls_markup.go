package toolcall

import (
	"encoding/json"
	"html"
	"reflect"
	"regexp"
	"strings"
)

var toolCallMarkupTagNames = []string{"tool_call", "function_call", "invoke"}
var toolCallMarkupTagPatternByName = map[string]*regexp.Regexp{
	"tool_call":     regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?tool_call\b([^>]*)>(.*?)</(?:[a-z0-9_:-]+:)?tool_call>`),
	"function_call": regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?function_call\b([^>]*)>(.*?)</(?:[a-z0-9_:-]+:)?function_call>`),
	"invoke":        regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?invoke\b([^>]*)>(.*?)</(?:[a-z0-9_:-]+:)?invoke>`),
}
var toolCallMarkupSelfClosingPattern = regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?invoke\b([^>]*)/>`)
var toolCallMarkupKVPattern = regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?([a-z0-9_\-.]+)\b([^>]*)>(.*?)</(?:[a-z0-9_:-]+:)?([a-z0-9_\-.]+)>`)
var toolCallMarkupAttrPattern = regexp.MustCompile(`(?is)(name|function|tool)\s*=\s*["']([^"']+)["']`)
var directNamedMarkupParamOpenPattern = regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?(?:parameter|argument)\b([^>]*)>`)
var toolCallMarkupInnerNameAttrPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?tool_name\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*>`),
	regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?function_name\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*>`),
}
var anyTagPattern = regexp.MustCompile(`(?is)<[^>]+>`)
var toolCallMarkupNameTagNames = []string{"name", "function"}
var toolCallMarkupNamePatternByTag = map[string]*regexp.Regexp{
	"name":     regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?name\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?name>`),
	"function": regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?function\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?function>`),
}

// cdataPattern matches a standalone CDATA section.
var cdataPattern = regexp.MustCompile(`(?is)^<!\[CDATA\[(.*?)]]>$`)
var toolCallMarkupArgsTagNames = []string{"input", "arguments", "argument", "parameters", "parameter", "args", "params"}
var toolCallMarkupArgsPatternByTag = map[string]*regexp.Regexp{
	"input":      regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?input\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?input>`),
	"arguments":  regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?arguments\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?arguments>`),
	"argument":   regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?argument\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?argument>`),
	"parameters": regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?parameters\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?parameters>`),
	"parameter":  regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?parameter\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?parameter>`),
	"args":       regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?args\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?args>`),
	"params":     regexp.MustCompile(`(?is)<(?:[a-z0-9_:-]+:)?params\b[^>]*>(.*?)</(?:[a-z0-9_:-]+:)?params>`),
}

func parseMarkupToolCalls(text string) []ParsedToolCall {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	out := make([]ParsedToolCall, 0)
	for _, tagName := range toolCallMarkupTagNames {
		pattern := toolCallMarkupTagPatternByName[tagName]
		for _, m := range pattern.FindAllStringSubmatch(trimmed, -1) {
			if len(m) < 3 {
				continue
			}
			attrs := strings.TrimSpace(m[1])
			inner := strings.TrimSpace(m[2])
			if parsed := parseMarkupSingleToolCall(attrs, inner); parsed.Name != "" {
				out = append(out, parsed)
			}
		}
	}
	for _, m := range toolCallMarkupSelfClosingPattern.FindAllStringSubmatch(trimmed, -1) {
		if len(m) < 2 {
			continue
		}
		if parsed := parseMarkupSingleToolCall(strings.TrimSpace(m[1]), ""); parsed.Name != "" {
			out = append(out, parsed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseMarkupSingleToolCall(attrs string, inner string) ParsedToolCall {
	// Try parsing inner content as a JSON tool call object.
	if raw := strings.TrimSpace(inner); raw != "" && strings.HasPrefix(raw, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err == nil {
			name, _ := obj["name"].(string)
			if name == "" {
				if fn, ok := obj["function"].(map[string]any); ok {
					name, _ = fn["name"].(string)
				}
			}
			if name == "" {
				if fc, ok := obj["functionCall"].(map[string]any); ok {
					name, _ = fc["name"].(string)
				}
			}
			if strings.TrimSpace(name) != "" {
				input := parseToolCallInput(obj["input"])
				if len(input) == 0 {
					if args, ok := obj["arguments"]; ok {
						input = parseToolCallInput(args)
					}
				}
				return ParsedToolCall{Name: strings.TrimSpace(name), Input: input}
			}
		}
	}

	name := ""
	if m := toolCallMarkupAttrPattern.FindStringSubmatch(attrs); len(m) >= 3 {
		name = strings.TrimSpace(m[2])
	}
	if name == "" {
		name = extractMarkupToolName(inner)
	}
	if name == "" {
		return ParsedToolCall{}
	}

	input := map[string]any{}
	if argsRaw := findMarkupTagValue(inner, toolCallMarkupArgsTagNames, toolCallMarkupArgsPatternByTag); argsRaw != "" {
		input = parseMarkupInput(argsRaw)
	} else if kv := parseMarkupKVObject(inner); len(kv) > 0 {
		input = kv
	}
	input = mergeDirectNamedMarkupParams(input, inner)
	return ParsedToolCall{Name: name, Input: input}
}

func parseMarkupInput(raw string) map[string]any {
	return parseStructuredToolCallInput(raw)
}

func parseMarkupKVObject(text string) map[string]any {
	matches := toolCallMarkupKVPattern.FindAllStringSubmatch(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		key := strings.TrimSpace(m[1])
		attrs := strings.TrimSpace(m[2])
		endKey := strings.TrimSpace(m[4])
		if key == "" {
			continue
		}
		if !strings.EqualFold(key, endKey) {
			continue
		}
		if strings.EqualFold(key, "parameter") || strings.EqualFold(key, "argument") {
			if attrMatch := toolCallMarkupAttrPattern.FindStringSubmatch(attrs); len(attrMatch) >= 3 && strings.EqualFold(attrMatch[1], "name") {
				key = strings.TrimSpace(attrMatch[2])
			}
		}
		value := parseMarkupValue(m[3])
		if value == nil || strings.TrimSpace(key) == "" {
			continue
		}
		appendMarkupValue(out, key, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractMarkupToolName(inner string) string {
	name := findMarkupTagValue(inner, toolCallMarkupNameTagNames, toolCallMarkupNamePatternByTag)
	if name != "" {
		return name
	}
	for _, pattern := range toolCallMarkupInnerNameAttrPatterns {
		if m := pattern.FindStringSubmatch(inner); len(m) >= 2 {
			if v := strings.TrimSpace(html.UnescapeString(m[1])); v != "" {
				return v
			}
		}
	}
	return ""
}

func mergeDirectNamedMarkupParams(base map[string]any, inner string) map[string]any {
 out := map[string]any{}
 for k, v := range base {
 out[k] = v
 }
 found := false
 lower := strings.ToLower(inner)
 matches := directNamedMarkupParamOpenPattern.FindAllStringSubmatchIndex(inner, -1)
 for _, loc := range matches {
 if len(loc) < 4 {
 continue
 }
 attrs := inner[loc[2]:loc[3]]
 attrMatch := toolCallMarkupAttrPattern.FindStringSubmatch(attrs)
 if len(attrMatch) < 3 || !strings.EqualFold(strings.TrimSpace(attrMatch[1]), "name") {
 continue
 }
 key := strings.TrimSpace(html.UnescapeString(attrMatch[2]))
 if key == "" {
 continue
 }
 bodyStart := loc[1]
 restLower := lower[bodyStart:]
 end := len(inner)
 for _, marker := range []string{"</parameter>", "</argument>", "<parameter", "<argument", "<tool_call", "</tool_call>", "</tool_calls>"} {
 idx := strings.Index(restLower, marker)
 if idx >= 0 && bodyStart+idx < end {
 end = bodyStart + idx
 }
 }
 value := extractRawTagValue(inner[bodyStart:end])
 if parsed := parseStructuredToolCallInput(value); len(parsed) > 0 {
 if isOnlyRawValue(parsed, value) {
 if existing, ok := out[key]; !ok || !reflect.DeepEqual(existing, value) {
 appendMarkupValue(out, key, value)
 }
 } else {
 if existing, ok := out[key]; !ok || !reflect.DeepEqual(existing, parsed) {
 appendMarkupValue(out, key, parsed)
 }
 }
 } else if strings.TrimSpace(value) != "" {
 if existing, ok := out[key]; !ok || !reflect.DeepEqual(existing, value) {
 appendMarkupValue(out, key, value)
 }
 }
 found = true
 }
 if found {
 if _, ok := out["_raw"]; ok && len(out) > 1 {
 delete(out, "_raw")
 }
 }
 return out
}

func parseMarkupValue(inner string) any {
	value := strings.TrimSpace(extractRawTagValue(inner))
	if value == "" {
		return ""
	}

	if strings.Contains(value, "<") && strings.Contains(value, ">") {
		if parsed := parseStructuredToolCallInput(value); len(parsed) > 0 {
			if len(parsed) == 1 {
				if raw, ok := parsed["_raw"].(string); ok {
					return raw
				}
			}
			return parsed
		}
	}

	var jsonValue any
	if json.Unmarshal([]byte(value), &jsonValue) == nil {
		return jsonValue
	}
	return value
}

func appendMarkupValue(out map[string]any, key string, value any) {
	if existing, ok := out[key]; ok {
		switch current := existing.(type) {
		case []any:
			out[key] = append(current, value)
		default:
			out[key] = []any{current, value}
		}
		return
	}
	out[key] = value
}

// extractRawTagValue treats the inner content of a tag robustly.
// It detects CDATA and strips it, otherwise it unescapes standard HTML entities.
// It avoids over-aggressive tag stripping that might break user content.
func extractRawTagValue(inner string) string {
	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return ""
	}

	// 1. Check for CDATA - if present, it's the ultimate "safe" container.
	if cdataMatches := cdataPattern.FindStringSubmatch(trimmed); len(cdataMatches) >= 2 {
		return cdataMatches[1] // Return raw content between CDATA brackets
	}

	// 2. If no CDATA, we still want to be robust.
	// We unescape standard HTML entities (like &lt; &gt; &amp;)
	// but we DON'T recursively strip tags unless they are actually valid XML tags
	// at the start/end (which should have been handled by the outer matcher anyway).

	// If it contains what looks like a single tag and no other text, it might be nested XML
	// but for KV objects we usually want the value.
	return html.UnescapeString(inner)
}

func stripTagText(text string) string {
	return strings.TrimSpace(anyTagPattern.ReplaceAllString(text, ""))
}

func findMarkupTagValue(text string, tagNames []string, patternByTag map[string]*regexp.Regexp) string {
	for _, tag := range tagNames {
		pattern := patternByTag[tag]
		if pattern == nil {
			continue
		}
		if m := pattern.FindStringSubmatch(text); len(m) >= 2 {
			value := extractRawTagValue(m[1])
			if value != "" {
				return value
			}
		}
	}
	return ""
}
