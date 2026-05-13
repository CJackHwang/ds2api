package chat

import (
	"context"
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

func TestHandleNonStreamSingleAttemptReturns503WhenUpstreamOutputEmpty(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/content","v":""}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-empty", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for empty upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_unavailable" {
		t.Fatalf("expected code=upstream_unavailable, got %#v", out)
	}
}

func TestHandleNonStreamSingleAttemptReturnsContentFilterErrorWhenUpstreamFilteredWithoutOutput(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"code":"content_filter"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-empty-filtered", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for filtered upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "content_filter" {
		t.Fatalf("expected code=content_filter, got %#v", out)
	}
}

func TestHandleNonStreamSingleAttemptReturns429WhenUpstreamHasOnlyThinking(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"Only thinking"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-thinking-only", "deepseek-v4-pro", "prompt", 0, true, false, nil, nil, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for thinking-only upstream output, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	errObj, _ := out["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_empty_output" {
		t.Fatalf("expected code=upstream_empty_output, got %#v", out)
	}
}

func TestHandleNonStreamPromotesThinkingToolCallsWhenTextEmpty(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"<tool_calls><invoke name=\"search\"><parameter name=\"q\">from-thinking</parameter></invoke></tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-thinking-tool", "deepseek-v4-pro", "prompt", 0, true, false, []string{"search"}, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for thinking tool calls, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("expected choices, got %#v", out)
	}
	choice, _ := choices[0].(map[string]any)
	if got := asString(choice["finish_reason"]); got != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
	}
	message, _ := choice["message"].(map[string]any)
	toolCalls, _ := message["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", message["tool_calls"])
	}
	if content, exists := message["content"]; !exists || content != nil {
		t.Fatalf("expected content nil when tool call promoted, got %#v", message["content"])
	}
}

func TestHandleNonStreamPromotesHiddenThinkingDSMLToolCallsWhenTextEmpty(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"<|DSML|tool_calls><|DSML|invoke name=\"search\"><|DSML|parameter name=\"q\">from-hidden-thinking</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()

	h.handleNonStream(rec, resp, "cid-hidden-thinking-tool", "deepseek-v4-pro", "prompt", 0, false, false, []string{"search"}, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for hidden thinking tool calls, got %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if _, ok := message["reasoning_content"]; ok {
		t.Fatalf("expected hidden thinking not to be exposed, got %#v", message)
	}
	toolCalls, _ := message["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one hidden-thinking tool call, got %#v", message["tool_calls"])
	}
	if got := asString(choice["finish_reason"]); got != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, got %#v", choice["finish_reason"])
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

	h.handleStream(rec, req, resp, "cid6", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, nil)

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

func TestHandleStreamThinkingDisabledDoesNotLeakHiddenFragmentContinuations(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"THINK","content":"我们"}]}`,
		`data: {"p":"response/fragments/-1/content","v":"被"}`,
		`data: {"v":"要求"}`,
		`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"RESPONSE","content":"答"}]}`,
		`data: {"p":"response/fragments/-1/content","v":"案"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-hidden-fragment", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
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
	if got := content.String(); got != "答案" {
		t.Fatalf("expected only visible response text, got %q body=%s", got, rec.Body.String())
	}
}

func TestHandleStreamEmitsSingleChoiceFramesForMultipleParsedParts(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"THINK","content":"我们"},{"type":"THINK","content":"被"},{"type":"THINK","content":"要求"},{"type":"RESPONSE","content":"答"},{"type":"RESPONSE","content":"案"}]}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-multi-parts", "deepseek-v4-pro", "prompt", 0, true, false, nil, nil, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	var reasoning, content strings.Builder
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		if len(choices) != 1 {
			t.Fatalf("expected exactly one choice per stream frame, got %d frame=%#v body=%s", len(choices), frame, rec.Body.String())
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		reasoning.WriteString(asString(delta["reasoning_content"]))
		content.WriteString(asString(delta["content"]))
	}
	if got := reasoning.String(); got != "我们被要求" {
		t.Fatalf("first-choice-only client would miss reasoning tokens: got %q body=%s", got, rec.Body.String())
	}
	if got := content.String(); got != "答案" {
		t.Fatalf("first-choice-only client would miss content tokens: got %q body=%s", got, rec.Body.String())
	}
}

func TestHandleStreamCoalescesSmallContentDeltas(t *testing.T) {
	h := &Handler{}
	lines := make([]string, 0, 101)
	for i := 0; i < 100; i++ {
		b, _ := json.Marshal(map[string]any{
			"p": "response/content",
			"v": "字",
		})
		lines = append(lines, "data: "+string(b))
	}
	lines = append(lines, "data: [DONE]")
	resp := makeSSEHTTPResponse(lines...)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-coalesce", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	var content strings.Builder
	contentDeltaFrames := 0
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		if len(choices) != 1 {
			t.Fatalf("expected exactly one choice per stream frame, got %d frame=%#v body=%s", len(choices), frame, rec.Body.String())
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if c, ok := delta["content"].(string); ok {
			contentDeltaFrames++
			content.WriteString(c)
		}
	}
	if got, want := content.String(), strings.Repeat("字", 100); got != want {
		t.Fatalf("coalesced stream content mismatch: got %q want %q body=%s", got, want, rec.Body.String())
	}
	if contentDeltaFrames >= 100 {
		t.Fatalf("expected coalescing to reduce 100 tiny content frames, got %d body=%s", contentDeltaFrames, rec.Body.String())
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

	h.handleStream(rec, req, resp, "cid10", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, nil)

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

func TestHandleStreamPromotesThinkingToolCallsOnFinalizeWithoutMidstreamIntercept(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"<tool_calls><invoke name=\"search\"><parameter name=\"q\">from-thinking</parameter></invoke></tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-thinking-stream", "deepseek-v4-pro", "prompt", 0, true, false, []string{"search"}, nil, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta from finalize fallback, body=%s", rec.Body.String())
	}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if asString(delta["reasoning_content"]) != "" {
				t.Fatalf("did not expect leaked reasoning_content markup, body=%s", rec.Body.String())
			}
		}
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
	}
}

func TestHandleStreamPromotesHiddenThinkingDSMLToolCallsOnFinalize(t *testing.T) {
	h := &Handler{}
	resp := makeSSEHTTPResponse(
		`data: {"p":"response/thinking_content","v":"<|DSML|tool_calls><|DSML|invoke name=\"search\"><|DSML|parameter name=\"q\">from-hidden-thinking</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	h.handleStream(rec, req, resp, "cid-hidden-thinking-stream", "deepseek-v4-pro", "prompt", 0, false, false, []string{"search"}, nil, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if !streamHasToolCallsDelta(frames) {
		t.Fatalf("expected tool_calls delta from hidden thinking fallback, body=%s", rec.Body.String())
	}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if asString(delta["reasoning_content"]) != "" {
				t.Fatalf("did not expect hidden reasoning_content delta, body=%s", rec.Body.String())
			}
		}
	}
	if streamFinishReason(frames) != "tool_calls" {
		t.Fatalf("expected finish_reason=tool_calls, body=%s", rec.Body.String())
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

	h.handleStream(rec, req, resp, "cid-multi", "deepseek-v4-flash", "prompt", 0, false, false, []string{"read_file", "search"}, nil, nil)

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

func TestHandleStreamCoercesSchemaDeclaredStringArgumentsOnFinalize(t *testing.T) {
	h := &Handler{}
	line := func(v string) string {
		b, _ := json.Marshal(map[string]any{"p": "response/content", "v": v})
		return "data: " + string(b)
	}
	resp := makeSSEHTTPResponse(
		line(`<tool_calls><invoke name="Write">{"input":{"content":{"message":"hi"},"taskId":1}}</invoke></tool_calls>`),
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	toolsRaw := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "Write",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
						"taskId":  map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	h.handleStream(rec, req, resp, "cid-string-protect", "deepseek-v4-flash", "prompt", 0, false, false, []string{"Write"}, toolsRaw, nil)

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			toolCalls, _ := delta["tool_calls"].([]any)
			if len(toolCalls) == 0 {
				continue
			}
			call, _ := toolCalls[0].(map[string]any)
			fn, _ := call["function"].(map[string]any)
			args := map[string]any{}
			if err := json.Unmarshal([]byte(asString(fn["arguments"])), &args); err != nil {
				t.Fatalf("decode streamed tool arguments failed: %v", err)
			}
			if args["content"] != `{"message":"hi"}` {
				t.Fatalf("expected streamed content stringified by schema, got %#v", args["content"])
			}
			if args["taskId"] != "1" {
				t.Fatalf("expected streamed taskId stringified by schema, got %#v", args["taskId"])
			}
			return
		}
	}
	t.Fatalf("expected at least one streamed tool call delta, body=%s", rec.Body.String())
}

func TestHandleNonStreamWithRetryIncludesRefFileTokensInUsage(t *testing.T) {
	h := &Handler{}

	run := func(refFileTokens int) map[string]any {
		resp := makeSSEHTTPResponse(
			`data: {"p":"response/content","v":"hello world"}`,
			`data: [DONE]`,
		)
		rec := httptest.NewRecorder()
		h.handleNonStreamWithRetry(rec, context.Background(), nil, resp, nil, "", "cid-ref", "deepseek-v4-flash", "prompt", refFileTokens, false, false, nil, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		return decodeJSONBody(t, rec.Body.String())
	}

	base := run(0)
	withRef := run(7)

	baseUsage, _ := base["usage"].(map[string]any)
	refUsage, _ := withRef["usage"].(map[string]any)
	if baseUsage == nil || refUsage == nil {
		t.Fatalf("expected usage objects, base=%#v ref=%#v", base["usage"], withRef["usage"])
	}

	getInt := func(m map[string]any, key string) int {
		t.Helper()
		v, ok := m[key].(float64)
		if !ok {
			t.Fatalf("expected numeric %s, got %#v", key, m[key])
		}
		return int(v)
	}

	if got := getInt(refUsage, "prompt_tokens") - getInt(baseUsage, "prompt_tokens"); got != 7 {
		t.Fatalf("expected prompt_tokens delta 7, got %d", got)
	}
	if got := getInt(refUsage, "total_tokens") - getInt(baseUsage, "total_tokens"); got != 7 {
		t.Fatalf("expected total_tokens delta 7, got %d", got)
	}
}
