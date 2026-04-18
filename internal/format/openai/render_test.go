package openai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildResponseObjectToolCallsFollowChatShape(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		`{"tool_calls":[{"name":"search","input":{"q":"golang"}}]}`,
		[]string{"search"},
	)

	outputText, _ := obj["output_text"].(string)
	if outputText != "" {
		t.Fatalf("expected output_text to be hidden for tool calls, got %q", outputText)
	}

	output, _ := obj["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected function_call output only, got %#v", obj["output"])
	}

	first, _ := output[0].(map[string]any)
	if first["type"] != "function_call" {
		t.Fatalf("expected first output item type function_call, got %#v", first["type"])
	}
	if first["call_id"] == "" {
		t.Fatalf("expected function_call item to have call_id, got %#v", first)
	}
	if first["name"] != "search" {
		t.Fatalf("unexpected function name: %#v", first["name"])
	}
	argsRaw, _ := first["arguments"].(string)
	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		t.Fatalf("arguments should be valid json string, got=%q err=%v", argsRaw, err)
	}
	if args["q"] != "golang" {
		t.Fatalf("unexpected arguments: %#v", args)
	}
}

func TestBuildResponseObjectPromotesMixedProseToolPayloadToFunctionCall(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		`示例格式：{"tool_calls":[{"name":"search","input":{"q":"golang"}}]}，但这条是普通回答。`,
		[]string{"search"},
	)

	outputText, _ := obj["output_text"].(string)
	if outputText != "" {
		t.Fatalf("expected output_text hidden for mixed prose tool payload, got %q", outputText)
	}
	output, _ := obj["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected one function_call output item, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "function_call" {
		t.Fatalf("expected function_call output type, got %#v", first["type"])
	}
}

func TestBuildResponseObjectKeepsFencedToolPayloadAsText(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		"```json\n{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"golang\"}}]}\n```",
		[]string{"search"},
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

func TestBuildResponseObjectIgnoresToolCallFromThinkingChannel(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		`{"tool_calls":[{"name":"search","input":{"q":"from-thinking"}}]}`,
		"",
		[]string{"search"},
	)

	output, _ := obj["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected one message output item, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected output message, got %#v", first["type"])
	}
}

func TestBuildResponseObjectToolCallWithEmptyNameFallsBackToMessage(t *testing.T) {
	obj := BuildResponseObject(
		"resp_test",
		"gpt-4o",
		"prompt",
		"",
		`{"tool_calls":[{"name":"","input":{"q":"x"}}]}`,
		[]string{"search"},
	)

	output, _ := obj["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("expected non-empty output fallback, got %#v", obj["output"])
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected message fallback, got %#v", first["type"])
	}
	id, _ := first["id"].(string)
	if strings.TrimSpace(id) == "" {
		t.Fatalf("expected message id in fallback output, got %#v", first["id"])
	}
}
