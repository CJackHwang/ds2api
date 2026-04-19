package toolcall

import (
	"testing"
)

// --- FormatOpenAIStreamToolCalls ---

func TestFormatOpenAIStreamToolCalls(t *testing.T) {
	formatted := FormatOpenAIStreamToolCalls([]ParsedToolCall{
		{Name: "search", Input: map[string]any{"q": "test"}},
	})
	if len(formatted) != 1 {
		t.Fatalf("expected 1, got %d", len(formatted))
	}
	fn, _ := formatted[0]["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Fatalf("unexpected function name: %#v", fn)
	}
	if formatted[0]["index"] != 0 {
		t.Fatalf("expected index 0, got %v", formatted[0]["index"])
	}
}

// --- ParseToolCalls edge cases ---

func TestParseToolCallsEmptyText(t *testing.T) {
	calls := ParseToolCalls("", []string{"search"})
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls for empty text, got %d", len(calls))
	}
}
