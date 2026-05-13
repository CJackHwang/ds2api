package claude

import (
	"ds2api/internal/toolcall"
	"encoding/json"
	"fmt"
	"strings"

	"ds2api/internal/prompt"
)

func normalizeClaudeMessages(messages []any) []any {
	out := make([]any, 0, len(messages))
	state := &claudeToolCallState{
		nameByID:       map[string]string{},
		lastIDByName:   map[string]string{},
		callIDSequence: 0,
	}
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", msg["role"])))
		switch content := msg["content"].(type) {
		case []any:
			textParts := make([]string, 0, len(content))
			pendingThinking := ""
			flushText := func() {
				if len(textParts) == 0 {
					return
				}
				message := map[string]any{
					"role":    role,
					"content": strings.Join(textParts, "\n"),
				}
				if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
					message["reasoning_content"] = pendingThinking
					message["content"] = prependClaudeReasoningForPrompt(pendingThinking, safeStringValue(message["content"]))
					pendingThinking = ""
				}
				out = append(out, message)
				textParts = textParts[:0]
			}
			for _, block := range content {
				b, ok := block.(map[string]any)
				if !ok {
					continue
				}
				typeStr := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", b["type"])))
				switch typeStr {
				case "text":
					if t, ok := b["text"].(string); ok {
						textParts = append(textParts, t)
					}
				case "thinking":
					if role == "assistant" {
						if thinking := extractClaudeThinkingBlockText(b); thinking != "" {
							if pendingThinking == "" {
								pendingThinking = thinking
							} else {
								pendingThinking += "\n" + thinking
							}
						}
						continue
					}
					if raw := strings.TrimSpace(formatClaudeUnknownBlockForPrompt(b)); raw != "" {
						textParts = append(textParts, raw)
					}
				case "tool_use":
					if role == "assistant" {
						flushText()
						if toolMsg := normalizeClaudeToolUseToAssistant(b, state); toolMsg != nil {
							if strings.TrimSpace(pendingThinking) != "" {
								toolMsg["reasoning_content"] = pendingThinking
								toolMsg["content"] = prependClaudeReasoningForPrompt(pendingThinking, safeStringValue(toolMsg["content"]))
								pendingThinking = ""
							}
							out = append(out, toolMsg)
						}
						continue
					}
					if raw := strings.TrimSpace(formatClaudeUnknownBlockForPrompt(b)); raw != "" {
						textParts = append(textParts, raw)
					}
				case "tool_result":
					flushText()
					if toolMsg := normalizeClaudeToolResultToToolMessage(b, state); toolMsg != nil {
						out = append(out, toolMsg)
					}
				default:
					if raw := strings.TrimSpace(formatClaudeUnknownBlockForPrompt(b)); raw != "" {
						textParts = append(textParts, raw)
					}
				}
			}
			flushText()
			if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
				out = append(out, map[string]any{
					"role":              "assistant",
					"reasoning_content": pendingThinking,
					"content":           formatClaudeReasoningForPrompt(pendingThinking),
				})
			}
		default:
			copied := cloneMap(msg)
			out = append(out, copied)
		}
	}
	return out
}

func prependClaudeReasoningForPrompt(reasoning, content string) string {
	reasoning = strings.TrimSpace(reasoning)
	content = strings.TrimSpace(content)
	if reasoning == "" {
		return content
	}
	block := formatClaudeReasoningForPrompt(reasoning)
	if content == "" {
		return block
	}
	return block + "\n\n" + content
}

func formatClaudeReasoningForPrompt(reasoning string) string {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return ""
	}
	return "[reasoning_content]\n" + reasoning + "\n[/reasoning_content]"
}

func extractClaudeThinkingBlockText(block map[string]any) string {
	if block == nil {
		return ""
	}
	for _, key := range []string{"thinking", "text", "content"} {
		if text := strings.TrimSpace(safeStringValue(block[key])); text != "" {
			return text
		}
	}
	return ""
}

func buildClaudeToolPrompt(tools []any) string {
	toolSchemas := make([]string, 0, len(tools))
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, desc, schemaObj := extractClaudeToolMeta(m)
		if name == "" {
			continue
		}
		names = append(names, name)
		schema, _ := json.Marshal(schemaObj)
		toolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, schema))
	}
	if len(toolSchemas) == 0 {
		return ""
	}
	return "You have access to these tools:\n\n" +
		strings.Join(toolSchemas, "\n\n") + "\n\n" +
		toolcall.BuildToolCallInstructions(names)
}

//nolint:unused // retained for compatibility with pending Claude tool-result prompt flow.
func formatClaudeToolResultForPrompt(block map[string]any) string {
	if block == nil {
		return ""
	}
	payload := map[string]any{
		"type":    "tool_result",
		"content": block["content"],
	}
	if toolCallID := strings.TrimSpace(fmt.Sprintf("%v", block["tool_use_id"])); toolCallID != "" {
		payload["tool_call_id"] = toolCallID
	} else if toolCallID := strings.TrimSpace(fmt.Sprintf("%v", block["tool_call_id"])); toolCallID != "" {
		payload["tool_call_id"] = toolCallID
	}
	if name := strings.TrimSpace(fmt.Sprintf("%v", block["name"])); name != "" {
		payload["name"] = name
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", payload))
	}
	return string(b)
}

func normalizeClaudeToolUseToAssistant(block map[string]any, state *claudeToolCallState) map[string]any {
	if block == nil {
		return nil
	}
	name := strings.TrimSpace(fmt.Sprintf("%v", block["name"]))
	if name == "" {
		return nil
	}
	callID := safeStringValue(block["id"])
	if callID == "" {
		callID = safeStringValue(block["tool_use_id"])
	}
	if callID == "" {
		callID = state.nextID()
	}
	state.nameByID[callID] = name
	state.lastIDByName[strings.ToLower(name)] = callID
	arguments := block["input"]
	if arguments == nil {
		arguments = map[string]any{}
	}
	argsJSON, err := json.Marshal(arguments)
	if err != nil || len(argsJSON) == 0 {
		argsJSON = []byte("{}")
	}
	toolCalls := []any{
		map[string]any{
			"id":   callID,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": string(argsJSON),
			},
		},
	}
	return map[string]any{
		"role":       "assistant",
		"content":    prompt.FormatToolCallsForPrompt(toolCalls),
		"tool_calls": toolCalls,
	}
}

func normalizeClaudeToolResultToToolMessage(block map[string]any, state *claudeToolCallState) map[string]any {
	if block == nil {
		return nil
	}
	name := safeStringValue(block["name"])
	toolCallID := safeStringValue(block["tool_use_id"])
	if toolCallID == "" {
		toolCallID = safeStringValue(block["tool_call_id"])
	}
	if toolCallID == "" {
		if name != "" {
			toolCallID = strings.TrimSpace(state.lastIDByName[strings.ToLower(name)])
		}
	}
	if toolCallID == "" {
		toolCallID = state.nextID()
	}
	out := map[string]any{
		"role":         "tool",
		"tool_call_id": toolCallID,
		"content":      normalizeClaudeToolResultContent(block["content"]),
	}
	if name != "" {
		out["name"] = name
		state.nameByID[toolCallID] = name
		state.lastIDByName[strings.ToLower(name)] = toolCallID
	} else if inferred := strings.TrimSpace(state.nameByID[toolCallID]); inferred != "" {
		out["name"] = inferred
	}
	return out
}

func normalizeClaudeToolResultContent(content any) any {
	if text, ok := content.(string); ok {
		return text
	}
	payload := map[string]any{
		"type":    "tool_result",
		"content": content,
	}
	b, err := json.Marshal(sanitizeClaudeBlockForPrompt(payload))
	if err != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", content))
	}
	return string(b)
}

func formatClaudeBlockRaw(block map[string]any) string {
	if block == nil {
		return ""
	}
	b, err := json.Marshal(block)
	if err != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", block))
	}
	return string(b)
}
