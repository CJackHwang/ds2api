package openai

import (
	"testing"

	"ds2api/internal/config"
	"ds2api/internal/util"
)

func newEmptyStoreForNormalizeTest(t *testing.T) *config.Store {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	return config.LoadStore()
}

func TestNormalizeOpenAIChatRequest(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model": "gpt-5-codex",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"temperature": 0.3,
		"stream":      true,
	}
	n, err := normalizeOpenAIChatRequest(store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if n.ResolvedModel != "deepseek-reasoner" {
		t.Fatalf("unexpected resolved model: %s", n.ResolvedModel)
	}
	if !n.Stream {
		t.Fatalf("expected stream=true")
	}
	if _, ok := n.PassThrough["temperature"]; !ok {
		t.Fatalf("expected temperature passthrough")
	}
	if n.FinalPrompt == "" {
		t.Fatalf("expected non-empty final prompt")
	}
}

func TestNormalizeOpenAIResponsesRequestInput(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model":        "gpt-4o",
		"input":        "ping",
		"instructions": "system",
	}
	n, err := normalizeOpenAIResponsesRequest(store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if n.ResolvedModel != "deepseek-chat" {
		t.Fatalf("unexpected resolved model: %s", n.ResolvedModel)
	}
	if len(n.Messages) != 2 {
		t.Fatalf("expected 2 normalized messages, got %d", len(n.Messages))
	}
}

func TestNormalizeOpenAIResponsesRequestToolChoiceRequired(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model": "gpt-4o",
		"input": "ping",
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "search",
					"parameters": map[string]any{
						"type": "object",
					},
				},
			},
		},
		"tool_choice": "required",
	}
	n, err := normalizeOpenAIResponsesRequest(store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if n.ToolChoice.Mode != util.ToolChoiceRequired {
		t.Fatalf("expected tool choice mode required, got %q", n.ToolChoice.Mode)
	}
	if len(n.ToolNames) != 1 || n.ToolNames[0] != "search" {
		t.Fatalf("unexpected tool names: %#v", n.ToolNames)
	}
}

func TestNormalizeOpenAIResponsesRequestToolChoiceForcedFunction(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model": "gpt-4o",
		"input": "ping",
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "search",
				},
			},
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "read_file",
				},
			},
		},
		"tool_choice": map[string]any{
			"type": "function",
			"name": "read_file",
		},
	}
	n, err := normalizeOpenAIResponsesRequest(store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if n.ToolChoice.Mode != util.ToolChoiceForced {
		t.Fatalf("expected tool choice mode forced, got %q", n.ToolChoice.Mode)
	}
	if n.ToolChoice.ForcedName != "read_file" {
		t.Fatalf("expected forced tool name read_file, got %q", n.ToolChoice.ForcedName)
	}
	if len(n.ToolNames) != 1 || n.ToolNames[0] != "read_file" {
		t.Fatalf("expected filtered tool names [read_file], got %#v", n.ToolNames)
	}
}

func TestNormalizeOpenAIResponsesRequestToolChoiceForcedUndeclaredFails(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model": "gpt-4o",
		"input": "ping",
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "search",
				},
			},
		},
		"tool_choice": map[string]any{
			"type": "function",
			"name": "read_file",
		},
	}
	if _, err := normalizeOpenAIResponsesRequest(store, req, ""); err == nil {
		t.Fatalf("expected forced undeclared tool to fail")
	}
}

func TestNormalizeOpenAIResponsesRequestToolChoiceNoneKeepsToolDetectionEnabled(t *testing.T) {
	store := newEmptyStoreForNormalizeTest(t)
	req := map[string]any{
		"model": "gpt-4o",
		"input": "ping",
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "search",
				},
			},
		},
		"tool_choice": "none",
	}
	n, err := normalizeOpenAIResponsesRequest(store, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if n.ToolChoice.Mode != util.ToolChoiceNone {
		t.Fatalf("expected tool choice mode none, got %q", n.ToolChoice.Mode)
	}
	if len(n.ToolNames) == 0 {
		t.Fatalf("expected tool detection sentinel when tool_choice=none, got %#v", n.ToolNames)
	}
}
