package openai

import (
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/toolcall"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

func BuildResponseObject(responseID, model, finalPrompt, finalThinking, finalText string, toolNames []string) map[string]any {
	// Strict mode: only standalone, structured tool-call payloads are treated
	// as executable tool calls.
	detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, toolNames)
	// Fallback: if text has no tool calls, check thinking content.
	if len(detected.Calls) == 0 && strings.Contains(finalThinking, "<tool_calls") {
		detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
		if len(detected.Calls) > 0 {
			finalThinking = shared.cleanToolCallXML(finalThinking)
		}
	}
	exposedOutputText := finalText
	output := make([]any, 0, 2)
	if len(detected.Calls) > 0 {
		exposedOutputText = ""
		output = append(output, toResponsesFunctionCallItems(detected.Calls)...)
	} else {
		content := make([]any, 0, 2)
		if finalThinking != "" {
			content = append([]any{map[string]any{
				"type": "reasoning",
				"text": finalThinking,
			}}, content...)
		}
		if strings.TrimSpace(finalText) != "" {
			content = append(content, map[string]any{
				"type": "output_text",
				"text": finalText,
			})
		}
		if strings.TrimSpace(finalText) == "" && strings.TrimSpace(finalThinking) != "" {
			exposedOutputText = finalThinking
		}
		output = append(output, map[string]any{
			"type":    "message",
			"id":      "msg_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"role":    "assistant",
			"content": content,
		})
	}
	return BuildResponseObjectFromItems(
		responseID,
		model,
		finalPrompt,
		finalThinking,
		finalText,
		output,
		exposedOutputText,
	)
}

func BuildResponseObjectFromItems(responseID, model, finalPrompt, finalThinking, finalText string, output []any, outputText string) map[string]any {
	if output == nil {
		output = []any{}
	}
	return map[string]any{
		"id":          responseID,
		"type":        "response",
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"status":      "completed",
		"model":       model,
		"output":      output,
		"output_text": outputText,
		"usage":       BuildResponsesUsage(finalPrompt, finalThinking, finalText),
	}
}

func toResponsesFunctionCallItems(toolCalls []toolcall.ParsedToolCall) []any {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]any, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		argsBytes, _ := json.Marshal(tc.Input)
		args := normalizeJSONString(string(argsBytes))
		out = append(out, map[string]any{
			"id":        "fc_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"type":      "function_call",
			"call_id":   "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"name":      tc.Name,
			"arguments": args,
			"status":    "completed",
		})
	}
	return out
}

func normalizeJSONString(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "{}"
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
}
