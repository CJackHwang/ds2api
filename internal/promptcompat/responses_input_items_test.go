package promptcompat

import (
	"strings"
	"testing"
)

func TestNormalizeResponsesInputItemPreservesAssistantReasoningContent(t *testing.T) {
	item := map[string]any{
		"role":              "assistant",
		"reasoning_content": "hidden reasoning",
		"tool_calls": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":      "search",
					"arguments": `{"q":"docs"}`,
				},
			},
		},
	}

	got := normalizeResponsesInputItem(item)
	if got == nil {
		t.Fatal("expected assistant item to be preserved")
	}
	if got["role"] != "assistant" {
		t.Fatalf("unexpected role: %#v", got["role"])
	}
	if got["reasoning_content"] != "hidden reasoning" {
		t.Fatalf("expected reasoning_content preserved, got %#v", got["reasoning_content"])
	}
}

func TestNormalizeResponsesInputItemAssistantMessageWithReasoningBlocks(t *testing.T) {
	item := map[string]any{
		"type": "message",
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "reasoning", "text": "internal chain"},
			map[string]any{"type": "output_text", "text": "visible answer"},
		},
	}

	got := normalizeResponsesInputItem(item)
	if got == nil {
		t.Fatal("expected assistant message item to be preserved")
	}
	content, _ := got["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected content blocks preserved, got %#v", got["content"])
	}
}

func TestNormalizeResponsesInputArrayMergesReasoningMessageIntoFunctionCallHistory(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "reasoning", "text": "need fresh docs before answering"},
			},
		},
		map[string]any{
			"type":      "function_call",
			"call_id":   "call_search",
			"name":      "search_web",
			"arguments": `{"query":"docs"}`,
		},
	}

	got := NormalizeResponsesInputAsMessages(input)
	if len(got) != 1 {
		t.Fatalf("expected reasoning and function_call merged into one assistant message, got %#v", got)
	}
	msg, _ := got[0].(map[string]any)
	if msg["role"] != "assistant" {
		t.Fatalf("expected assistant message, got %#v", msg)
	}
	if msg["reasoning_content"] != "need fresh docs before answering" {
		t.Fatalf("expected reasoning_content on tool-call message, got %#v", msg)
	}
	toolCalls, _ := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", msg["tool_calls"])
	}
	history := BuildOpenAIHistoryTranscript(got)
	if !strings.Contains(history, "[reasoning_content]\nneed fresh docs before answering\n[/reasoning_content]") {
		t.Fatalf("expected reasoning in history transcript, got %q", history)
	}
	if !strings.Contains(history, `<|DSML|invoke name="search_web">`) {
		t.Fatalf("expected tool call in history transcript, got %q", history)
	}
}
