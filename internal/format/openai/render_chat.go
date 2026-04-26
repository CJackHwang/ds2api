package openai

import (
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/toolcall"
	"strings"
	"time"
)

func BuildChatCompletion(completionID, model, finalPrompt, finalThinking, finalText string, toolNames []string) map[string]any {
	detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, toolNames)
	// Fallback: if text has no tool calls, check thinking content.
	// DeepSeek may emit <tool_calls> XML inside thinking fragments.
	if len(detected.Calls) == 0 && strings.Contains(finalThinking, "<tool_calls") {
		detected = toolcall.ParseStandaloneToolCallsDetailed(finalThinking, toolNames)
		if len(detected.Calls) > 0 {
			finalThinking = cleanToolCallXML(finalThinking)
		}
	}
	finishReason := "stop"
	messageObj := map[string]any{"role": "assistant", "content": finalText}
	if strings.TrimSpace(finalThinking) != "" {
		messageObj["reasoning_content"] = finalThinking
	}
	if len(detected.Calls) > 0 {
		finishReason = "tool_calls"
		messageObj["tool_calls"] = toolcall.FormatOpenAIToolCalls(detected.Calls)
		messageObj["content"] = nil
	}

	return map[string]any{
		"id":      completionID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "message": messageObj, "finish_reason": finishReason}},
		"usage":   BuildChatUsage(finalPrompt, finalThinking, finalText),
	}
}

func BuildChatStreamDeltaChoice(index int, delta map[string]any) map[string]any {
	return map[string]any{
		"delta": delta,
		"index": index,
	}
}

func BuildChatStreamFinishChoice(index int, finishReason string) map[string]any {
	return map[string]any{
		"delta":         map[string]any{},
		"index":         index,
		"finish_reason": finishReason,
	}
}

func BuildChatStreamChunk(completionID string, created int64, model string, choices []map[string]any, usage map[string]any) map[string]any {
	out := map[string]any{
		"id":      completionID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": choices,
	}
	if len(usage) > 0 {
		out["usage"] = usage
	}
	return out
}
