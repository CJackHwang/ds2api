package claude

import (
	"strings"
	"testing"
)

// ─── normalizeClaudeMessages ─────────────────────────────────────────

func TestNormalizeClaudeMessagesSimpleString(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": "Hello"},
	}
	got := normalizeClaudeMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	m := got[0].(map[string]any)
	if m["content"] != "Hello" {
		t.Fatalf("expected 'Hello', got %v", m["content"])
	}
}

func TestNormalizeClaudeMessagesArrayContent(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "line1"},
				map[string]any{"type": "text", "text": "line2"},
			},
		},
	}
	got := normalizeClaudeMessages(msgs)
	m := got[0].(map[string]any)
	if m["content"] != "line1\nline2" {
		t.Fatalf("expected joined text, got %q", m["content"])
	}
}

func TestNormalizeClaudeMessagesToolResult(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "tool_result", "content": "tool output"},
			},
		},
	}
	got := normalizeClaudeMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected one normalized message, got %d", len(got))
	}
	m := got[0].(map[string]any)
	if m["role"] != "tool" {
		t.Fatalf("expected tool role preserved, got %#v", m["role"])
	}
	content, _ := m["content"].(string)
	if content != "tool output" {
		t.Fatalf("expected raw tool output content preserved, got %q", content)
	}
}

func TestNormalizeClaudeMessagesToolUseToAssistantToolCalls(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"id":    "call_1",
					"name":  "search_web",
					"input": map[string]any{"query": "latest"},
				},
			},
		},
	}

	got := normalizeClaudeMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected one normalized tool-call message, got %d", len(got))
	}
	m := got[0].(map[string]any)
	if m["role"] != "assistant" {
		t.Fatalf("expected assistant role, got %#v", m["role"])
	}
	tc, _ := m["tool_calls"].([]any)
	if len(tc) != 1 {
		t.Fatalf("expected one tool call, got %#v", m["tool_calls"])
	}
	call, _ := tc[0].(map[string]any)
	if call["id"] != "call_1" {
		t.Fatalf("expected call id preserved, got %#v", call)
	}
	content, _ := m["content"].(string)
	if !containsStr(content, "<tool_calls>") || !containsStr(content, "<tool_name>search_web</tool_name>") {
		t.Fatalf("expected assistant content to include XML tool call history, got %q", content)
	}
	if !containsStr(content, "<parameters>\n      <query><![CDATA[latest]]></query>\n    </parameters>") {
		t.Fatalf("expected assistant content to include serialized parameters, got %q", content)
	}
}

func TestNormalizeClaudeMessagesDoesNotPromoteUserToolUse(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"id":    "call_unsafe",
					"name":  "dangerous_tool",
					"input": map[string]any{"value": "x"},
				},
			},
		},
	}

	got := normalizeClaudeMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected one normalized message, got %d", len(got))
	}
	m := got[0].(map[string]any)
	if m["role"] != "user" {
		t.Fatalf("expected user role preserved, got %#v", m["role"])
	}
	if _, ok := m["tool_calls"]; ok {
		t.Fatalf("expected no tool_calls promotion for user message, got %#v", m["tool_calls"])
	}
	content, _ := m["content"].(string)
	if !containsStr(content, `"type":"tool_use"`) || !containsStr(content, "dangerous_tool") {
		t.Fatalf("expected raw tool_use block preserved in user content, got %q", content)
	}
}

func TestNormalizeClaudeMessagesSkipsNonMap(t *testing.T) {
	msgs := []any{"not a map", 42}
	got := normalizeClaudeMessages(msgs)
	if len(got) != 0 {
		t.Fatalf("expected 0 messages for non-map items, got %d", len(got))
	}
}

func TestNormalizeClaudeMessagesEmpty(t *testing.T) {
	got := normalizeClaudeMessages(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestNormalizeClaudeMessagesPreservesRole(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "assistant", "content": "response"},
	}
	got := normalizeClaudeMessages(msgs)
	m := got[0].(map[string]any)
	if m["role"] != "assistant" {
		t.Fatalf("expected 'assistant', got %q", m["role"])
	}
}

func TestNormalizeClaudeMessagesMixedContentBlocks(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello"},
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "data": strings.Repeat("A", 2048)}},
				map[string]any{"type": "text", "text": "World"},
			},
		},
	}
	got := normalizeClaudeMessages(msgs)
	m := got[0].(map[string]any)
	content, _ := m["content"].(string)
	if !containsStr(content, "Hello") || !containsStr(content, "World") || !containsStr(content, `"type":"image"`) {
		t.Fatalf("expected text plus non-text block marker preserved, got %q", content)
	}
	if !containsStr(content, omittedBinaryMarker) {
		t.Fatalf("expected binary payload omitted marker, got %q", content)
	}
	if containsStr(content, strings.Repeat("A", 100)) {
		t.Fatalf("expected raw base64 payload not to be included, got %q", content)
	}
}

func TestNormalizeClaudeMessagesToolResultNonTextPayloadStringified(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "call_image_1",
					"name":        "vision_tool",
					"content": []any{
						map[string]any{"type": "text", "text": "image analysis"},
						map[string]any{
							"type":   "image",
							"source": map[string]any{"type": "base64", "media_type": "image/png", "data": strings.Repeat("B", 2048)},
						},
					},
				},
			},
		},
	}

	got := normalizeClaudeMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected one normalized message, got %d", len(got))
	}
	m := got[0].(map[string]any)
	if m["role"] != "tool" {
		t.Fatalf("expected tool role, got %#v", m["role"])
	}
	content, _ := m["content"].(string)
	if !containsStr(content, `"type":"tool_result"`) || !containsStr(content, `"type":"image"`) {
		t.Fatalf("expected non-text tool_result payload to be JSON stringified, got %q", content)
	}
	if !containsStr(content, omittedBinaryMarker) {
		t.Fatalf("expected binary data to be sanitized with omitted marker, got %q", content)
	}
	if containsStr(content, strings.Repeat("B", 100)) {
		t.Fatalf("expected raw base64 payload not to be included, got %q", content)
	}
}

func TestNormalizeClaudeMessagesBackfillsToolResultCallIDByName(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"name":  "search_web",
					"input": map[string]any{"query": "latest"},
				},
			},
		},
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type":    "tool_result",
					"name":    "search_web",
					"content": "ok",
				},
			},
		},
	}

	got := normalizeClaudeMessages(msgs)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %#v", got)
	}
	assistant, _ := got[0].(map[string]any)
	tc, _ := assistant["tool_calls"].([]any)
	call, _ := tc[0].(map[string]any)
	callID, _ := call["id"].(string)
	if !strings.HasPrefix(callID, "call_claude_") {
		t.Fatalf("expected generated call id, got %#v", call)
	}
	toolMsg, _ := got[1].(map[string]any)
	if toolMsg["tool_call_id"] != callID {
		t.Fatalf("expected tool_result to reuse generated id, got %#v", toolMsg)
	}
}

// ─── buildClaudeToolPrompt ───────────────────────────────────────────

func TestBuildClaudeToolPromptSingleTool(t *testing.T) {
	tools := []any{
		map[string]any{
			"name":        "search",
			"description": "Search the web",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}
	prompt := buildClaudeToolPrompt(tools)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	// Should contain tool name and description
	if !containsStr(prompt, "search") {
		t.Fatalf("expected 'search' in prompt")
	}
	if !containsStr(prompt, "Search the web") {
		t.Fatalf("expected description in prompt")
	}
	if !containsStr(prompt, "<tool_calls>") {
		t.Fatalf("expected XML tool_calls format in prompt")
	}
	if !containsStr(prompt, "TOOL CALL FORMAT") {
		t.Fatalf("expected tool call format header in prompt")
	}
}

func TestBuildClaudeToolPromptMultipleTools(t *testing.T) {
	tools := []any{
		map[string]any{"name": "tool1", "description": "desc1"},
		map[string]any{"name": "tool2", "description": "desc2"},
	}
	prompt := buildClaudeToolPrompt(tools)
	if !containsStr(prompt, "tool1") || !containsStr(prompt, "tool2") {
		t.Fatalf("expected both tools in prompt")
	}
}

func TestBuildClaudeToolPromptSupportsOpenAIStyleFunctionTool(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "search",
				"description": "Search via function tool",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	prompt := buildClaudeToolPrompt(tools)
	if !containsStr(prompt, "Tool: search") {
		t.Fatalf("expected OpenAI-style function tool name in prompt, got: %q", prompt)
	}
	if !containsStr(prompt, "Search via function tool") {
		t.Fatalf("expected OpenAI-style function tool description in prompt, got: %q", prompt)
	}
	if !containsStr(prompt, "\"q\"") {
		t.Fatalf("expected parameters schema serialized in prompt, got: %q", prompt)
	}
}

func TestBuildClaudeToolPromptSkipsNonMap(t *testing.T) {
	tools := []any{"not a map"}
	prompt := buildClaudeToolPrompt(tools)
	// No valid tools → empty prompt
	if prompt != "" {
		t.Fatalf("expected empty prompt for non-map tools, got: %q", prompt)
	}
}

// ─── hasSystemMessage ────────────────────────────────────────────────

func TestHasSystemMessageTrue(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "system", "content": "You are a helper"},
		map[string]any{"role": "user", "content": "Hi"},
	}
	if !hasSystemMessage(msgs) {
		t.Fatal("expected true")
	}
}

func TestHasSystemMessageFalse(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": "Hi"},
		map[string]any{"role": "assistant", "content": "Hello"},
	}
	if hasSystemMessage(msgs) {
		t.Fatal("expected false")
	}
}

func TestHasSystemMessageEmpty(t *testing.T) {
	if hasSystemMessage(nil) {
		t.Fatal("expected false for nil")
	}
}

func TestHasSystemMessageNonMap(t *testing.T) {
	msgs := []any{"not a map"}
	if hasSystemMessage(msgs) {
		t.Fatal("expected false for non-map")
	}
}

// ─── extractClaudeToolNames ──────────────────────────────────────────

func TestExtractClaudeToolNamesSingle(t *testing.T) {
	tools := []any{
		map[string]any{"name": "search"},
	}
	names := extractClaudeToolNames(tools)
	if len(names) != 1 || names[0] != "search" {
		t.Fatalf("expected [search], got %v", names)
	}
}

func TestExtractClaudeToolNamesMultiple(t *testing.T) {
	tools := []any{
		map[string]any{"name": "search"},
		map[string]any{"name": "calculate"},
	}
	names := extractClaudeToolNames(tools)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %v", names)
	}
}

func TestExtractClaudeToolNamesSkipsEmptyName(t *testing.T) {
	tools := []any{
		map[string]any{"name": ""},
		map[string]any{"name": "valid"},
	}
	names := extractClaudeToolNames(tools)
	if len(names) != 1 || names[0] != "valid" {
		t.Fatalf("expected [valid], got %v", names)
	}
}

func TestExtractClaudeToolNamesSkipsNonMap(t *testing.T) {
	tools := []any{"not a map", 42}
	names := extractClaudeToolNames(tools)
	if len(names) != 0 {
		t.Fatalf("expected 0, got %v", names)
	}
}

func TestExtractClaudeToolNamesNil(t *testing.T) {
	names := extractClaudeToolNames(nil)
	if len(names) != 0 {
		t.Fatalf("expected 0, got %v", names)
	}
}

func TestExtractClaudeToolNamesSupportsOpenAIStyleFunctionTool(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "search",
			},
		},
	}
	names := extractClaudeToolNames(tools)
	if len(names) != 1 || names[0] != "search" {
		t.Fatalf("expected [search], got %v", names)
	}
}

// ─── toMessageMaps ───────────────────────────────────────────────────

func TestToMessageMapsNormal(t *testing.T) {
	input := []any{
		map[string]any{"role": "user", "content": "Hello"},
	}
	got := toMessageMaps(input)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestToMessageMapsNonSlice(t *testing.T) {
	got := toMessageMaps("not a slice")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestToMessageMapsSkipsNonMap(t *testing.T) {
	input := []any{"string", map[string]any{"role": "user"}, 42}
	got := toMessageMaps(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 map, got %d", len(got))
	}
}

func TestToMessageMapsNil(t *testing.T) {
	got := toMessageMaps(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// ─── extractMessageContent ──────────────────────────────────────────

func TestExtractMessageContentString(t *testing.T) {
	if got := extractMessageContent("hello"); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestExtractMessageContentArray(t *testing.T) {
	input := []any{"part1", "part2"}
	got := extractMessageContent(input)
	if got != "part1\npart2" {
		t.Fatalf("expected joined, got %q", got)
	}
}

func TestExtractMessageContentOther(t *testing.T) {
	got := extractMessageContent(42)
	if got != "42" {
		t.Fatalf("expected '42', got %q", got)
	}
}

func TestExtractMessageContentNil(t *testing.T) {
	got := extractMessageContent(nil)
	if got != "<nil>" {
		t.Fatalf("expected '<nil>', got %q", got)
	}
}

// ─── cloneMap ────────────────────────────────────────────────────────

func TestCloneMapBasic(t *testing.T) {
	original := map[string]any{"a": 1, "b": "hello"}
	clone := cloneMap(original)
	original["a"] = 999
	if clone["a"] != 1 {
		t.Fatalf("expected 1, got %v", clone["a"])
	}
	if clone["b"] != "hello" {
		t.Fatalf("expected 'hello', got %v", clone["b"])
	}
}

func TestCloneMapEmpty(t *testing.T) {
	clone := cloneMap(map[string]any{})
	if len(clone) != 0 {
		t.Fatalf("expected empty, got %v", clone)
	}
}

func TestCloneMapNested(t *testing.T) {
	// cloneMap is shallow, so nested maps share references
	inner := map[string]any{"key": "value"}
	original := map[string]any{"nested": inner}
	clone := cloneMap(original)
	// Shallow clone means inner is shared
	inner["key"] = "modified"
	cloneNested := clone["nested"].(map[string]any)
	if cloneNested["key"] != "modified" {
		t.Fatal("expected shallow clone to share nested references")
	}
}

// helper
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
