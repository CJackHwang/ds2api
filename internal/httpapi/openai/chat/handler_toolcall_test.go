package chat

import (
	"encoding/json"
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

// Backward-compatible alias for historical test name used in CI logs.
func TestHandleNonStreamReturns429WhenUpstreamOutputEmpty(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":""}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-empty", "deepseek-v4-flash", "prompt", false, false, nil, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for empty upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_empty_output" {
		t.Fatalf("expected code=upstream_empty_output, got %#v", out)
	}
}

func TestHandleNonStreamReturnsContentFilterErrorWhenUpstreamFilteredWithoutOutput(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"code":"content_filter"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-empty-filtered", "deepseek-v4-flash", "prompt", false, false, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for filtered upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "content_filter" {
		t.Fatalf("expected code=content_filter, got %#v", out)
	}
}

func TestHandleNonStreamReturns429WhenUpstreamHasOnlyThinking(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"Only thinking"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-thinking-only", "deepseek-v4-pro", "prompt", true, false, nil, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for thinking-only upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_empty_output" {
		t.Fatalf("expected code=upstream_empty_output, got %#v", out)
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

	h.handleStream(rec, req, resp, "cid6", "deepseek-v4-flash", "prompt", false, false, []string{"search"}, nil)

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

func TestHandleStreamIncompleteCapturedToolJSONFlushesAsTextOnFinalize(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"{\"tool_calls\":[{\"name\":\"search\""}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid10", "deepseek-v4-flash", "prompt", false, false, []string{"search"}, nil)

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

func TestHandleStreamEmitsDistinctToolCallIDsAcrossSeparateToolBlocks(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":"前置文本\n<tool_calls>\n  <invoke name=\"read_file\">\n    <parameter name=\"path\">README.MD</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: {"p":"response/content","v":"中间文本\n<tool_calls>\n  <invoke name=\"search\">\n    <parameter name=\"q\">golang</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-multi", "deepseek-v4-flash", "prompt", false, false, []string{"read_file", "search"}, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}

	ids := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			toolCalls, _ := delta["tool_calls"].([]any)
			for _, rawCall := range toolCalls {
				call, _ := rawCall.(map[string]any)
				id := asString(call["id"])
				if id == "" {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}

	if len(ids) != 2 {
		t.Fatalf("expected two distinct tool call ids, got %#v body=%s", ids, rec.Body.String())
	}
	if ids[0] == ids[1] {
		t.Fatalf("expected distinct tool call ids across blocks, got %#v body=%s", ids, rec.Body.String())
	}
}

// TestHandleStreamDetectsToolCallInThinkingChannel tests that streaming path
// detects tool calls when they appear inside thinking content (not text content).
// This verifies the fix for the 429 empty output bug when thinking has tool calls.
func TestHandleStreamDetectsToolCallInThinkingChannel(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"我需要调用 read_file 工具来读取文件\n<tool_calls>\n  <invoke name=\"read_file\">\n    <parameter name=\"path\">README.MD</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: {"p":"response/content","v":""}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-thinking-tc", "deepseek-v4-flash", "prompt", true, false, []string{"read_file"}, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}

	reason := streamFinishReason(frames)
	if reason != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls when thinking has tool calls, got %s body=%s", reason, rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta when thinking has tool calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamThinkingToolCallDoesNotLeakXML(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"先读取文件\n<tool_calls>\n  "}`,
		`data: {"p":"response/thinking_content","v":"<invoke name=\"read_file\">\n    <parameter name=\"path\">README.MD</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-thinking-split", "deepseek-v4-flash", "prompt", true, false, []string{"read_file"}, nil)

	body := rec.Body.String()
	if strings.Contains(body, "<tool_calls>") || strings.Contains(body, "<invoke name=\"read_file\">") {
		t.Fatalf("expected thinking tool XML to stay out of chat SSE body, got %s", body)
	}
}

// TestHandleNonStreamDetectsToolCallInThinkingChannel tests non-streaming path
// for tool calls in thinking content instead of 429 empty output.
func TestHandleNonStreamDetectsToolCallInThinkingChannel(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"工具调用出现在 thinking 中\n<tool_calls>\n  <invoke name=\"search\">\n    <parameter name=\"q\">golang</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-ns-thinking", "deepseek-v4-flash", "prompt", true, false, []string{"search"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 when thinking contains tool calls, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("expected at least one choice, got %#v", out)
	}
	choice := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	if _, ok := msg["tool_calls"]; !ok {
		t.Fatalf("expected tool_calls in message when thinking has <tool_calls>, got %#v", out)
	}
	reasoning, _ := msg["reasoning_content"].(string)
	if strings.Contains(reasoning, "<tool_calls>") || strings.Contains(reasoning, "</tool_calls>") {
		t.Fatalf("expected reasoning_content to be cleaned of <tool_calls> XML, got %q", reasoning)
	}
}

// TestHandleStreamThinkingOnlyToolCallNo429 tests that streaming returns 200
// when ALL content is thinking with tool calls (no text at all).
func TestHandleStreamThinkingOnlyToolCallNo429(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"<tool_calls>\n  <invoke name=\"write_file\">\n    <parameter name=\"path\">output.txt</parameter>\n    <parameter name=\"content\">hello</parameter>\n  </invoke>\n</tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-thinking-only", "deepseek-v4-flash", "prompt", true, false, []string{"write_file"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 when thinking has tool calls, got %d body=%s", rec.Code, rec.Body.String())
	}
	frames, _ := parseSSEDataFrames(t, rec.Body.String())
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls when thinking has plain tool calls, got %v body=%s", streamFinishReason(frames), rec.Body.String())
	}
}
