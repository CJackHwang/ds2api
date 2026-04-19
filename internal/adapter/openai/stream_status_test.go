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
	"ds2api/internal/deepseek"
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

func (m streamStatusDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ deepseek.UploadFileRequest, _ int) (*deepseek.UploadFileResult, error) {
	return &deepseek.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m streamStatusDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return m.resp, nil
}

func (m streamStatusDSStub) DeleteSessionForToken(_ context.Context, _ string, _ string) (*deepseek.DeleteSessionResult, error) {
	return &deepseek.DeleteSessionResult{Success: true}, nil
}

func (m streamStatusDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
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
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, "data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`
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
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, "data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","input":"hi","stream":true}`
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
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"合法前缀"}`,
			`data: {"p":"response/status","v":"CONTENT_FILTER","accumulated_token_usage":77}`,
			`data: {"p":"response/content","v":"CONTENT_FILTER你好，这个问题我暂时无法回答，让我们换个话题再聊聊吧。"}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`
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
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS:    streamStatusDSStub{resp: makeOpenAISSEHTTPResponse("data: [DONE]")},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`
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
	if !ok || int(statusCode) != http.StatusTooManyRequests {
		t.Fatalf("expected status_code=429, got %#v body=%s", last["status_code"], rec.Body.String())
	}
	errObj, _ := last["error"].(map[string]any)
	if asString(errObj["code"]) != "upstream_empty_output" {
		t.Fatalf("expected code=upstream_empty_output, got %#v", last)
	}
}

func TestResponsesStreamUsageIgnoresBatchAccumulatedTokenUsage(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"hello"}`,
			`data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":190},{"p":"quasi_status","v":"FINISHED"}]}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","input":"hi","stream":true}`
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

func TestResponsesNonStreamUsageIgnoresPromptAndOutputTokenUsage(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &Handler{
		Store: mockOpenAIConfig{wideInput: true},
		Auth:  streamStatusAuthStub{},
		DS: streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(
			`data: {"p":"response/content","v":"ok"}`,
			`data: {"p":"response","o":"BATCH","v":[{"p":"token_usage","v":{"prompt_tokens":11,"completion_tokens":29}},{"p":"quasi_status","v":"FINISHED"}]}`,
		)},
	}
	r := chi.NewRouter()
	r.Use(captureStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"deepseek-chat","input":"hi","stream":false}`
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
