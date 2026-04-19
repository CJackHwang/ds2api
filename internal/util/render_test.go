package util

import "testing"

func TestBuildOpenAIResponseObjectWithText(t *testing.T) {
	out := BuildOpenAIResponseObject(
		"resp_1",
		"gpt-4o",
		"prompt",
		"reasoning",
		"text",
		nil,
	)
	if out["object"] != "response" {
		t.Fatalf("unexpected object: %#v", out["object"])
	}
	output, _ := out["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("expected output entries")
	}
	first, _ := output[0].(map[string]any)
	if first["type"] != "message" {
		t.Fatalf("expected first output type message, got %#v", first["type"])
	}
}
