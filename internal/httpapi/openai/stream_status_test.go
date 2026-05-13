package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type streamStatusAuthStub struct{}

func (streamStatusAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{
		UseConfigToken: false,
		DeepSeekToken:  "direct-token",
		CallerID:       "caller:test",
		TriedAccounts:  map[string]bool{},
	}, nil
}

func (streamStatusAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{
		UseConfigToken: false,
		DeepSeekToken:  "direct-token",
		CallerID:       "caller:test",
		TriedAccounts:  map[string]bool{},
	}, nil
}

func (streamStatusAuthStub) Release(_ *auth.RequestAuth) {}

type streamStatusDSStub struct {
	resp *http.Response
}

func (m streamStatusDSStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m streamStatusDSStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (m streamStatusDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m streamStatusDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return m.resp, nil
}

func (m streamStatusDSStub) DeleteSessionForToken(_ context.Context, _ string, _ string) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (m streamStatusDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

type streamStatusDSSeqStub struct {
	resps    []*http.Response
	payloads []map[string]any
}

func (m *streamStatusDSSeqStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *streamStatusDSSeqStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (m *streamStatusDSSeqStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m *streamStatusDSSeqStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	m.payloads = append(m.payloads, clone)
	idx := len(m.payloads) - 1
	if idx >= len(m.resps) {
		idx = len(m.resps) - 1
	}
	return m.resps[idx], nil
}

func (m *streamStatusDSSeqStub) DeleteSessionForToken(_ context.Context, _ string, _ string) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (m *streamStatusDSSeqStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

func makeOpenAISSEHTTPResponse(lines ...string) *http.Response {
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

func newOpenAITestRouter(h *openAITestSurface) http.Handler {
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	return r
}

func captureStatusMiddleware(statuses *[]int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			*statuses = append(*statuses, ww.Status())
		})
	}
}

func TestChatCompletionsStreamStatusCapturedAs200(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, "data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one captured status, got %d", len(statuses))
	}
	if statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200 (not 000), got %d", statuses[0])
	}
}

func TestResponsesStreamStatusCapturedAs200(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, "data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","input":"hi","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one captured status, got %d", len(statuses))
	}
	if statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200 (not 000), got %d", statuses[0])
	}
}

func TestChatCompletionsStreamContentFilterStopsNormallyWithoutLeak(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"合法前缀"}`,
			`data: {"p":"response/status","v":"CONTENT_FILTER","accumulated_token_usage":77}`,
			`data: {"p":"response/content","v":"CONTENT_FILTER你好，这个问题我暂时无法回答，让我们换个话题再聊聊吧。"}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 || statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200, got %#v", statuses)
	}
	if strings.Contains(rec.Body.String(), "这个问题我暂时无法回答") {
		t.Fatalf("expected leaked content-filter suffix to be hidden, body=%s", rec.Body.String())
	}

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if len(frames) == 0 {
		t.Fatalf("expected at least one json frame, body=%s", rec.Body.String())
	}
	last := frames[len(frames)-1]
	choices, _ := last["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("expected one choice in final frame, got %#v", last)
	}
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] != "stop" {
		t.Fatalf("expected finish_reason=stop for content-filter upstream stop, got %#v", choice["finish_reason"])
	}
}

func TestChatCompletionsStreamEmitsFailureFrameWhenUpstreamOutputEmpty(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse("data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 || statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200, got %#v", statuses)
	}

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if len(frames) != 1 {
		t.Fatalf("expected one failure frame, got %#v body=%s", frames, rec.Body.String())
	}
	last := frames[0]
	statusCode, ok := last["status_code"].(float64)
	if !ok || int(statusCode) != http.StatusServiceUnavailable {
		t.Fatalf("expected status_code=503, got %#v body=%s", last["status_code"], rec.Body.String())
	}
	errObj, _ := last["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_unavailable" {
		t.Fatalf("expected code=upstream_unavailable, got %#v", last)
	}
}

func TestChatCompletionsStreamRetriesEmptyOutputOnSameSession(t *testing.T) {
	ds := &streamStatusDSSeqStub{resps: []*http.Response{
		makeOpenAISSEHTTPResponse(`data: {"response_message_id":42,"p":"response/thinking_content","v":"plan"}`, "data: [DONE]"),
		makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"visible"}`, "data: [DONE]"),
	}}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    ds,
	}
	reqBody := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newOpenAITestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.payloads) != 2 {
		t.Fatalf("expected one synthetic retry call, got %d", len(ds.payloads))
	}
	if ds.payloads[0]["chat_session_id"] != ds.payloads[1]["chat_session_id"] {
		t.Fatalf("expected retry to reuse session, payloads=%#v", ds.payloads)
	}
	retryPrompt := asString(ds.payloads[1]["prompt"])
	if !strings.Contains(retryPrompt, "Previous reply had no visible output. Please regenerate the visible final answer or tool call now.") {
		t.Fatalf("expected retry suffix in prompt, got %q", retryPrompt)
	}
	// Verify multi-turn chaining: retry must set parent_message_id from first call's response_message_id.
	if parentID, ok := ds.payloads[1]["parent_message_id"].(int); !ok || parentID != 42 {
		t.Fatalf("expected retry parent_message_id=42, got %#v", ds.payloads[1]["parent_message_id"])
	}

	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	doneCount := strings.Count(rec.Body.String(), "data: [DONE]")
	if doneCount != 1 {
		t.Fatalf("expected one [DONE], got %d body=%s", doneCount, rec.Body.String())
	}
	if len(frames) != 3 {
		t.Fatalf("expected reasoning, content, finish frames, got %#v body=%s", frames, rec.Body.String())
	}
	id := asString(frames[0]["id"])
	for _, frame := range frames[1:] {
		if asString(frame["id"]) != id {
			t.Fatalf("expected same completion id across retry stream, frames=%#v", frames)
		}
	}
	choices, _ := frames[1]["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if asString(delta["content"]) != "visible" {
		t.Fatalf("expected retry content delta, got %#v body=%s", delta, rec.Body.String())
	}
}

func TestChatCompletionsNonStreamRetriesThinkingOnlyOutput(t *testing.T) {
	ds := &streamStatusDSSeqStub{resps: []*http.Response{
		makeOpenAISSEHTTPResponse(`data: {"response_message_id":99,"p":"response/thinking_content","v":"plan"}`, "data: [DONE]"),
		makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"visible"}`, "data: [DONE]"),
	}}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    ds,
	}
	reqBody := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newOpenAITestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.payloads) != 2 {
		t.Fatalf("expected one synthetic retry call, got %d", len(ds.payloads))
	}
	// Verify multi-turn chaining.
	if parentID, ok := ds.payloads[1]["parent_message_id"].(int); !ok || parentID != 99 {
		t.Fatalf("expected retry parent_message_id=99, got %#v", ds.payloads[1]["parent_message_id"])
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	choices, _ := out["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if asString(message["content"]) != "visible" {
		t.Fatalf("expected retry visible content, got %#v", message)
	}
}

func TestChatCompletionsContentFilterDoesNotRetry(t *testing.T) {
	ds := &streamStatusDSSeqStub{resps: []*http.Response{
		makeOpenAISSEHTTPResponse(`data: {"code":"content_filter"}`),
		makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"visible"}`, "data: [DONE]"),
	}}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    ds,
	}
	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newOpenAITestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected content_filter 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.payloads) != 1 {
		t.Fatalf("expected no retry on content_filter, got %d calls", len(ds.payloads))
	}
}

func TestResponsesStreamUsageIgnoresBatchAccumulatedTokenUsage(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"hello"}`,
			`data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":190},{"p":"quasi_status","v":"FINISHED"}]}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","input":"hi","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 || statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200, got %#v", statuses)
	}
	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done {
		t.Fatalf("expected [DONE], body=%s", rec.Body.String())
	}
	if len(frames) == 0 {
		t.Fatalf("expected at least one json frame, body=%s", rec.Body.String())
	}
	last := frames[len(frames)-1]
	resp, _ := last["response"].(map[string]any)
	if resp == nil {
		t.Fatalf("expected response payload in final frame, got %#v", last)
	}
	usage, _ := resp["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("expected usage in response payload, got %#v", resp)
	}
	if got, _ := usage["output_tokens"].(float64); int(got) == 190 {
		t.Fatalf("expected upstream accumulated token usage to be ignored, got %#v", usage["output_tokens"])
	}
}

func TestResponsesStreamRetriesThinkingOnlyOutput(t *testing.T) {
	ds := &streamStatusDSSeqStub{resps: []*http.Response{
		makeOpenAISSEHTTPResponse(`data: {"response_message_id":77,"p":"response/thinking_content","v":"plan"}`, "data: [DONE]"),
		makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"visible"}`, "data: [DONE]"),
	}}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    ds,
	}
	reqBody := `{"model":"deepseek-v4-pro","input":"hi","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newOpenAITestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.payloads) != 2 {
		t.Fatalf("expected one synthetic retry call, got %d", len(ds.payloads))
	}
	// Verify multi-turn chaining.
	if parentID, ok := ds.payloads[1]["parent_message_id"].(int); !ok || parentID != 77 {
		t.Fatalf("expected retry parent_message_id=77, got %#v", ds.payloads[1]["parent_message_id"])
	}
	body := rec.Body.String()
	if strings.Contains(body, "response.failed") {
		t.Fatalf("did not expect premature response.failed, body=%s", body)
	}
	if !strings.Contains(body, "response.reasoning.delta") || !strings.Contains(body, "response.output_text.delta") || !strings.Contains(body, "response.completed") {
		t.Fatalf("expected reasoning, text delta, and completed events, body=%s", body)
	}
	if strings.Count(body, "data: [DONE]") != 1 {
		t.Fatalf("expected one [DONE], body=%s", body)
	}
}

func TestResponsesNonStreamRetriesThinkingOnlyOutput(t *testing.T) {
	ds := &streamStatusDSSeqStub{resps: []*http.Response{
		makeOpenAISSEHTTPResponse(`data: {"response_message_id":88,"p":"response/thinking_content","v":"plan"}`, "data: [DONE]"),
		makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"visible"}`, "data: [DONE]"),
	}}
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS:    ds,
	}
	reqBody := `{"model":"deepseek-v4-pro","input":"hi","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newOpenAITestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.payloads) != 2 {
		t.Fatalf("expected one synthetic retry call, got %d", len(ds.payloads))
	}
	// Verify multi-turn chaining.
	if parentID, ok := ds.payloads[1]["parent_message_id"].(int); !ok || parentID != 88 {
		t.Fatalf("expected retry parent_message_id=88, got %#v", ds.payloads[1]["parent_message_id"])
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	if asString(out["output_text"]) != "visible" {
		t.Fatalf("expected retry visible output_text, got %#v", out["output_text"])
	}
	output, _ := out["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("expected output items, got %#v", out)
	}
	item, _ := output[0].(map[string]any)
	content, _ := item["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("expected content entries, got %#v", item)
	}
	var textEntry map[string]any
	for _, entry := range content {
		obj, _ := entry.(map[string]any)
		if asString(obj["type"]) == "output_text" {
			textEntry = obj
			break
		}
	}
	if asString(textEntry["text"]) != "visible" {
		t.Fatalf("expected visible text entry, got %#v", content)
	}
}

func TestResponsesNonStreamUsageIgnoresPromptAndOutputTokenUsage(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &openAITestSurface{
		Store: mockOpenAIConfig{},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"ok"}`,
			`data: {"p":"response","o":"BATCH","v":[{"p":"token_usage","v":{"prompt_tokens":11,"completion_tokens":29}},{"p":"quasi_status","v":"FINISHED"}]}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	registerOpenAITestRoutes(r, h)

	reqBody := `{"model":"deepseek-v4-flash","input":"hi","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(statuses) != 1 || statuses[0] != http.StatusOK {
		t.Fatalf("expected captured status 200, got %#v", statuses)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	usage, _ := out["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("expected usage object, got %#v", out)
	}
	input, _ := usage["input_tokens"].(float64)
	output, _ := usage["output_tokens"].(float64)
	total, _ := usage["total_tokens"].(float64)
	if int(output) == 29 {
		t.Fatalf("expected upstream completion token usage to be ignored, got %#v", usage["output_tokens"])
	}
	if int(total) != int(input)+int(output) {
		t.Fatalf("expected total_tokens=input_tokens+output_tokens, usage=%#v", usage)
	}
}
