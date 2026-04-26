package promptcompat

import (
	"strings"
	"testing"

	"ds2api/internal/util"
)

func TestNormalizeOpenAIMessagesForPrompt_AssistantToolCallsAndToolResult(t *testing.T) {
	raw := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "查北京天气"},
		map[string]any{
			"role":    "assistant",
			"content": nil,
			"tool_calls": []any{
				map[string]any{
					"id":   "call_1",
					"type": "function",
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
			"content":      "{\"temp\":18}",
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 4 {
		t.Fatalf("expected 4 normalized messages with assistant tool history preserved, got %d", len(normalized))
	}
	assistantContent, _ := normalized[2]["content"].(string)
	if !strings.Contains(assistantContent, "<|DSML|tool_calls>") {
		t.Fatalf("assistant tool history should be preserved in DSML form, got %q", assistantContent)
	}
	if !strings.Contains(assistantContent, `<|DSML|invoke name="get_weather">`) {
		t.Fatalf("expected tool name in preserved history, got %q", assistantContent)
	}
	if !strings.Contains(normalized[3]["content"].(string), `"temp":18`) {
		t.Fatalf("tool result should be transparently forwarded, got %#v", normalized[3]["content"])
	}

	prompt := util.MessagesPrepare(normalized)
	if !strings.Contains(prompt, "<|DSML|tool_calls>") {
		t.Fatalf("expected preserved assistant tool history in prompt: %q", prompt)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_ToolObjectContentPreserved(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_2",
			"name":         "get_weather",
			"content": map[string]any{
				"temp":      18,
				"condition": "sunny",
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	got, _ := normalized[0]["content"].(string)
	if !strings.Contains(got, `"temp":18`) || !strings.Contains(got, `"condition":"sunny"`) {
		t.Fatalf("expected serialized object in tool content, got %q", got)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_ToolArrayBlocksJoined(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_3",
			"name":         "read_file",
			"content": []any{
				map[string]any{"type": "input_text", "text": "line-1"},
				map[string]any{"type": "output_text", "text": "line-2"},
				map[string]any{"type": "image_url", "image_url": "https://example.com/a.png"},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	got, _ := normalized[0]["content"].(string)
	if !strings.Contains(got, `line-1`) || !strings.Contains(got, `line-2`) {
		t.Fatalf("expected tool content blocks preserved, got %q", got)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_FunctionRoleCompatible(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":         "function",
			"tool_call_id": "call_4",
			"name":         "legacy_tool",
			"content": map[string]any{
				"ok": true,
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected one normalized message, got %d", len(normalized))
	}
	if normalized[0]["role"] != "tool" {
		t.Fatalf("expected function role normalized as tool, got %#v", normalized[0]["role"])
	}
	got, _ := normalized[0]["content"].(string)
	if !strings.Contains(got, `"ok":true`) || strings.Contains(got, `"name":"legacy_tool"`) {
		t.Fatalf("unexpected normalized function-role content: %q", got)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_EmptyToolContentPreservedAsNull(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_5",
			"name":         "noop_tool",
			"content":      "",
		},
		map[string]any{
			"role":    "assistant",
			"content": "done",
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 2 {
		t.Fatalf("expected tool completion turn to be preserved, got %#v", normalized)
	}
	if normalized[0]["role"] != "tool" {
		t.Fatalf("expected tool role preserved, got %#v", normalized[0]["role"])
	}
	got, _ := normalized[0]["content"].(string)
	if got != "null" {
		t.Fatalf("expected empty tool content normalized as null string, got %q", got)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_AssistantMultipleToolCallsRemainSeparated(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_search",
					"type": "function",
					"function": map[string]any{
						"name":      "search_web",
						"arguments": `{"query":"latest ai news"}`,
					},
				},
				map[string]any{
					"id":   "call_eval",
					"type": "function",
					"function": map[string]any{
						"name":      "eval_javascript",
						"arguments": `{"code":"1+1"}`,
					},
				},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected assistant tool_call-only message preserved, got %#v", normalized)
	}
	content, _ := normalized[0]["content"].(string)
	if strings.Count(content, "<|DSML|invoke name=") != 2 {
		t.Fatalf("expected two preserved tool call blocks, got %q", content)
	}
	if !strings.Contains(content, `<|DSML|invoke name="search_web">`) || !strings.Contains(content, `<|DSML|invoke name="eval_javascript">`) {
		t.Fatalf("expected both tool names in preserved history, got %q", content)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_PreservesConcatenatedToolArguments(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id": "call_1",
					"function": map[string]any{
						"name":      "search_web",
						"arguments": `{}{"query":"测试工具调用"}`,
					},
				},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected assistant tool_call-only content preserved, got %#v", normalized)
	}
	content, _ := normalized[0]["content"].(string)
	if !strings.Contains(content, `{}{"query":"测试工具调用"}`) {
		t.Fatalf("expected concatenated tool arguments preserved, got %q", content)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_AssistantToolCallsMissingNameAreDropped(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_missing_name",
					"type": "function",
					"function": map[string]any{
						"arguments": `{"path":"README.MD"}`,
					},
				},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 0 {
		t.Fatalf("expected assistant tool_calls without text to be dropped when name is missing, got %#v", normalized)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_AssistantNilContentDoesNotInjectNullLiteral(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":    "assistant",
			"content": nil,
			"tool_calls": []any{
				map[string]any{
					"id": "call_screenshot",
					"function": map[string]any{
						"name":      "send_file_to_user",
						"arguments": `{"file_path":"/tmp/a.png"}`,
					},
				},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected nil-content assistant tool_call-only message preserved, got %#v", normalized)
	}
	content, _ := normalized[0]["content"].(string)
	if strings.Contains(content, "null") {
		t.Fatalf("expected no null literal injection, got %q", content)
	}
	if !strings.Contains(content, "<|DSML|tool_calls>") {
		t.Fatalf("expected assistant tool history in normalized content, got %q", content)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_DeveloperRoleMapsToSystem(t *testing.T) {
	raw := []any{
		map[string]any{"role": "developer", "content": "必须先走工具调用"},
		map[string]any{"role": "user", "content": "你好"},
	}
	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 2 {
		t.Fatalf("expected 2 normalized messages, got %d", len(normalized))
	}
	if normalized[0]["role"] != "system" {
		t.Fatalf("expected developer role converted to system, got %#v", normalized[0]["role"])
	}
}

func TestNormalizeOpenAIMessagesForPrompt_AssistantArrayContentFallbackWhenTextEmpty(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "", "content": "工具说明文本"},
			},
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected one normalized message, got %d", len(normalized))
	}
	content, _ := normalized[0]["content"].(string)
	if content != "工具说明文本" {
		t.Fatalf("expected content fallback text preserved, got %q", content)
	}
}

func TestNormalizeOpenAIMessagesForPrompt_AssistantReasoningContentPreserved(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":              "assistant",
			"content":           "visible answer",
			"reasoning_content": "internal reasoning",
		},
	}

	normalized := NormalizeOpenAIMessagesForPrompt(raw, "")
	if len(normalized) != 1 {
		t.Fatalf("expected one normalized assistant message, got %#v", normalized)
	}
	content, _ := normalized[0]["content"].(string)
	if !strings.Contains(content, "[reasoning_content]") {
		t.Fatalf("expected labeled reasoning block in assistant content, got %q", content)
	}
	if !strings.Contains(content, "internal reasoning") {
		t.Fatalf("expected reasoning text in assistant content, got %q", content)
	}
	if !strings.Contains(content, "visible answer") {
		t.Fatalf("expected visible answer in assistant content, got %q", content)
	}
	if reasoningIdx := strings.Index(content, "[reasoning_content]"); reasoningIdx < 0 || reasoningIdx > strings.Index(content, "visible answer") {
		t.Fatalf("expected reasoning block before visible answer, got %q", content)
	}
}
