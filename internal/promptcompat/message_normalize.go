package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
	"ds2api/internal/toolcall"
)

const assistantReasoningLabel = "reasoning_content"

func NormalizeOpenAIMessagesForPrompt(raw []any, traceID string) []map[string]any {
	_ = traceID
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		switch role {
		case "assistant":
			content := buildAssistantContentForPrompt(msg)
			if content == "" {
				continue
			}
			normalized := map[string]any{
				"role":    "assistant",
				"content": content,
			}
			if prompt.FormatToolCallsForPrompt(msg["tool_calls"]) != "" {
				normalized["tool_calls"] = msg["tool_calls"]
			}
			out = append(out, normalized)
		case "tool", "function":
			content := buildToolContentForPrompt(msg)
			out = append(out, map[string]any{
				"role":    "tool",
				"content": content,
			})
		case "user", "system", "developer":
			out = append(out, map[string]any{
				"role":    normalizeOpenAIRoleForPrompt(role),
				"content": NormalizeOpenAIContentForPrompt(msg["content"]),
			})
		default:
			content := NormalizeOpenAIContentForPrompt(msg["content"])
			if content == "" {
				continue
			}
			if role == "" {
				role = "user"
			}
			out = append(out, map[string]any{
				"role":    normalizeOpenAIRoleForPrompt(role),
				"content": content,
			})
		}
	}
	return out
}

func buildAssistantContentForPrompt(msg map[string]any) string {
	content := strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	reasoning := strings.TrimSpace(normalizeOpenAIReasoningContentForPrompt(msg["reasoning_content"]))
	if reasoning == "" {
		reasoning = strings.TrimSpace(extractOpenAIReasoningContentFromMessage(msg["content"]))
	} else {
		content = stripPromptLabeledBlocks(content, assistantReasoningLabel)
	}
	toolHistory := prompt.FormatToolCallsForPrompt(msg["tool_calls"])
	if toolHistory == "" {
		content = normalizeAssistantToolMarkupContentForPrompt(content)
	} else {
		content = stripAssistantToolMarkupBlocks(content)
	}
	parts := make([]string, 0, 3)
	if reasoning != "" {
		parts = append(parts, formatPromptLabeledBlock(assistantReasoningLabel, reasoning))
	}
	if content != "" {
		parts = append(parts, content)
	}
	if toolHistory != "" {
		parts = append(parts, toolHistory)
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts, "\n\n")
	}
}

func normalizeAssistantToolMarkupContentForPrompt(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || !isStandaloneAssistantToolMarkupBlock(trimmed) {
		return content
	}
	parsed := toolcall.ParseStandaloneToolCallsDetailed(trimmed, nil)
	if len(parsed.Calls) == 0 {
		return content
	}
	raw := make([]any, 0, len(parsed.Calls))
	for _, call := range parsed.Calls {
		raw = append(raw, map[string]any{
			"name":  call.Name,
			"input": call.Input,
		})
	}
	if formatted := prompt.FormatToolCallsForPrompt(raw); formatted != "" {
		return formatted
	}
	return content
}

func stripPromptLabeledBlocks(content, label string) string {
	label = strings.TrimSpace(label)
	if strings.TrimSpace(content) == "" || label == "" {
		return content
	}
	openTag := "[" + label + "]\n"
	closeTag := "\n[/" + label + "]"
	var b strings.Builder
	pos := 0
	for {
		start := strings.Index(content[pos:], openTag)
		if start == -1 {
			b.WriteString(content[pos:])
			break
		}
		start += pos
		afterOpen := start + len(openTag)
		end := strings.Index(content[afterOpen:], closeTag)
		if end == -1 {
			b.WriteString(content[pos:])
			break
		}
		end += afterOpen
		b.WriteString(content[pos:start])
		pos = end + len(closeTag)
		if strings.HasPrefix(content[pos:], "\n\n") {
			pos += 2
		}
	}
	return strings.TrimSpace(b.String())
}

func stripAssistantToolMarkupBlocks(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	var b strings.Builder
	pos := 0
	for pos < len(content) {
		tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(content, pos)
		if !ok {
			b.WriteString(content[pos:])
			break
		}
		if tag.Start > pos {
			b.WriteString(content[pos:tag.Start])
		}
		if tag.Closing || tag.Name != "tool_calls" {
			b.WriteString(content[tag.Start : tag.End+1])
			pos = tag.End + 1
			continue
		}
		closeTag, ok := toolcall.FindMatchingToolMarkupClose(content, tag)
		if !ok {
			b.WriteString(content[tag.Start : tag.End+1])
			pos = tag.End + 1
			continue
		}
		pos = closeTag.End + 1
		if strings.HasPrefix(content[pos:], "\n\n") {
			pos += 2
		}
	}
	return strings.TrimSpace(b.String())
}

func isStandaloneAssistantToolMarkupBlock(trimmed string) bool {
	tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(trimmed, 0)
	if !ok || tag.Start != 0 || tag.Closing || tag.Name != "tool_calls" {
		return false
	}
	closeTag, ok := toolcall.FindMatchingToolMarkupClose(trimmed, tag)
	if !ok {
		return false
	}
	return strings.TrimSpace(trimmed[closeTag.End+1:]) == ""
}

func normalizeOpenAIReasoningContentForPrompt(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		return strings.Join(extractOpenAIReasoningPartsFromItems(x), "\n")
	case map[string]any:
		return extractOpenAIReasoningTextFromItem(x)
	default:
		return ""
	}
}

func extractOpenAIReasoningContentFromMessage(v any) string {
	switch x := v.(type) {
	case []any:
		return strings.Join(extractOpenAIReasoningPartsFromItems(x), "\n")
	case map[string]any:
		return extractOpenAIReasoningTextFromItem(x)
	default:
		return ""
	}
}

func extractOpenAIReasoningPartsFromItems(items []any) []string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text := extractOpenAIReasoningTextFromItemMap(item); text != "" {
			parts = append(parts, text)
		}
	}
	return parts
}

func extractOpenAIReasoningTextFromItemMap(item any) string {
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	return extractOpenAIReasoningTextFromItem(m)
}

func extractOpenAIReasoningTextFromItem(m map[string]any) string {
	if m == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(asString(m["type"]))) {
	case "reasoning", "thinking":
		for _, key := range []string{"text", "thinking", "content"} {
			if text := strings.TrimSpace(asString(m[key])); text != "" {
				return text
			}
		}
	}
	return ""
}

func formatPromptLabeledBlock(label, text string) string {
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" {
		return text
	}
	return "[" + label + "]\n" + text + "\n[/" + label + "]"
}

func buildToolContentForPrompt(msg map[string]any) string {
	content := NormalizeOpenAIContentForPrompt(msg["content"])
	if strings.TrimSpace(content) == "" {
		return "null"
	}
	return content
}

func NormalizeOpenAIContentForPrompt(v any) string {
	return prompt.NormalizeContent(v)
}

func normalizeOpenAIRoleForPrompt(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "developer" {
		return "system"
	}
	return role
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
