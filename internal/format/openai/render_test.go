package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/toolcall"
	"ds2api/internal/util"
)

func TestBuildResponseObjectKeepsFencedToolPayloadAsText(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		"```json\n{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"golang\"}}]}\n```",
		[]string{"search"},
		nil,
	)

	outputText, _ := obj["output_text"].(string)
	if !strings.Contains(outputText, "\"tool_calls\"") {
		t.Fatalf("expected output_text to preserve fenced tool payload, got %q", outputText)
	}
	output, _ := obj["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected one message output item, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected message output type, got %#v", first["type"])
	}
}

// Backward-compatible alias for historical test name used in CI logs.
func TestBuildResponseObjectPromotesFencedToolPayloadToFunctionCall(t *testing.T) {
	TestBuildResponseObjectKeepsFencedToolPayloadAsText(t)
}

func TestBuildResponseObjectReasoningOnlyFallsBackToOutputText(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"internal thinking content",
		"",
		nil,
		nil,
	)

	outputText, _ := obj["output_text"].(string)
	if outputText == "" {
		t.Fatalf("expected output_text fallback from reasoning when final text is empty")
	}

	output, _ := obj["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected one output item, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected output type message, got %#v", first["type"])
	}
	content, _ := first["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("expected reasoning content, got %#v", first["content"])
	}
	block0, _ := content[0].(map[string]any)
	if block0["type"] != "reasoning" {
		t.Fatalf("expected first content block reasoning, got %#v", block0["type"])
	}
}

func TestBuildResponseObjectPromotesToolCallFromThinkingWhenTextEmpty(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		`<tool_calls><invoke name="search"><parameter name="q">from-thinking</parameter></invoke></tool_calls>`,
		"",
		[]string{"search"},
		nil,
	)

	output, _ := obj["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("expected reasoning message plus function_call output, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected reasoning message output first, got %#v", first["type"])
	}
	content, _ := first["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected reasoning content, got %#v", first["content"])
	}
	block0, _ := content[0].(map[string]any)
	if block0["type"] != "reasoning" {
		t.Fatalf("expected reasoning block, got %#v", block0["type"])
	}
	second, _ := output[1].(map[string]any)
	if second["type"] != "function_call" {
		t.Fatalf("expected function_call output, got %#v", second["type"])
	}
}

func TestBuildChatCompletionWithToolCallsCoercesSchemaDeclaredStringArguments(t *testing.T) {
	toolsRaw := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "Write",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
						"taskId":  map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	obj := BuildChatCompletionWithToolCalls(
		"chat_test",
		"gpt-4o",
		"prompt",
		"",
		"",
		[]toolcall.ParsedToolCall{{
			Name: "Write",
			Input: map[string]any{
				"content": map[string]any{"message": "hi"},
				"taskId":  1,
			},
		}},
		toolsRaw,
	)
	choices, _ := obj["choices"].([]map[string]any)
	message, _ := choices[0]["message"].(map[string]any)
	toolCalls, _ := message["tool_calls"].([]map[string]any)
	fn, _ := toolCalls[0]["function"].(map[string]any)
	args := map[string]any{}
	if err := json.Unmarshal([]byte(fn["arguments"].(string)), &args); err != nil {
		t.Fatalf("decode arguments failed: %v", err)
	}
	if args["content"] != `{"message":"hi"}` {
		t.Fatalf("expected content stringified by schema, got %#v", args["content"])
	}
	if args["taskId"] != "1" {
		t.Fatalf("expected taskId stringified by schema, got %#v", args["taskId"])
	}
}

func TestBuildResponseObjectWithToolCallsCoercesSchemaDeclaredStringArguments(t *testing.T) {
	toolsRaw := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "Write",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	obj := BuildResponseObjectWithToolCalls(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		"",
		[]toolcall.ParsedToolCall{{
			Name:  "Write",
			Input: map[string]any{"content": []any{"a", 1}},
		}},
		toolsRaw,
	)
	output, _ := obj["output"].([]any)
	first, _ := output[0].(map[string]any)
	args := map[string]any{}
	if err := json.Unmarshal([]byte(first["arguments"].(string)), &args); err != nil {
		t.Fatalf("decode response arguments failed: %v", err)
	}
	if args["content"] != `["a",1]` {
		t.Fatalf("expected response content stringified by schema, got %#v", args["content"])
	}
}

func TestBuildChatUsageForModelUsesConservativePromptCount(t *testing.T) {
	prompt := strings.Repeat("上下文token ", 40)
	usage := BuildChatUsageForModel("deepseek-v4-flash", prompt, "", "ok", 0)
	promptTokens, _ := usage["prompt_tokens"].(int)
	if promptTokens <= util.EstimateTokens(prompt) {
		t.Fatalf("expected conservative prompt token count > rough estimate, got=%d estimate=%d", promptTokens, util.EstimateTokens(prompt))
	}
	totalTokens, _ := usage["total_tokens"].(int)
	completionTokens, _ := usage["completion_tokens"].(int)
	if totalTokens != promptTokens+completionTokens {
		t.Fatalf("expected total tokens to add up, got usage=%#v", usage)
	}
}
