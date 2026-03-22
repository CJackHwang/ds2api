package util

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/config"
)

// ─── EstimateTokens edge cases ───────────────────────────────────────

func TestEstimateTokensEmpty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("expected 0 for empty string, got %d", got)
	}
}

func TestEstimateTokensShortASCII(t *testing.T) {
	got := EstimateTokens("ab")
	if got != 1 {
		t.Fatalf("expected 1 for 2 ascii chars, got %d", got)
	}
}

func TestEstimateTokensLongASCII(t *testing.T) {
	got := EstimateTokens(strings.Repeat("x", 100))
	if got != 25 {
		t.Fatalf("expected 25 for 100 ascii chars, got %d", got)
	}
}

func TestEstimateTokensChinese(t *testing.T) {
	got := EstimateTokens("你好世界")
	if got < 1 {
		t.Fatalf("expected at least 1 token for Chinese text, got %d", got)
	}
}

func TestEstimateTokensMixed(t *testing.T) {
	got := EstimateTokens("Hello 你好世界")
	if got < 2 {
		t.Fatalf("expected at least 2 tokens for mixed text, got %d", got)
	}
}

func TestEstimateTokensSingleByte(t *testing.T) {
	got := EstimateTokens("x")
	if got != 1 {
		t.Fatalf("expected 1 for single char (minimum), got %d", got)
	}
}

func TestEstimateTokensSingleChinese(t *testing.T) {
	got := EstimateTokens("你")
	if got != 1 {
		t.Fatalf("expected 1 for single Chinese char, got %d", got)
	}
}

// ─── ToBool edge cases ───────────────────────────────────────────────

func TestToBoolTrue(t *testing.T) {
	if !ToBool(true) {
		t.Fatal("expected true")
	}
}

func TestToBoolFalse(t *testing.T) {
	if ToBool(false) {
		t.Fatal("expected false")
	}
}

func TestToBoolNonBool(t *testing.T) {
	if ToBool("true") {
		t.Fatal("expected false for string 'true'")
	}
	if ToBool(1) {
		t.Fatal("expected false for int 1")
	}
	if ToBool(nil) {
		t.Fatal("expected false for nil")
	}
}

// ─── IntFrom edge cases ─────────────────────────────────────────────

func TestIntFromFloat64(t *testing.T) {
	if got := IntFrom(float64(42.5)); got != 42 {
		t.Fatalf("expected 42 for float64(42.5), got %d", got)
	}
}

func TestIntFromInt(t *testing.T) {
	if got := IntFrom(int(42)); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestIntFromInt64(t *testing.T) {
	if got := IntFrom(int64(42)); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestIntFromString(t *testing.T) {
	if got := IntFrom("42"); got != 0 {
		t.Fatalf("expected 0 for string, got %d", got)
	}
}

func TestIntFromNil(t *testing.T) {
	if got := IntFrom(nil); got != 0 {
		t.Fatalf("expected 0 for nil, got %d", got)
	}
}

// ─── WriteJSON ───────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, 200, map[string]any{"key": "value"})
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json content type, got %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["key"] != "value" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestWriteJSONStatusCodes(t *testing.T) {
	for _, code := range []int{200, 201, 400, 404, 500} {
		rec := httptest.NewRecorder()
		WriteJSON(rec, code, map[string]any{"status": code})
		if rec.Code != code {
			t.Fatalf("expected %d, got %d", code, rec.Code)
		}
	}
}

// ─── MessagesPrepare edge cases ──────────────────────────────────────

func TestMessagesPrepareEmpty(t *testing.T) {
	got := MessagesPrepare(nil)
	if got != "" {
		t.Fatalf("expected empty for nil messages, got %q", got)
	}
}

func TestMessagesPrepareMergesConsecutiveSameRole(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "user", "content": "World"},
	}
	got := MessagesPrepare(messages)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("expected both messages, got %q", got)
	}
	// Should be merged without <｜User｜> between them
	count := strings.Count(got, "<｜User｜>")
	if count != 0 {
		t.Fatalf("expected no User marker for first message pair, got %d occurrences", count)
	}
}

func TestMessagesPrepareAssistantMarkers(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "Hi"},
		{"role": "assistant", "content": "Hello!"},
	}
	got := MessagesPrepare(messages)
	if !strings.Contains(got, "<｜Assistant｜>") {
		t.Fatalf("expected assistant marker, got %q", got)
	}
	if !strings.Contains(got, "<｜end▁of▁sentence｜>") {
		t.Fatalf("expected end of sentence marker, got %q", got)
	}
}

func TestMessagesPrepareUnknownRole(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "unknown_role", "content": "Unknown"},
	}
	got := MessagesPrepare(messages)
	if !strings.Contains(got, "Unknown") {
		t.Fatalf("expected unknown role content, got %q", got)
	}
}

func TestMessagesPrepareMarkdownImageReplaced(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "Look at this: ![alt](https://example.com/img.png)"},
	}
	got := MessagesPrepare(messages)
	if strings.Contains(got, "![alt]") {
		t.Fatalf("expected markdown image to be replaced, got %q", got)
	}
}

func TestMessagesPrepareNilContent(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": nil},
	}
	got := MessagesPrepare(messages)
	if got != "null" {
		t.Logf("nil content handled as: %q", got)
	}
}

// ─── normalizeContent edge cases ─────────────────────────────────────

func TestNormalizeContentString(t *testing.T) {
	got := normalizeContent("hello")
	if got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestNormalizeContentArray(t *testing.T) {
	got := normalizeContent([]any{
		map[string]any{"type": "text", "text": "line1"},
		map[string]any{"type": "text", "text": "line2"},
	})
	if got != "line1\nline2" {
		t.Fatalf("expected 'line1\\nline2', got %q", got)
	}
}

func TestNormalizeContentArrayWithContentField(t *testing.T) {
	got := normalizeContent([]any{
		map[string]any{"type": "text", "content": "from-content"},
	})
	if got != "from-content" {
		t.Fatalf("expected 'from-content', got %q", got)
	}
}

func TestNormalizeContentArraySkipsImage(t *testing.T) {
	got := normalizeContent([]any{
		map[string]any{"type": "image_url", "image_url": "https://example.com/img.png"},
		map[string]any{"type": "text", "text": "caption"},
	})
	if strings.Contains(got, "image") {
		t.Fatalf("expected image skipped, got %q", got)
	}
	if got != "caption" {
		t.Fatalf("expected 'caption', got %q", got)
	}
}

func TestNormalizeContentArrayNonMapItems(t *testing.T) {
	got := normalizeContent([]any{"string item", 42})
	if got != "" {
		t.Fatalf("expected empty for non-map items, got %q", got)
	}
}

func TestNormalizeContentJSON(t *testing.T) {
	got := normalizeContent(map[string]any{"key": "value"})
	if !strings.Contains(got, `"key":"value"`) {
		t.Fatalf("expected JSON serialized, got %q", got)
	}
}

// ─── ConvertClaudeToDeepSeek edge cases ──────────────────────────────

func TestConvertClaudeToDeepSeekDefaultModel(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["model"] == "" {
		t.Fatal("expected default model")
	}
}

func TestConvertClaudeToDeepSeekWithStopSequences(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":          "claude-sonnet-4-5",
		"messages":       []any{map[string]any{"role": "user", "content": "Hi"}},
		"stop_sequences": []any{"\n\n"},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["stop"] == nil {
		t.Fatal("expected stop field from stop_sequences")
	}
}

func TestConvertClaudeToDeepSeekWithTemperature(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":       "claude-sonnet-4-5",
		"messages":    []any{map[string]any{"role": "user", "content": "Hi"}},
		"temperature": 0.7,
		"top_p":       0.9,
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["temperature"] != 0.7 {
		t.Fatalf("expected temperature 0.7, got %v", out["temperature"])
	}
	if out["top_p"] != 0.9 {
		t.Fatalf("expected top_p 0.9, got %v", out["top_p"])
	}
}

func TestConvertClaudeToDeepSeekNoSystem(t *testing.T) {
	store := config.LoadStore()
	req := map[string]any{
		"model":    "claude-sonnet-4-5",
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	msgs, _ := out["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message without system, got %d", len(msgs))
	}
}

func TestConvertClaudeToDeepSeekOpusUsesSlowMapping(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":[],"accounts":[],"claude_mapping":{"fast":"deepseek-chat","slow":"deepseek-reasoner"}}`)
	store := config.LoadStore()
	req := map[string]any{
		"model":    "claude-opus-4-6",
		"messages": []any{map[string]any{"role": "user", "content": "Hi"}},
	}
	out := ConvertClaudeToDeepSeek(req, store)
	if out["model"] != "deepseek-reasoner" {
		t.Fatalf("expected opus to use slow mapping, got %q", out["model"])
	}
}

// ─── FormatOpenAIStreamToolCalls ─────────────────────────────────────

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

// ─── ParseToolCalls more edge cases ──────────────────────────────────

func TestParseToolCallsNoToolNames(t *testing.T) {
	text := `{"tool_calls":[{"name":"search","input":{"q":"go"}}]}`
	calls := ParseToolCalls(text, nil)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call with nil tool names, got %d", len(calls))
	}
}

func TestParseToolCallsEmptyText(t *testing.T) {
	calls := ParseToolCalls("", []string{"search"})
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls for empty text, got %d", len(calls))
	}
}

func TestParseToolCallsMultipleTools(t *testing.T) {
	text := `{"tool_calls":[{"name":"search","input":{"q":"go"}},{"name":"get_weather","input":{"city":"beijing"}}]}`
	calls := ParseToolCalls(text, []string{"search", "get_weather"})
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
}

func TestParseToolCallsInputAsString(t *testing.T) {
	text := `{"tool_calls":[{"name":"search","input":"{\"q\":\"golang\"}"}]}`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Input["q"] != "golang" {
		t.Fatalf("expected parsed string input, got %#v", calls[0].Input)
	}
}

func TestParseToolCallsWithFunctionWrapper(t *testing.T) {
	text := `{"tool_calls":[{"function":{"name":"calc","arguments":{"x":1,"y":2}}}]}`
	calls := ParseToolCalls(text, []string{"calc"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "calc" {
		t.Fatalf("expected calc, got %q", calls[0].Name)
	}
}

func TestParseStandaloneToolCallsFencedCodeBlock(t *testing.T) {
	fenced := "Here's an example:\n```json\n{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}\n```\nDon't execute this."
	calls := ParseStandaloneToolCalls(fenced, []string{"search"})
	if len(calls) != 0 {
		t.Fatalf("expected fenced code block to be ignored, got %d calls", len(calls))
	}
}

// ─── looksLikeToolExampleContext ─────────────────────────────────────

func TestLooksLikeToolExampleContextNone(t *testing.T) {
	if looksLikeToolExampleContext("I will call the tool now") {
		t.Fatal("expected false for non-example context")
	}
}

func TestLooksLikeToolExampleContextFenced(t *testing.T) {
	if !looksLikeToolExampleContext("```json") {
		t.Fatal("expected true for fenced code block context")
	}
}
