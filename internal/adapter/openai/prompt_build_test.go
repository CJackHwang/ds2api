package openai

import (
	"strings"
	"testing"
)

func TestBuildOpenAIFinalPrompt_HandlerPathIncludesToolRoundtripSemantics(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "查北京天气"},
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id": "call_1",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": "{\"city\":\"beijing\"}",
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_1",
			"name":         "get_weather",
			"content":      map[string]any{"temp": 18, "condition": "sunny"},
		},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	finalPrompt, toolNames := buildOpenAIFinalPrompt(messages, tools, "")
	if len(toolNames) != 1 || toolNames[0] != "get_weather" {
		t.Fatalf("unexpected tool names: %#v", toolNames)
	}
	if !strings.Contains(finalPrompt, "tool_call_id: call_1") ||
		!strings.Contains(finalPrompt, "function.name: get_weather") ||
		!strings.Contains(finalPrompt, "[TOOL_RESULT_HISTORY]") ||
		!strings.Contains(finalPrompt, `"condition":"sunny"`) {
		t.Fatalf("handler finalPrompt missing tool roundtrip semantics: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPrompt_VercelPreparePathKeepsFinalAnswerInstruction(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "请调用工具"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "search",
				"description": "search docs",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "")
	if !strings.Contains(finalPrompt, "After receiving a tool result, you MUST use it to produce the final answer.") {
		t.Fatalf("vercel prepare finalPrompt missing final-answer instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "Only call another tool when the previous result is missing required data or returned an error.") {
		t.Fatalf("vercel prepare finalPrompt missing retry guard instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "Never output [TOOL_CALL_HISTORY] or [TOOL_RESULT_HISTORY] tags in your response") {
		t.Fatalf("vercel prepare finalPrompt missing history-tag output guard instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "[TOOL_RESULT_HISTORY]") {
		t.Fatalf("vercel prepare finalPrompt missing history marker instruction: %q", finalPrompt)
	}
}
