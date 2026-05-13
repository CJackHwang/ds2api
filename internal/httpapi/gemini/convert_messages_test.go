package gemini

import (
	"ds2api/internal/promptcompat"
	"strings"
	"testing"
)

func TestGeminiMessagesFromRequestPreservesFunctionRoundtrip(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"id":   "call_g1",
							"name": "search_web",
							"args": map[string]any{"query": "ai"},
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"id":       "call_g1",
							"name":     "search_web",
							"response": "ok",
						},
					},
				},
			},
		},
	}

	got := geminiMessagesFromRequest(req)
	if len(got) != 2 {
		t.Fatalf("expected two normalized messages, got %#v", got)
	}
	assistant, _ := got[0].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("expected assistant first, got %#v", assistant)
	}
	tc, _ := assistant["tool_calls"].([]any)
	if len(tc) != 1 {
		t.Fatalf("expected one tool call, got %#v", assistant["tool_calls"])
	}
	toolMsg, _ := got[1].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_g1" {
		t.Fatalf("expected tool message with call id, got %#v", toolMsg)
	}
}

func TestGeminiMessagesFromRequestPreservesThoughtOnFunctionCallHistory(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{"text": "need current state before answering", "thought": true},
					map[string]any{
						"functionCall": map[string]any{
							"id":   "call_g1",
							"name": "search_web",
							"args": map[string]any{"query": "ai"},
						},
					},
				},
			},
		},
	}

	got := geminiMessagesFromRequest(req)
	if len(got) != 1 {
		t.Fatalf("expected one normalized message, got %#v", got)
	}
	assistant, _ := got[0].(map[string]any)
	if assistant["reasoning_content"] != "need current state before answering" {
		t.Fatalf("expected thought preserved as reasoning_content, got %#v", assistant)
	}
	tc, _ := assistant["tool_calls"].([]any)
	if len(tc) != 1 {
		t.Fatalf("expected one tool call, got %#v", assistant["tool_calls"])
	}
	prompt, _ := promptcompat.BuildOpenAIPromptForAdapter(got, nil, "", true)
	if !strings.Contains(prompt, "[reasoning_content]\nneed current state before answering\n[/reasoning_content]") {
		t.Fatalf("expected thought in prompt history, got %q", prompt)
	}
	if !strings.Contains(prompt, `<|DSML|invoke name="search_web">`) {
		t.Fatalf("expected tool call in prompt history, got %q", prompt)
	}
}

func TestGeminiMessagesFromRequestPreservesUnknownPartAsRawJSONText(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "hello"},
					map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": strings.Repeat("A", 2048)}},
				},
			},
		},
	}

	got := geminiMessagesFromRequest(req)
	if len(got) != 1 {
		t.Fatalf("expected one normalized message, got %#v", got)
	}
	msg, _ := got[0].(map[string]any)
	content, _ := msg["content"].(string)
	if !strings.Contains(content, "hello") || !strings.Contains(content, "inlineData") {
		t.Fatalf("expected unknown part preserved as raw json text, got %q", content)
	}
	if !strings.Contains(content, "[omitted_binary_payload]") {
		t.Fatalf("expected inlineData payload to be redacted, got %q", content)
	}
	if strings.Contains(content, strings.Repeat("A", 100)) {
		t.Fatalf("expected raw base64 payload not to be embedded, got %q", content)
	}
}

func TestGeminiMessagesFromRequestBackfillsFunctionResponseCallIDByName(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "search_web",
							"args": map[string]any{"query": "docs"},
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name":     "search_web",
							"response": map[string]any{"ok": true},
						},
					},
				},
			},
		},
	}

	got := geminiMessagesFromRequest(req)
	if len(got) != 2 {
		t.Fatalf("expected two normalized messages, got %#v", got)
	}
	assistant, _ := got[0].(map[string]any)
	tc, _ := assistant["tool_calls"].([]any)
	call, _ := tc[0].(map[string]any)
	callID, _ := call["id"].(string)
	if !strings.HasPrefix(callID, "call_gemini_") {
		t.Fatalf("expected generated call id prefix, got %#v", call)
	}
	toolMsg, _ := got[1].(map[string]any)
	if toolMsg["tool_call_id"] != callID {
		t.Fatalf("expected tool response to inherit generated call id, tool=%#v call=%#v", toolMsg, call)
	}
}
