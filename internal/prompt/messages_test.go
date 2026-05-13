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
	if !strings.HasPrefix(got, "<|begin▁of▁sentence|>") {
		t.Fatalf("expected begin-of-sentence marker, got %q", got)
	}
	if !strings.Contains(got, "<|System|>") || !strings.Contains(got, "<|end▁of▁instructions|>") || !strings.Contains(got, "System rule") {
		t.Fatalf("expected system instructions to remain present, got %q", got)
	}
	if !strings.Contains(got, "<|User|>Question") {
		t.Fatalf("expected user question, got %q", got)
	}
	if !strings.Contains(got, "<|Assistant|>Answer<|end▁of▁sentence|>") {
		t.Fatalf("expected assistant sentence suffix, got %q", got)
	}
	if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
		t.Fatalf("did not expect think tags in prompt, got %q", got)
	}
}

func TestMessagesPreparePrependsOutputIntegrityGuard(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "System rule"},
		{"role": "user", "content": "Question"},
	}
	got := MessagesPrepare(messages)
	if !strings.HasPrefix(got, beginSentenceMarker+systemMarker+outputIntegrityGuardPrompt) {
		t.Fatalf("expected output integrity guard to be prepended, got %q", got)
	}
	if !strings.Contains(got, outputIntegrityGuardPrompt+"\n\nSystem rule") {
		t.Fatalf("expected output integrity guard to precede system prompt content, got %q", got)
	}
	if !strings.Contains(got, "<|User|>Question") {
		t.Fatalf("expected user question after guard, got %q", got)
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

func TestMessagesPrepareWithThinkingPreservesPromptShape(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "Question"}}
	gotThinking := MessagesPrepareWithThinking(messages, true)
	gotPlain := MessagesPrepareWithThinking(messages, false)
	if gotThinking != gotPlain {
		t.Fatalf("expected thinking flag not to add extra continuity instructions, got thinking=%q plain=%q", gotThinking, gotPlain)
	}
	if !strings.HasSuffix(gotThinking, "<|Assistant|>") {
		t.Fatalf("expected assistant suffix, got %q", gotThinking)
	}
}
