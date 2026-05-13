package util

import (
	"strings"
	"testing"

	"ds2api/internal/config"
)

func TestMessagesPrepareBasic(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "Hello"}}
	got := MessagesPrepare(messages)
	if got == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.HasPrefix(got, "<|begin▁of▁sentence|><|System|>") {
		t.Fatalf("expected output integrity guard at the start, got %q", got)
	}
	if !strings.Contains(got, "Hello") || !strings.HasSuffix(got, "<|Assistant|>") {
		t.Fatalf("unexpected prompt: %q", got)
	}
}

func TestMessagesPrepareRoles(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "You are helper"},
		{"role": "user", "content": "Hi"},
		{"role": "assistant", "content": "Hello"},
		{"role": "tool", "content": "Search results"},
		{"role": "user", "content": "How are you"},
	}
	got := MessagesPrepare(messages)
	if !contains(got, "Output integrity guard") {
		t.Fatalf("expected output integrity guard in %q", got)
	}
	if !contains(got, "You are helper") || !contains(got, "<|User|>Hi") {
		t.Fatalf("expected system/user content in %q", got)
	}
	if !contains(got, "<|begin▁of▁sentence|>") {
		t.Fatalf("expected begin marker in %q", got)
	}
	if !contains(got, "<|User|>Hi<|Assistant|>Hello<|end▁of▁sentence|>") {
		t.Fatalf("expected user/assistant separation in %q", got)
	}
	if !contains(got, "<|Assistant|>Hello<|end▁of▁sentence|><|Tool|>Search results<|end▁of▁toolresults|>") {
		t.Fatalf("expected assistant/tool separation in %q", got)
	}
	if !contains(got, "<|Tool|>Search results<|end▁of▁toolresults|><|User|>How are you") {
		t.Fatalf("expected tool/user separation in %q", got)
	}
	if !contains(got, "<|Assistant|>") {
		t.Fatalf("expected assistant marker in %q", got)
	}
	if !contains(got, "<|System|>") {
		t.Fatalf("expected system marker in %q", got)
	}
	if !contains(got, "<|User|>") {
		t.Fatalf("expected user marker in %q", got)
	}
	if !contains(got, "<|Tool|>") {
		t.Fatalf("expected tool marker in %q", got)
	}
}

func TestMessagesPrepareObjectContent(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": map[string]any{"temp": 18, "ok": true}},
	}
	got := MessagesPrepare(messages)
	if !contains(got, `"temp":18`) || !contains(got, `"ok":true`) {
		t.Fatalf("expected serialized object content, got %q", got)
	}
}

func TestMessagesPrepareArrayTextVariants(t *testing.T) {
	messages := []map[string]any{
		{
			"role": "user",
			"content": []any{
				map[string]any{"type": "output_text", "text": "line1"},
				map[string]any{"type": "input_text", "text": "line2"},
				map[string]any{"type": "image_url", "image_url": "https://example.com/a.png"},
			},
		},
	}
	got := MessagesPrepare(messages)
	if !contains(got, "line1\nline2") {
		t.Fatalf("unexpected content from text variants: %q", got)
	}
	if !strings.Contains(got, "Output integrity guard") {
		t.Fatalf("expected output integrity guard in %q", got)
	}
}

func TestConvertClaudeToDeepSeek(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":    "claude-opus-4-6",
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
		"system":   "You are helpful",
		"stream":   true,
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["model"] == "" {
		t.Fatal("expected mapped model")
	}
	msgs, ok := out["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatal("expected messages")
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Fatalf("expected first message system, got %#v", first)
	}
}

func TestConvertClaudeToDeepSeekUsesGlobalAliasResolution(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":    "claude-3-5-sonnet-latest",
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["model"] != "deepseek-v4-flash" {
		t.Fatalf("expected global alias resolution, got model=%q", out["model"])
	}
}

func TestConvertClaudeToDeepSeekUsesNoThinkingAliasResolution(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":    "claude-sonnet-4-6-nothinking",
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["model"] != "deepseek-v4-flash-nothinking" {
		t.Fatalf("expected noThinking alias resolution, got model=%q", out["model"])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
