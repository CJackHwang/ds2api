package gemini

import (
	"fmt"
	"strings"
)

const maxGeminiRawPromptChars = 1024

func geminiMessagesFromRequest(req map[string]any) []any {
	out := make([]any, 0, 8)
	toolCallCounter := 0
	nextToolCallID := func() string {
		toolCallCounter++
		return fmt.Sprintf("call_gemini_%d", toolCallCounter)
	}
	lastToolCallIDByName := map[string]string{}
	if sys := normalizeGeminiSystemInstruction(req["systemInstruction"]); strings.TrimSpace(sys) != "" {
		out = append(out, map[string]any{
			"role":    "system",
			"content": sys,
		})
	}

	contents, _ := req["contents"].([]any)
	for _, item := range contents {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := mapGeminiRole(content["role"])
		if role == "" {
			role = "user"
		}
		parts, _ := content["parts"].([]any)
		if len(parts) == 0 {
			if text := strings.TrimSpace(asString(content["text"])); text != "" {
				out = append(out, map[string]any{
					"role":    role,
					"content": text,
				})
			}
			continue
		}

		textParts := make([]string, 0, len(parts))
		pendingThinking := ""
		flushText := func() {
			if len(textParts) == 0 {
				return
			}
			msg := map[string]any{
				"role":    role,
				"content": strings.Join(textParts, "\n"),
			}
			if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
				msg["reasoning_content"] = pendingThinking
				pendingThinking = ""
			}
			out = append(out, msg)
			textParts = textParts[:0]
		}

		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if text := strings.TrimSpace(asString(part["text"])); text != "" {
				if role == "assistant" && isGeminiThoughtPart(part) {
					if pendingThinking == "" {
						pendingThinking = text
					} else {
						pendingThinking += "\n" + text
					}
					continue
				}
				textParts = append(textParts, text)
				continue
			}

			if fnCall, ok := part["functionCall"].(map[string]any); ok {
				flushText()
				if name := strings.TrimSpace(asString(fnCall["name"])); name != "" {
					callID := strings.TrimSpace(asString(fnCall["id"]))
					if callID == "" {
						if callID = strings.TrimSpace(asString(fnCall["call_id"])); callID == "" {
							callID = nextToolCallID()
						}
					}
					lastToolCallIDByName[strings.ToLower(name)] = callID
					msg := map[string]any{
						"role": "assistant",
						"tool_calls": []any{
							map[string]any{
								"id":   callID,
								"type": "function",
								"function": map[string]any{
									"name":      name,
									"arguments": stringifyJSON(fnCall["args"]),
								},
							},
						},
					}
					if strings.TrimSpace(pendingThinking) != "" {
						msg["reasoning_content"] = pendingThinking
						pendingThinking = ""
					}
					out = append(out, msg)
				}
				continue
			}

			if fnResp, ok := part["functionResponse"].(map[string]any); ok {
				flushText()
				name := strings.TrimSpace(asString(fnResp["name"]))
				callID := strings.TrimSpace(asString(fnResp["id"]))
				if callID == "" {
					callID = strings.TrimSpace(asString(fnResp["callId"]))
				}
				if callID == "" {
					callID = strings.TrimSpace(asString(fnResp["tool_call_id"]))
				}
				if callID == "" {
					callID = strings.TrimSpace(lastToolCallIDByName[strings.ToLower(name)])
				}
				if callID == "" {
					callID = nextToolCallID()
				}
				content := fnResp["response"]
				if content == nil {
					content = fnResp["output"]
				}
				if content == nil {
					content = ""
				}
				msg := map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      content,
				}
				if name != "" {
					msg["name"] = name
				}
				out = append(out, msg)
				continue
			}

			if raw := strings.TrimSpace(formatGeminiUnknownPartForPrompt(part)); raw != "" && raw != "null" {
				textParts = append(textParts, raw)
			}
		}
		flushText()
		if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
			out = append(out, map[string]any{
				"role":              "assistant",
				"reasoning_content": pendingThinking,
			})
		}
	}
	return out
}

func isGeminiThoughtPart(part map[string]any) bool {
	if part == nil {
		return false
	}
	if v, ok := part["thought"].(bool); ok {
		return v
	}
	if v, ok := part["thoughtSignature"].(string); ok && strings.TrimSpace(v) != "" {
		return true
	}
	return false
}

func normalizeGeminiSystemInstruction(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if parts, ok := v["parts"].([]any); ok {
			texts := make([]string, 0, len(parts))
			for _, item := range parts {
				part, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text := strings.TrimSpace(asString(part["text"])); text != "" {
					texts = append(texts, text)
				}
			}
			return strings.Join(texts, "\n")
		}
		if text := strings.TrimSpace(asString(v["text"])); text != "" {
			return text
		}
	}
	return ""
}

func mapGeminiRole(v any) string {
	switch strings.ToLower(strings.TrimSpace(asString(v))) {
	case "user":
		return "user"
	case "model", "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return ""
	}
}

func formatGeminiUnknownPartForPrompt(part map[string]any) string {
	safe := sanitizeGeminiPartForPrompt(part)
	raw := strings.TrimSpace(stringifyJSON(safe))
	if raw == "" {
		return ""
	}
	if len(raw) > maxGeminiRawPromptChars {
		return raw[:maxGeminiRawPromptChars] + "...(truncated)"
	}
	return raw
}

func sanitizeGeminiPartForPrompt(part map[string]any) map[string]any {
	out := make(map[string]any, len(part))
	for k, v := range part {
		if looksLikeGeminiBinaryField(k) {
			out[k] = "[omitted_binary_payload]"
			continue
		}
		switch x := v.(type) {
		case map[string]any:
			out[k] = sanitizeGeminiPartForPrompt(x)
		case []any:
			out[k] = sanitizeGeminiArrayForPrompt(x)
		case string:
			out[k] = sanitizeGeminiStringForPrompt(k, x)
		default:
			out[k] = v
		}
	}
	return out
}

func sanitizeGeminiArrayForPrompt(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch x := item.(type) {
		case map[string]any:
			out = append(out, sanitizeGeminiPartForPrompt(x))
		case []any:
			out = append(out, sanitizeGeminiArrayForPrompt(x))
		default:
			out = append(out, x)
		}
	}
	return out
}

func sanitizeGeminiStringForPrompt(key, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if looksLikeGeminiBinaryField(key) || looksLikeGeminiBase64(trimmed) {
		return "[omitted_binary_payload]"
	}
	if len(trimmed) > maxGeminiRawPromptChars {
		return trimmed[:maxGeminiRawPromptChars] + "...(truncated)"
	}
	return trimmed
}

func looksLikeGeminiBinaryField(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "data" || n == "bytes" || n == "inlinedata" || n == "inline_data" || n == "base64"
}

func looksLikeGeminiBase64(v string) bool {
	if len(v) < 512 {
		return false
	}
	compact := strings.TrimRight(v, "=")
	if compact == "" {
		return false
	}
	for _, ch := range compact {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '+' || ch == '/' || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}
