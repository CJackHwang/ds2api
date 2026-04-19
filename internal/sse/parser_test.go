package sse

import "testing"

func TestParseDeepSeekSSELine(t *testing.T) {
	chunk, done, ok := ParseDeepSeekSSELine([]byte(`data: {"v":"你好"}`))
	if !ok || done {
		t.Fatalf("expected parsed chunk")
	}
	if chunk["v"] != "你好" {
		t.Fatalf("unexpected chunk: %#v", chunk)
	}
}

func TestParseDeepSeekSSELineDone(t *testing.T) {
	_, done, ok := ParseDeepSeekSSELine([]byte(`data: [DONE]`))
	if !ok || !done {
		t.Fatalf("expected done signal")
	}
}

func TestParseSSEChunkForContentSimple(t *testing.T) {
	parts, finished, _ := ParseSSEChunkForContent(map[string]any{"v": "hello"}, false, "text")
	if finished {
		t.Fatal("expected unfinished")
	}
	if len(parts) != 1 || parts[0].Text != "hello" || parts[0].Type != "text" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestParseSSEChunkForContentThinking(t *testing.T) {
	parts, finished, _ := ParseSSEChunkForContent(map[string]any{"p": "response/thinking_content", "v": "think"}, true, "thinking")
	if finished {
		t.Fatal("expected unfinished")
	}
	if len(parts) != 1 || parts[0].Type != "thinking" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestIsCitation(t *testing.T) {
	if !IsCitation("[citation:1] abc") {
		t.Fatal("expected citation true")
	}
	if IsCitation("normal text") {
		t.Fatal("expected citation false")
	}
}

func TestParseSSEChunkForContentFragmentsAppendSwitchToResponse(t *testing.T) {
	chunk := map[string]any{
		"p": "response/fragments",
		"o": "APPEND",
		"v": []any{
			map[string]any{
				"type":    "RESPONSE",
				"content": "你好",
			},
		},
	}
	parts, finished, nextType := ParseSSEChunkForContent(chunk, true, "thinking")
	if finished {
		t.Fatal("expected unfinished")
	}
	if nextType != "text" {
		t.Fatalf("expected next type text, got %q", nextType)
	}
	if len(parts) != 1 || parts[0].Type != "text" || parts[0].Text != "你好" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestParseSSEChunkForContentAfterAppendUsesUpdatedType(t *testing.T) {
	chunk := map[string]any{
		"p": "response/fragments/-1/content",
		"v": "！",
	}
	parts, finished, nextType := ParseSSEChunkForContent(chunk, true, "text")
	if finished {
		t.Fatal("expected unfinished")
	}
	if nextType != "text" {
		t.Fatalf("expected next type text, got %q", nextType)
	}
	if len(parts) != 1 || parts[0].Type != "text" || parts[0].Text != "！" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestParseSSEChunkForContentAutoTransitionsThinkClose(t *testing.T) {
	chunk := map[string]any{
		"p": "response/thinking_content",
		"v": "deep thoughts</think>actual answer",
	}
	parts, _, _ := ParseSSEChunkForContent(chunk, true, "thinking")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts from split, got %d: %#v", len(parts), parts)
	}
	if parts[0].Type != "thinking" || parts[0].Text != "deep thoughts" {
		t.Fatalf("first part should be thinking: %#v", parts[0])
	}
	if parts[1].Type != "text" || parts[1].Text != "actual answer" {
		t.Fatalf("second part should be text: %#v", parts[1])
	}
}

func TestParseSSEChunkForContentStripsLeakedThinkTags(t *testing.T) {
	chunk := map[string]any{
		"p": "response/thinking_content",
		"v": "<think>more thoughts</think>  answer",
	}
	parts, _, _ := ParseSSEChunkForContent(chunk, true, "thinking")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %#v", len(parts), parts)
	}
	if parts[0].Type != "thinking" || parts[0].Text != "<think>more thoughts" {
		// note: the open tag is before the split, so it remains in the thinking part.
		// that's fine, the output sanitization handles the final string.
		t.Fatalf("first part mismatch: %#v", parts[0])
	}
	if parts[1].Type != "text" || parts[1].Text != "  answer" {
		t.Fatalf("second part mismatch: %#v", parts[1])
	}
}

func TestParseSSEChunkForContentAutoTransitionsState(t *testing.T) {
	chunk1 := map[string]any{
		"p": "response/thinking_content",
		"v": "end of thought</think>start of text",
	}
	parts1, _, nextType1 := ParseSSEChunkForContent(chunk1, true, "thinking")
	if len(parts1) != 2 || parts1[1].Type != "text" {
		t.Fatalf("expected split parts, got %#v", parts1)
	}
	if nextType1 != "text" {
		t.Fatalf("expected nextType to transition to text, got %q", nextType1)
	}

	chunk2 := map[string]any{
		"p": "response/thinking_content",
		"v": "more actual text sent to thinking path",
	}
	parts2, _, nextType2 := ParseSSEChunkForContent(chunk2, true, nextType1)
	if len(parts2) != 1 || parts2[0].Type != "text" {
		t.Fatalf("expected subsequent parts to be text, got %#v", parts2)
	}
	if nextType2 != "text" {
		t.Fatalf("expected nextType2 to remain text, got %q", nextType2)
	}
}

func TestParseSSEChunkForContentStripsLeakedThinkTagsFromText(t *testing.T) {
	chunk := map[string]any{
		"p": "response/content", // This makes the part type "text"
		"v": "normal text <think>leaked</think> end",
	}
	parts, _, _ := ParseSSEChunkForContent(chunk, true, "text")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d: %#v", len(parts), parts)
	}
	if parts[0].Type != "text" || parts[0].Text != "normal text leaked end" {
		t.Fatalf("expected leaked think tag to be stripped, got %#v", parts[0])
	}
}
