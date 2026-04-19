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

	finalPrompt, toolNames := buildOpenAIFinalPrompt(messages, tools, "", false)
	if len(toolNames) != 1 || toolNames[0] != "get_weather" {
		t.Fatalf("unexpected tool names: %#v", toolNames)
	}
	if !strings.Contains(finalPrompt, `"condition":"sunny"`) {
		t.Fatalf("handler finalPrompt should preserve tool output content: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "<tool_calls>") {
		t.Fatalf("handler finalPrompt should preserve assistant tool history: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "<tool_name>get_weather</tool_name>") {
		t.Fatalf("handler finalPrompt should include tool name history: %q", finalPrompt)
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

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
	if !strings.Contains(finalPrompt, "Remember: The ONLY valid way to use tools is the <tool_calls> XML block at the end of your response.") {
		t.Fatalf("vercel prepare finalPrompt missing final tool-call anchor instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "TOOL CALL FORMAT") {
		t.Fatalf("vercel prepare finalPrompt missing xml format instruction: %q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "Do NOT wrap XML in markdown fences") {
		t.Fatalf("vercel prepare finalPrompt missing no-fence xml instruction: %q", finalPrompt)
	}
	if strings.Contains(finalPrompt, "```json") {
		t.Fatalf("vercel prepare finalPrompt should not require fenced tool calls: %q", finalPrompt)
	}
}

func TestBuildOpenAIFinalPromptWithThinkingAddsContinuationContract(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "继续回答上一个问题"},
	}

	finalPrompt, _ := buildOpenAIFinalPrompt(messages, nil, "", true)
	if !strings.Contains(finalPrompt, "Continue the conversation from the full prior context") {
		t.Fatalf("expected continuation contract in thinking prompt, got=%q", finalPrompt)
	}
	if !strings.Contains(finalPrompt, "final user-facing answer only in reasoning") {
		t.Fatalf("expected visible-answer contract in thinking prompt, got=%q", finalPrompt)
	}
}
