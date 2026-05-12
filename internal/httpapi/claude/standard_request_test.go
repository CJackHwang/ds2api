package claude

import (
	"strings"
	"testing"

	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func TestNormalizeClaudeRequest(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	req := map[string]any{
		"model": "claude-opus-4-6",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"stream": true,
		"tools": []any{
			map[string]any{"name": "search", "description": "Search"},
		},
	}
	norm, err := normalizeClaudeRequest(store, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if norm.Standard.ResolvedModel == "" {
		t.Fatalf("expected resolved model")
	}
	if !norm.Standard.Stream {
		t.Fatalf("expected stream=true")
	}
	if len(norm.Standard.ToolNames) == 0 {
		t.Fatalf("expected tool names")
	}
	if norm.Standard.ToolsRaw == nil {
		t.Fatalf("expected ToolsRaw preserved for downstream normalization")
	}
	if norm.Standard.FinalPrompt == "" {
		t.Fatalf("expected non-empty final prompt")
	}
}

func TestNormalizeClaudeRequestSupportsCamelCaseInputSchemaPromptInjection(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	req := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"tools": []any{
			map[string]any{
				"name":        "todowrite",
				"description": "Write todos",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"todos": map[string]any{"type": "array"}}},
			},
		},
	}
	norm, err := normalizeClaudeRequest(store, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if !containsStr(norm.Standard.FinalPrompt, `"type":"array"`) {
		t.Fatalf("expected inputSchema to be injected into prompt, got=%q", norm.Standard.FinalPrompt)
	}
}

func TestNormalizeClaudeRequestInjectsToolsIntoExistingSystemMessage(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	req := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{"role": "system", "content": "baseline rule"},
			map[string]any{"role": "user", "content": "hello"},
		},
		"tools": []any{
			map[string]any{"name": "search", "description": "Search"},
		},
	}

	norm, err := normalizeClaudeRequest(store, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	if !containsStr(norm.Standard.FinalPrompt, "You have access to these tools") {
		t.Fatalf("expected tool prompt injected into final prompt, got=%q", norm.Standard.FinalPrompt)
	}
	if !containsStr(norm.Standard.FinalPrompt, "baseline rule") {
		t.Fatalf("expected existing system message preserved, got=%q", norm.Standard.FinalPrompt)
	}
}

func TestNormalizeClaudeRequestInjectsToolsIntoTopLevelSystem(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	req := map[string]any{
		"model":  "claude-sonnet-4-5",
		"system": "top-level system",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"tools": []any{
			map[string]any{"name": "search", "description": "Search"},
		},
	}

	norm, err := normalizeClaudeRequest(store, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	if !containsStr(norm.Standard.FinalPrompt, "top-level system") {
		t.Fatalf("expected top-level system preserved, got=%q", norm.Standard.FinalPrompt)
	}
	if !containsStr(norm.Standard.FinalPrompt, "You have access to these tools") {
		t.Fatalf("expected tool prompt injected, got=%q", norm.Standard.FinalPrompt)
	}
}

func TestNormalizeClaudeRequestRendersStructuredHistoryOnce(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	req := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "thinking", "thinking": "check branch first"},
					map[string]any{
						"type":  "tool_use",
						"id":    "call_branch",
						"name":  "Bash",
						"input": map[string]any{"command": "git branch --show-current"},
					},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "call_branch",
						"content":     "fix/context-claude-history-dedupe",
					},
				},
			},
			map[string]any{"role": "user", "content": "continue"},
		},
		"tools": []any{
			map[string]any{
				"name":        "Bash",
				"description": "Run shell commands",
				"input_schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"command": map[string]any{"type": "string"}},
				},
			},
		},
	}

	norm, err := normalizeClaudeRequest(store, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	history := promptcompat.BuildOpenAICurrentInputContextTranscript(norm.Standard.Messages)
	if got := strings.Count(history, "[reasoning_content]"); got != 1 {
		t.Fatalf("expected one reasoning block in history transcript, got %d in %q", got, history)
	}
	if got := strings.Count(history, "<|DSML|tool_calls>"); got != 1 {
		t.Fatalf("expected one tool call block in history transcript, got %d in %q", got, history)
	}
	if got := strings.Count(norm.Standard.FinalPrompt, "[reasoning_content]\ncheck branch first\n[/reasoning_content]"); got != 1 {
		t.Fatalf("expected one reasoning block in final prompt, got %d in %q", got, norm.Standard.FinalPrompt)
	}
	if got := strings.Count(norm.Standard.FinalPrompt, "git branch --show-current"); got != 1 {
		t.Fatalf("expected one command argument in final prompt, got %d in %q", got, norm.Standard.FinalPrompt)
	}
	if got := strings.Count(norm.Standard.FinalPrompt, `<|DSML|parameter name="command"><![CDATA[git branch --show-current]]></|DSML|parameter>`); got != 1 {
		t.Fatalf("expected one concrete command parameter in final prompt, got %d in %q", got, norm.Standard.FinalPrompt)
	}
}
