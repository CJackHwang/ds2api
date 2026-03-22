package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func makeSSEHTTPResponse(lines ...string) *http.Response {
	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func decodeJSONBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode json failed: %v, body=%s", err, body)
	}
	return out
}

func parseSSEDataFrames(t *testing.T, body string) ([]map[string]any, bool) {
	t.Helper()
	lines := strings.Split(body, "\n")
	frames := make([]map[string]any, 0, len(lines))
	done := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			done = true
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			t.Fatalf("decode sse frame failed: %v, payload=%s", err, payload)
		}
		frames = append(frames, frame)
	}
	return frames, done
}

func streamHasRawToolJSONContent(frames []map[string]any) bool {
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			content, _ := delta["content"].(string)
			if strings.Contains(content, `"tool_calls"`) {
				return true
			}
		}
	}
	return false
}

func streamHasToolCallsDelta(frames []map[string]any) bool {
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if _, ok := delta["tool_calls"]; ok {
				return true
			}
		}
	}
	return false
}

func streamFinishReason(frames []map[string]any) string {
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				return reason
			}
		}
	}
	return ""
}

func streamToolCallArgumentChunks(frames []map[string]any) []string {
	out := make([]string, 0, 4)
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			toolCalls, _ := delta["tool_calls"].([]any)
			for _, tc := range toolCalls {
				tcm, _ := tc.(map[string]any)
				fn, _ := tcm["function"].(map[string]any)
				if args, ok := fn["arguments"].(string); ok && args != "" {
					out = append(out, args)
				}
			}
		}
	}
	return out
}

func TestHandleNonStreamToolCallInterceptsChatModel(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, context.Background(), resp, "cid1", "deepseek-chat", "prompt", false, []string{"search"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("unexpected choices: %#v", out["choices"])
	}
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
	}
	msg, _ := choice["message"].(map[string]any)
	if msg["content"] != nil {
		t.Fatalf("expected content nil, got %#v", msg["content"])
	}
	toolCalls, _ := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", msg["tool_calls"])
	}
}

func TestHandleNonStreamToolCallInterceptsReasonerModel(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"先想一下"}`,
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, context.Background(), resp, "cid2", "deepseek-reasoner", "prompt", true, []string{"search"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	if msg["reasoning_content"] != "先想一下" {
		t.Fatalf("expected reasoning_content, got %#v", msg["reasoning_content"])
	}
	if msg["content"] != nil {
		t.Fatalf("expected content nil, got %#v", msg["content"])
	}
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
	}
}

func TestHandleNonStreamUnknownToolIntercepted(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"not_in_schema\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, context.Background(), resp, "cid2b", "deepseek-chat", "prompt", false, []string{"search"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
	}
	msg, _ := choice["message"].(map[string]any)
	toolCalls, _ := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected tool_calls for unknown schema name, got %#v", msg["tool_calls"])
	}
}

func TestHandleNonStreamEmbeddedToolCallExamplePromotesToolCall(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"下面是示例："}`,
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: {"p":"response/content","v":"请勿执行。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, context.Background(), resp, "cid2c", "deepseek-chat", "prompt", false, []string{"search"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
	}
	msg, _ := choice["message"].(map[string]any)
	toolCalls, _ := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool_call field for embedded example: %#v", msg["tool_calls"])
	}
	content, _ := msg["content"].(string)
	if strings.Contains(content, `"tool_calls"`) {
		t.Fatalf("expected raw tool_calls json stripped from content, got %#v", content)
	}
}

func TestHandleNonStreamFencedToolCallExampleDoesNotPromoteToolCall(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		"data: {\"p\":\"response/content\",\"v\":\"```json\\n{\\\"tool_calls\\\":[{\\\"name\\\":\\\"search\\\",\\\"input\\\":{\\\"q\\\":\\\"go\\\"}}]}\\n```\"}",
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, context.Background(), resp, "cid2d", "deepseek-chat", "prompt", false, []string{"search"})
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] == "tool_calls" {
		t.Fatalf("expected fenced example to remain content-only, got finish_reason=%#v", choice["finish_reason"])
	}
	msg, _ := choice["message"].(map[string]any)
	toolCalls, _ := msg["tool_calls"].([]any)
	if len(toolCalls) != 0 {
		t.Fatalf("expected no tool_call field for fenced example: %#v", msg["tool_calls"])
	}
	content, _ := msg["content"].(string)
	if !strings.Contains(content, `"tool_calls"`) {
		t.Fatalf("expected fenced example content preserved, got %q", content)
	}
}

// Backward-compatible alias for historical test name used in CI logs.
func TestHandleNonStreamFencedToolCallExamplePromotesToolCall(t *testing.T) {
	TestHandleNonStreamFencedToolCallExampleDoesNotPromoteToolCall(t)
}

func TestHandleStreamToolCallInterceptsWithoutRawContentLeak(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\""}`,
		`data: {"p":"response/content","v":",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid3", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	foundToolIndex := false
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			toolCalls, _ := delta["tool_calls"].([]any)
			for _, tc := range toolCalls {
				tcm, _ := tc.(map[string]any)
				if _, ok := tcm["index"].(float64); ok {
					foundToolIndex = true
				}
			}
		}
	}
	if !foundToolIndex {
		t.Fatalf("expected stream tool_calls item with index, body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("raw tool_calls JSON leaked in content delta: %s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolCallLargeArgumentsStillIntercepted(t *testing.T) {
	h := &Handler{}
	large := strings.Repeat("a", 9000)
	payload := fmt.Sprintf(`{"tool_calls":[{"name":"search","input":{"q":"%s"}}]}`, large)
	splitAt := len(payload) / 2
	resp := makeSSEHTTPResponse(
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, payload[:splitAt]),
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, payload[splitAt:]),
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid3-large", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("raw tool_calls JSON leaked in content delta: %s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamReasonerToolCallInterceptsWithoutRawContentLeak(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"思考中"}`,
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid4", "deepseek-reasoner", "prompt", true, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	foundToolIndex := false
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			toolCalls, _ := delta["tool_calls"].([]any)
			for _, tc := range toolCalls {
				tcm, _ := tc.(map[string]any)
				if _, ok := tcm["index"].(float64); ok {
					foundToolIndex = true
				}
			}
		}
	}
	if !foundToolIndex {
		t.Fatalf("expected stream tool_calls item with index, body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("raw tool_calls JSON leaked in content delta: %s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}

	hasThinkingDelta := false
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if _, ok := delta["reasoning_content"]; ok {
				hasThinkingDelta = true
			}
		}
	}
	if !hasThinkingDelta {
		t.Fatalf("expected reasoning_content delta in reasoner stream: %s", rec.Body.String())
	}
}

func TestHandleStreamUnknownToolEmitsToolCall(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"not_in_schema\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid5", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta for unknown schema name, body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("did not expect raw tool_calls json leak for unknown schema name: %s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamUnknownToolNoArgsEmitsToolCall(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"not_in_schema\"}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid5b", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta for unknown schema name (no args), body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("did not expect raw tool_calls json leak for unknown schema name (no args): %s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolsPlainTextStreamsBeforeFinish(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"你好，"}`,
		`data: {"p":"response/content","v":"这是普通文本回复。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid6", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if streamHasToolCallsDelta(frames) {
		t.Fatalf("did not expect tool_calls delta for plain text: %s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	if got := content.String(); got == "" {
		t.Fatalf("expected streamed content in tool mode plain text, body=%s", rec.Body.String())
	}
	if streamFinishReason(frames) != "stop" {
		t.Fatalf("expected finish_reason=stop, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolCallMixedWithPlainTextSegments(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"下面是示例："}`,
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: {"p":"response/content","v":"请勿执行。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid7", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta in mixed prose stream, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "下面是示例：") || !strings.Contains(got, "请勿执行。") {
		t.Fatalf("expected pre/post plain text to pass sieve, got=%q", got)
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls for mixed prose, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolCallAfterLeadingTextRemainsText(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"我将调用工具。"}`,
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid7b", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "我将调用工具。") {
		t.Fatalf("expected leading text to keep streaming, got=%q", got)
	}

	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolCallWithSameChunkTrailingTextRemainsText(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}接下来我会继续说明。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid7c", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "接下来我会继续说明。") {
		t.Fatalf("expected trailing plain text to be preserved, got=%q", got)
	}

	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamFencedToolCallSnippetPromotesToolCall(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, "下面是调用示例：\n```json\n"),
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, "{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}\n```\n仅示例，不要执行。"),
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid7f", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta for fenced snippet, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if strings.Contains(strings.ToLower(got), "tool_calls") {
		t.Fatalf("expected raw fenced tool_calls snippet stripped from content, got=%q", got)
	}
	if strings.Contains(strings.ToLower(got), "```json") || strings.Contains(got, "\n```\n") {
		t.Fatalf("expected consumed fenced tool payload to not leave empty code fence, got=%q", got)
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamStandaloneToolCallAfterClosedFenceKeepsFence(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, "先给一个代码示例：\n```text\nhello\n```\n"),
		fmt.Sprintf(`data: {"p":"response/content","v":%q}`, "{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"),
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid7g", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta for standalone payload, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "```") {
		t.Fatalf("expected closed fence before standalone tool json to be preserved, got=%q", got)
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamToolCallKeyAppearsLateRemainsText(t *testing.T) {
	h := &Handler{}
	spaces := strings.Repeat(" ", 200)
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{`+spaces+`"}`,
		`data: {"p":"response/content","v":"\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go\"}}]}"}`,
		`data: {"p":"response/content","v":"后置正文C。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid8", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "后置正文C。") {
		t.Fatalf("expected stream to continue after tool json convergence, got=%q", got)
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamInvalidToolJSONDoesNotLeakRawObject(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"前置正文D。"}`,
		`data: {"p":"response/content","v":"{'tool_calls':[{'name':'search','input':{'q':'go'}}]}"}`,
		`data: {"p":"response/content","v":"后置正文E。"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid9", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if streamHasToolCallsDelta(frames) {
		t.Fatalf("did not expect tool_calls delta for invalid json, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	got := content.String()
	if !strings.Contains(got, "前置正文D。") || !strings.Contains(got, "后置正文E。") {
		t.Fatalf("expected pre/post plain text to remain, got=%q", content.String())
	}
	if !strings.Contains(strings.ToLower(got), "tool_calls") {
		t.Fatalf("expected invalid embedded tool-like json to pass through as text, got=%q", got)
	}
}

func TestHandleStreamIncompleteCapturedToolJSONFlushesAsTextOnFinalize(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\""}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid10", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if streamHasToolCallsDelta(frames) {
		t.Fatalf("did not expect tool_calls delta for incomplete json, body=%s", rec.Body.String())
	}
	content := strings.Builder{}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if c, ok := delta["content"].(string); ok {
				content.WriteString(c)
			}
		}
	}
	if !strings.Contains(strings.ToLower(content.String()), "tool_calls") || !strings.Contains(content.String(), "{") {
		t.Fatalf("expected incomplete capture to flush as plain text instead of stalling, got=%q", content.String())
	}
}

func TestHandleStreamToolCallArgumentsEmitAsSingleCompletedChunk(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\",\"input\":{\"q\":\"go"}`,
		`data: {"p":"response/content","v":"lang\",\"page\":1}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid11", "deepseek-chat", "prompt", false, false, []string{"search"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}
	if streamHasRawToolJSONContent(frames) {
		t.Fatalf("raw tool_calls JSON leaked in content delta: %s", rec.Body.String())
	}
	argChunks := streamToolCallArgumentChunks(frames)
	if len(argChunks) == 0 {
		t.Fatalf("expected tool call arguments chunk, got=%v body=%s", argChunks, rec.Body.String())
	}
	joined := strings.Join(argChunks, "")
	if !strings.Contains(joined, `"q":"golang"`) || !strings.Contains(joined, `"page":1`) {
		t.Fatalf("unexpected merged arguments stream: %q", joined)
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamMultiToolCallDoesNotMergeNamesOrArguments(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search_web\",\"input\":{\"query\":\"latest ai news\"}},{"}`,
		`data: {"p":"response/content","v":"\"name\":\"eval_javascript\",\"input\":{\"code\":\"1+1\"}}]}"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid12", "deepseek-chat", "prompt", false, false, []string{"search_web", "eval_javascript"})

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta, body=%s", rec.Body.String())
	}

	foundSearch := false
	foundEval := false
	foundIndex1 := false
	toolCallsDeltaLens := make([]int, 0, 2)
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			rawToolCalls, hasToolCalls := delta["tool_calls"]
			if !hasToolCalls {
				continue
			}
			toolCalls, _ := rawToolCalls.([]any)
			toolCallsDeltaLens = append(toolCallsDeltaLens, len(toolCalls))
			for _, tc := range toolCalls {
				tcm, _ := tc.(map[string]any)
				if idx, ok := tcm["index"].(float64); ok && int(idx) == 1 {
					foundIndex1 = true
				}
				fn, _ := tcm["function"].(map[string]any)
				name, _ := fn["name"].(string)
				switch name {
				case "search_web":
					foundSearch = true
				case "eval_javascript":
					foundEval = true
				case "search_webeval_javascript":
					t.Fatalf("unexpected merged tool name: %s, body=%s", name, rec.Body.String())
				}
				if args, ok := fn["arguments"].(string); ok && strings.Contains(args, `}{"`) {
					t.Fatalf("unexpected concatenated tool arguments: %q, body=%s", args, rec.Body.String())
				}
			}
		}
	}
	if !foundSearch || !foundEval {
		t.Fatalf("expected both tool names in stream deltas, foundSearch=%v foundEval=%v body=%s", foundSearch, foundEval, rec.Body.String())
	}
	if len(toolCallsDeltaLens) != 1 || toolCallsDeltaLens[0] != 2 {
		t.Fatalf("expected exactly one tool_calls delta with two calls, got lens=%v body=%s", toolCallsDeltaLens, rec.Body.String())
	}
	if !foundIndex1 {
		t.Fatalf("expected second tool call index in stream deltas, body=%s", rec.Body.String())
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}
