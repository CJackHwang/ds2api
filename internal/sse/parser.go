package sse

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"ds2api/internal/deepseek"
)

type ContentPart struct {
	Text string
	Type string
}

func ParseDeepSeekSSELine(raw []byte) (map[string]any, bool, bool) {
	line := strings.TrimSpace(string(raw))
	if line == "" || !strings.HasPrefix(line, "data:") {
		return nil, false, false
	}
	dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if dataStr == "[DONE]" {
		return nil, true, true
	}
	chunk := map[string]any{}
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		return nil, false, false
	}
	return chunk, false, true
}

func shouldSkipPath(path string) bool {
	if isFragmentStatusPath(path) {
		return true
	}
	if _, ok := deepseek.SkipExactPathSet[path]; ok {
		return true
	}
	for _, p := range deepseek.SkipContainsPatterns {
		if strings.Contains(path, p) {
			return true
		}
	}
	return false
}

func isFragmentStatusPath(path string) bool {
	if path == "" || path == "response/status" {
		return false
	}
	if !strings.HasPrefix(path, "response/fragments/") || !strings.HasSuffix(path, "/status") {
		return false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(path, "response/fragments/"), "/status")
	if mid == "" {
		return false
	}
	mid = strings.TrimPrefix(mid, "-")
	if mid == "" {
		return false
	}
	for _, r := range mid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func ParseSSEChunkForContent(chunk map[string]any, thinkingEnabled bool, currentFragmentType string) ([]ContentPart, bool, string) {
	v, ok := chunk["v"]
	if !ok {
		return nil, false, currentFragmentType
	}
	path, _ := chunk["p"].(string)
	if shouldSkipPath(path) {
		return nil, false, currentFragmentType
	}
	if isStatusPath(path) {
		if s, ok := v.(string); ok {
			if strings.EqualFold(strings.TrimSpace(s), "FINISHED") {
				return nil, true, currentFragmentType
			}
			return nil, false, currentFragmentType
		}
	}
	newType := currentFragmentType
	parts := make([]ContentPart, 0, 8)
	collectDirectFragments(path, chunk, v, &newType, &parts)
	updateTypeFromNestedResponse(path, v, &newType)
	partType := resolvePartType(path, thinkingEnabled, newType)
	finished := appendChunkValueContent(v, partType, &newType, &parts, path)
	if finished {
		return nil, true, newType
	}
	var transitioned bool
	parts, transitioned = splitThinkingParts(parts)
	if transitioned {
		newType = "text"
	}
	return parts, false, newType
}

func collectDirectFragments(path string, chunk map[string]any, v any, newType *string, parts *[]ContentPart) {
	if path != "response/fragments" {
		return
	}
	op, _ := chunk["o"].(string)
	if !strings.EqualFold(op, "APPEND") {
		return
	}
	frags, ok := v.([]any)
	if !ok {
		return
	}
	for _, frag := range frags {
		m, ok := frag.(map[string]any)
		if !ok {
			continue
		}
		typeName, content, fragType := parseFragmentTypeContent(m)
		if typeName == "" {
			typeName = fragType
		}
		switch typeName {
		case "THINK", "THINKING":
			*newType = "thinking"
			appendContentPart(parts, content, "thinking")
		case "RESPONSE":
			*newType = "text"
			appendContentPart(parts, content, "text")
		default:
			appendContentPart(parts, content, "text")
		}
	}
}

func updateTypeFromNestedResponse(path string, v any, newType *string) {
	if path != "response" {
		return
	}
	arr, ok := v.([]any)
	if !ok {
		return
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok || m["p"] != "fragments" || m["o"] != "APPEND" {
			continue
		}
		frags, ok := m["v"].([]any)
		if !ok {
			continue
		}
		for _, frag := range frags {
			fm, ok := frag.(map[string]any)
			if !ok {
				continue
			}
			typeName, _, _ := parseFragmentTypeContent(fm)
			switch typeName {
			case "THINK", "THINKING":
				*newType = "thinking"
			case "RESPONSE":
				*newType = "text"
			}
		}
	}
}

func resolvePartType(path string, thinkingEnabled bool, newType string) string {
	switch {
	case path == "response/thinking_content":
		if newType == "text" {
			return "text"
		}
		return "thinking"
	case path == "response/content":
		return "text"
	case strings.Contains(path, "response/fragments") && strings.Contains(path, "/content"):
		return newType
	case path == "" && thinkingEnabled:
		return newType
	default:
		return "text"
	}
}

func appendChunkValueContent(v any, partType string, newType *string, parts *[]ContentPart, path string) bool {
	switch val := v.(type) {
	case string:
		if val == "FINISHED" && (path == "" || path == "status") {
			return true
		}
		if isStatusPath(path) {
			return false
		}
		appendContentPart(parts, val, partType)
	case []any:
		pp, finished := extractContentRecursive(val, partType)
		if finished {
			return true
		}
		*parts = append(*parts, pp...)
	case map[string]any:
		appendWrappedFragments(val, partType, newType, parts)
	}
	return false
}

func appendWrappedFragments(val map[string]any, partType string, newType *string, parts *[]ContentPart) {
	resp := val
	if wrapped, ok := val["response"].(map[string]any); ok {
		resp = wrapped
	}
	frags, ok := resp["fragments"].([]any)
	if !ok {
		return
	}
	for _, item := range frags {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typeName, content, fragType := parseFragmentTypeContent(m)
		if typeName == "" {
			typeName = fragType
		}
		switch typeName {
		case "THINK", "THINKING":
			*newType = "thinking"
			appendContentPart(parts, content, "thinking")
		case "RESPONSE":
			*newType = "text"
			appendContentPart(parts, content, "text")
		default:
			appendContentPart(parts, content, partType)
		}
	}
}

func parseFragmentTypeContent(m map[string]any) (string, string, string) {
	typeName, _ := m["type"].(string)
	content, _ := m["content"].(string)
	return strings.ToUpper(typeName), content, strings.ToUpper(typeName)
}

func appendContentPart(parts *[]ContentPart, content, kind string) {
	if content == "" {
		return
	}
	*parts = append(*parts, ContentPart{Text: content, Type: kind})
}

var thinkClosePattern = regexp.MustCompile(`(?i)</\s*think\s*>`)
var thinkOpenPattern = regexp.MustCompile(`(?i)<\s*think\s*>`)

// splitThinkingParts detects </think> inside thinking content and
// auto-transitions everything after it to text. This handles the
// DeepSeek API bug where the upstream SSE keeps sending
// reasoning_content even though the model has finished thinking.
func splitThinkingParts(parts []ContentPart) ([]ContentPart, bool) {
	var out []ContentPart
	thinkingDone := false
	for _, p := range parts {
		if thinkingDone && p.Type == "thinking" {
			// Already transitioned — treat remaining thinking as text.
			cleaned := stripThinkTags(p.Text)
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: "text"})
			}
			continue
		}
		if p.Type != "thinking" {
			cleaned := stripThinkTags(p.Text)
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: p.Type})
			}
			continue
		}
		loc := thinkClosePattern.FindStringIndex(p.Text)
		if loc == nil {
			out = append(out, p)
			continue
		}
		// Split at </think>: before is still thinking, after becomes text.
		thinkingDone = true
		before := p.Text[:loc[0]]
		after := p.Text[loc[1]:]
		if before != "" {
			out = append(out, ContentPart{Text: before, Type: "thinking"})
		}
		after = stripThinkTags(after)
		if after != "" {
			out = append(out, ContentPart{Text: after, Type: "text"})
		}
	}
	if !thinkingDone {
		// Return 'out' instead of 'parts' because text parts might have been cleaned via stripThinkTags
		return out, false
	}
	return out, true
}

// stripThinkTags removes any remaining <think> or </think> tags from text.
func stripThinkTags(s string) string {
	s = thinkClosePattern.ReplaceAllString(s, "")
	s = thinkOpenPattern.ReplaceAllString(s, "")
	return s
}

func isStatusPath(path string) bool {
	return path == "response/status" || path == "status"
}

func extractContentRecursive(items []any, defaultType string) ([]ContentPart, bool) {
	parts := make([]ContentPart, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		itemPath, _ := m["p"].(string)
		itemV, hasV := m["v"]
		if !hasV {
			continue
		}
		if isStatusPath(itemPath) {
			if s, ok := itemV.(string); ok && strings.EqualFold(strings.TrimSpace(s), "FINISHED") {
				return nil, true
			}
			continue
		}
		if shouldSkipPath(itemPath) {
			continue
		}
		if content, ok := m["content"].(string); ok && content != "" {
			typeName, _ := m["type"].(string)
			typeName = strings.ToUpper(typeName)
			switch typeName {
			case "THINK", "THINKING":
				parts = append(parts, ContentPart{Text: content, Type: "thinking"})
			case "RESPONSE":
				parts = append(parts, ContentPart{Text: content, Type: "text"})
			default:
				parts = append(parts, ContentPart{Text: content, Type: defaultType})
			}
			continue
		}
		partType := defaultType
		if strings.Contains(itemPath, "thinking") {
			partType = "thinking"
		} else if strings.Contains(itemPath, "content") || itemPath == "response" || itemPath == "fragments" {
			partType = "text"
		}
		switch v := itemV.(type) {
		case string:
			if isStatusPath(itemPath) {
				continue
			}
			if v != "" && v != "FINISHED" {
				parts = append(parts, ContentPart{Text: v, Type: partType})
			}
		case []any:
			for _, inner := range v {
				switch x := inner.(type) {
				case map[string]any:
					ct, _ := x["content"].(string)
					if ct == "" {
						continue
					}
					typeName, _ := x["type"].(string)
					typeName = strings.ToUpper(typeName)
					switch typeName {
					case "THINK", "THINKING":
						parts = append(parts, ContentPart{Text: ct, Type: "thinking"})
					case "RESPONSE":
						parts = append(parts, ContentPart{Text: ct, Type: "text"})
					default:
						parts = append(parts, ContentPart{Text: ct, Type: partType})
					}
				case string:
					if x != "" {
						parts = append(parts, ContentPart{Text: x, Type: partType})
					}
				}
			}
		}
	}
	return parts, false
}

func IsCitation(text string) bool {
	return bytes.HasPrefix([]byte(strings.TrimSpace(text)), []byte("[citation:"))
}

func hasContentFilterStatus(chunk map[string]any) bool {
	if code, _ := chunk["code"].(string); strings.EqualFold(strings.TrimSpace(code), "content_filter") {
		return true
	}
	return hasContentFilterStatusValue(chunk)
}

func hasContentFilterStatusValue(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if hasContentFilterStatusValue(item) {
				return true
			}
		}
	case map[string]any:
		if p, _ := x["p"].(string); strings.Contains(strings.ToLower(p), "status") {
			if s, _ := x["v"].(string); strings.EqualFold(strings.TrimSpace(s), "content_filter") {
				return true
			}
		}
		if code, _ := x["code"].(string); strings.EqualFold(strings.TrimSpace(code), "content_filter") {
			return true
		}
		for _, vv := range x {
			if hasContentFilterStatusValue(vv) {
				return true
			}
		}
	}
	return false
}
