package prompt

import (
	"strings"
	"testing"
)

func TestNormalizeContentNilReturnsEmpty(t *testing.T) {
	if got := NormalizeContent(nil); got != "" {
		t.Fatalf("expected empty string for nil content, got %q", got)
	}
}

func TestMessagesPrepareNilContentNoNullLiteral(t *testing.T) {
	messages := []map[string]any{
		{"role": "assistant", "content": nil},
		{"role": "user", "content": "ok"},
	}
	got := MessagesPrepare(messages)
	if got == "" {
		t.Fatalf("expected non-empty output")
	}
	if got == "null" {
		t.Fatalf("expected no null literal output, got %q", got)
	}
}

func TestMessagesPrepareUsesTurnSuffixes(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "System rule"},
		{"role": "user", "content": "Question"},
		{"role": "assistant", "content": "Answer"},
	}
	got := MessagesPrepare(messages)
	if !strings.HasPrefix(got, "<｜begin▁of▁sentence｜>") {
		t.Fatalf("expected begin-of-sentence marker, got %q", got)
	}
	if !strings.Contains(got, "<｜System｜>System rule<｜end▁of▁instructions｜>") {
		t.Fatalf("expected system instructions suffix, got %q", got)
	}
	if !strings.Contains(got, "<｜User｜>Question") {
		t.Fatalf("expected user question, got %q", got)
	}
	if !strings.Contains(got, "<｜Assistant｜>Answer<｜end▁of▁sentence｜>") {
		t.Fatalf("expected assistant sentence suffix, got %q", got)
	}
	if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
		t.Fatalf("did not expect think tags in prompt, got %q", got)
	}
}

func TestNormalizeContentArrayFallsBackToContentWhenTextEmpty(t *testing.T) {
	got := NormalizeContent([]any{
		map[string]any{"type": "text", "text": "", "content": "from-content"},
	})
	if got != "from-content" {
		t.Fatalf("expected fallback to content when text is empty, got %q", got)
	}
}

func TestMessagesPrepareWithThinkingAddsContinuityContract(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "Question"}}
	gotThinking := MessagesPrepareWithThinking(messages, true)
	gotPlain := MessagesPrepareWithThinking(messages, false)
	if gotThinking == gotPlain {
		t.Fatalf("expected thinking-enabled prompt to include extra continuity instructions")
	}
	if !strings.HasSuffix(gotThinking, "<｜Assistant｜>") {
		t.Fatalf("expected assistant suffix, got %q", gotThinking)
	}
	if !strings.Contains(gotThinking, "Continue the conversation from the full prior context") {
		t.Fatalf("expected continuity instruction in thinking prompt, got %q", gotThinking)
	}
	if !strings.Contains(gotThinking, "final user-facing answer only in reasoning") {
		t.Fatalf("expected visible-answer instruction in thinking prompt, got %q", gotThinking)
	}
	if strings.Contains(gotPlain, "Continue the conversation from the full prior context") {
		t.Fatalf("did not expect thinking-only instruction in plain prompt, got %q", gotPlain)
	}
}
