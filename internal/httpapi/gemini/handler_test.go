package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	dsclient "ds2api/internal/deepseek/client"
)

type testGeminiConfig struct{}

func (testGeminiConfig) ModelAliases() map[string]string { return nil }
func (testGeminiConfig) CurrentInputFileEnabled() bool   { return true }
func (testGeminiConfig) CurrentInputFileMinChars() int   { return 0 }

type testGeminiAuth struct {
	a   *auth.RequestAuth
	err error
}

func (m testGeminiAuth) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.a != nil {
		return m.a, nil
	}
	return &auth.RequestAuth{
		UseConfigToken: false,
		DeepSeekToken:  "direct-token",
		CallerID:       "caller:test",
		TriedAccounts:  map[string]bool{},
	}, nil
}

func (testGeminiAuth) Release(_ *auth.RequestAuth) {}

//nolint:unused // reserved test double for native Gemini DS-call path coverage.
type testGeminiDS struct {
	resp        *http.Response
	err         error
	uploadCalls []dsclient.UploadFileRequest
	payloads    []map[string]any
}

//nolint:unused // reserved test double for native Gemini DS-call path coverage.
func (m *testGeminiDS) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

//nolint:unused // reserved test double for native Gemini DS-call path coverage.
func (m *testGeminiDS) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

//nolint:unused // reserved test double for native Gemini DS-call path coverage.
func (m *testGeminiDS) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	m.uploadCalls = append(m.uploadCalls, req)
	id := "file-gemini-history"
	if len(m.uploadCalls) > 1 {
		id = "file-gemini-tools"
	}
	return &dsclient.UploadFileResult{ID: id}, nil
}

//nolint:unused // reserved test double for native Gemini DS-call path coverage.
func (m *testGeminiDS) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	m.payloads = append(m.payloads, payload)
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

type geminiOpenAIErrorStub struct {
	status  int
	body    string
	headers map[string]string
}

func (s geminiOpenAIErrorStub) ChatCompletions(w http.ResponseWriter, _ *http.Request) {
	for k, v := range s.headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

type geminiOpenAISuccessStub struct {
	stream  bool
	body    string
	seenReq map[string]any
}

func (s *geminiOpenAISuccessStub) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r != nil {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.seenReq = req
	}
	if s.stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello \"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		return
	}
	out := s.body
	if strings.TrimSpace(out) == "" {
		out = `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"eval_javascript","arguments":"{\"code\":\"1+1\"}"}}]},"finish_reason":"tool_calls"}]}`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}

//nolint:unused // helper retained for native Gemini stream fixture tests.
func makeGeminiUpstreamResponse(lines ...string) *http.Response {
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

func TestGeminiDirectAppliesCurrentInputFile(t *testing.T) {
	ds := &testGeminiDS{
		resp: makeGeminiUpstreamResponse(`data: {"p":"response/content","v":"ok"}`),
	}
	historyStore := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{
		Store:       testGeminiConfig{},
		Auth:        testGeminiAuth{},
		DS:          ds,
		ChatHistory: historyStore,
	}
	reqBody := `{"contents":[{"role":"user","parts":[{"text":"hello from gemini"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected one current input upload, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("unexpected upload filename: %q", ds.uploadCalls[0].Filename)
	}
	if len(ds.payloads) != 1 {
		t.Fatalf("expected one completion payload, got %d", len(ds.payloads))
	}
	refIDs, _ := ds.payloads[0]["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-gemini-history" {
		t.Fatalf("expected uploaded history ref id, got %#v", ds.payloads[0]["ref_file_ids"])
	}
	prompt, _ := ds.payloads[0]["prompt"].(string)
	if !strings.Contains(prompt, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation prompt, got %q", prompt)
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot history: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	full, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("get history item: %v", err)
	}
	if full.Surface != "gemini.generate_content" {
		t.Fatalf("unexpected surface: %q", full.Surface)
	}
	if full.Content != "ok" {
		t.Fatalf("expected raw upstream content, got %q", full.Content)
	}
	if full.HistoryText != string(ds.uploadCalls[0].Data) {
		t.Fatalf("expected uploaded current input file to be persisted in history text")
	}
	if len(full.Messages) != 1 || !strings.Contains(full.Messages[0].Content, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected persisted message to match upstream continuation prompt, got %#v", full.Messages)
	}
}

func TestGeminiCurrentInputFileUploadsToolsSeparately(t *testing.T) {
	ds := &testGeminiDS{
		resp: makeGeminiUpstreamResponse(`data: {"p":"response/content","v":"ok"}`),
	}
	h := &Handler{
		Store: testGeminiConfig{},
		Auth:  testGeminiAuth{},
		DS:    ds,
	}
	reqBody := `{
		"contents":[{"role":"user","parts":[{"text":"run code"}]}],
		"tools":[{"functionDeclarations":[{"name":"eval_javascript","description":"eval","parameters":{"type":"object","properties":{"code":{"type":"string"}}}}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" || ds.uploadCalls[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("unexpected upload filenames: %#v", ds.uploadCalls)
	}
	historyText := string(ds.uploadCalls[0].Data)
	if strings.Contains(historyText, "Description: eval") {
		t.Fatalf("history transcript should not embed tool descriptions, got %q", historyText)
	}
	toolsText := string(ds.uploadCalls[1].Data)
	if !strings.Contains(toolsText, "# DS2API_TOOLS.txt") || !strings.Contains(toolsText, "Tool: eval_javascript") || !strings.Contains(toolsText, "Description: eval") {
		t.Fatalf("expected tools transcript to include Gemini tool schema, got %q", toolsText)
	}
	refIDs, _ := ds.payloads[0]["ref_file_ids"].([]any)
	if len(refIDs) < 2 || refIDs[0] != "file-gemini-history" || refIDs[1] != "file-gemini-tools" {
		t.Fatalf("expected history and tools ref ids first, got %#v", ds.payloads[0]["ref_file_ids"])
	}
	prompt, _ := ds.payloads[0]["prompt"].(string)
	if !strings.Contains(prompt, "DS2API_TOOLS.txt") || !strings.Contains(prompt, "TOOL CALL FORMAT") {
		t.Fatalf("expected live prompt to reference tools file and retain format instructions, got %q", prompt)
	}
	if strings.Contains(prompt, "Description: eval") {
		t.Fatalf("live prompt should not inline tool descriptions, got %q", prompt)
	}
}

func TestGeminiRoutesRegistered(t *testing.T) {
	h := &Handler{
		Store: testGeminiConfig{},
		Auth:  testGeminiAuth{err: auth.ErrUnauthorized},
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	paths := []string{
		"/v1beta/models/gemini-2.5-pro:generateContent",
		"/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		"/v1/models/gemini-2.5-pro:generateContent",
		"/v1/models/gemini-2.5-pro:streamGenerateContent",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("expected route %s to be registered, got 404", path)
		}
	}
}

func TestGenerateContentReturnsFunctionCallParts(t *testing.T) {
	h := &Handler{
		Store: testGeminiConfig{},
		OpenAI: &geminiOpenAISuccessStub{
			body: `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"eval_javascript","arguments":"{\"code\":\"1+1\"}"}}]},"finish_reason":"tool_calls"}]}`,
		},
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{
		"contents":[{"role":"user","parts":[{"text":"call tool"}]}],
		"tools":[{"functionDeclarations":[{"name":"eval_javascript","description":"eval","parameters":{"type":"object","properties":{"code":{"type":"string"}}}}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	candidates, _ := out["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatalf("expected non-empty candidates: %#v", out)
	}
	c0, _ := candidates[0].(map[string]any)
	content, _ := c0["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	if len(parts) == 0 {
		t.Fatalf("expected non-empty parts: %#v", content)
	}
	part0, _ := parts[0].(map[string]any)
	functionCall, _ := part0["functionCall"].(map[string]any)
	if functionCall["name"] != "eval_javascript" {
		t.Fatalf("expected functionCall name eval_javascript, got %#v", functionCall)
	}
}

func TestGenerateContentMixedToolSnippetAlsoTriggersFunctionCall(t *testing.T) {
	h := &Handler{Store: testGeminiConfig{}, OpenAI: &geminiOpenAISuccessStub{}}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{
		"contents":[{"role":"user","parts":[{"text":"call tool"}]}],
		"tools":[{"functionDeclarations":[{"name":"eval_javascript","description":"eval","parameters":{"type":"object","properties":{"code":{"type":"string"}}}}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	candidates, _ := out["candidates"].([]any)
	c0, _ := candidates[0].(map[string]any)
	content, _ := c0["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	part0, _ := parts[0].(map[string]any)
	functionCall, _ := part0["functionCall"].(map[string]any)
	if functionCall["name"] != "eval_javascript" {
		t.Fatalf("expected functionCall name eval_javascript for mixed snippet, got %#v", functionCall)
	}
}

func TestStreamGenerateContentEmitsSSE(t *testing.T) {
	h := &Handler{
		Store:  testGeminiConfig{},
		OpenAI: &geminiOpenAISuccessStub{stream: true},
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-2.5-pro:streamGenerateContent?alt=sse", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	frames := extractGeminiSSEFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("expected non-empty stream frames, body=%s", rec.Body.String())
	}
	last := frames[len(frames)-1]
	candidates, _ := last["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatalf("expected finish frame candidates, got %#v", last)
	}
	c0, _ := candidates[0].(map[string]any)
	content, _ := c0["content"].(map[string]any)
	if content == nil {
		t.Fatalf("expected non-null content in finish frame, got %#v", c0)
	}
	parts, _ := content["parts"].([]any)
	if len(parts) == 0 {
		t.Fatalf("expected non-empty parts in finish frame content, got %#v", content)
	}
}

func TestNativeStreamGenerateContentEmitsThoughtParts(t *testing.T) {
	h := &Handler{}
	resp := makeGeminiUpstreamResponse(
		`data: {"p":"response/thinking_content","v":"think"}`,
		`data: {"p":"response/content","v":"answer"}`,
		`data: [DONE]`,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", nil)

	h.handleStreamGenerateContent(rec, req, resp, "gemini-2.5-pro", "prompt", true, false, nil, nil)

	frames := extractGeminiSSEFrames(t, rec.Body.String())
	if len(frames) < 2 {
		t.Fatalf("expected thought and text stream frames, body=%s", rec.Body.String())
	}
	var gotThought, gotText string
	for _, frame := range frames {
		for _, part := range geminiPartsFromFrame(frame) {
			if part["thought"] == true {
				gotThought += asString(part["text"])
			} else {
				gotText += asString(part["text"])
			}
		}
	}
	if gotThought != "think" {
		t.Fatalf("expected thought part, got %q body=%s", gotThought, rec.Body.String())
	}
	if !strings.Contains(gotText, "answer") {
		t.Fatalf("expected text part answer, got %q body=%s", gotText, rec.Body.String())
	}
}

func TestBuildGeminiPartsFromFinalIncludesThoughtPart(t *testing.T) {
	parts := buildGeminiPartsFromFinal("answer", "think", nil)
	if len(parts) != 2 {
		t.Fatalf("expected thought + answer parts, got %#v", parts)
	}
	if parts[0]["thought"] != true || parts[0]["text"] != "think" {
		t.Fatalf("expected first part to be thought, got %#v", parts[0])
	}
	if _, ok := parts[1]["thought"]; ok {
		t.Fatalf("expected second part to be visible text, got %#v", parts[1])
	}
	if parts[1]["text"] != "answer" {
		t.Fatalf("expected answer text, got %#v", parts[1])
	}
}

func TestGeminiProxyTranslatesInlineImageToOpenAIDataURL(t *testing.T) {
	openAI := &geminiOpenAISuccessStub{}
	h := &Handler{Store: testGeminiConfig{}, OpenAI: openAI}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{"contents":[{"role":"user","parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"QUJDRA=="}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	messages, _ := openAI.seenReq["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected one translated message, got %#v", openAI.seenReq)
	}
	msg, _ := messages[0].(map[string]any)
	content, _ := msg["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected translated content blocks, got %#v", msg)
	}
	imageBlock, _ := content[1].(map[string]any)
	if strings.TrimSpace(asString(imageBlock["type"])) != "image_url" {
		t.Fatalf("expected image_url block, got %#v", imageBlock)
	}
	imageURL, _ := imageBlock["image_url"].(map[string]any)
	if !strings.HasPrefix(strings.TrimSpace(asString(imageURL["url"])), "data:image/png;base64,") {
		t.Fatalf("expected translated data url, got %#v", imageBlock)
	}
}

func TestGeminiProxyViaOpenAIDisablesThinkingBudgetZero(t *testing.T) {
	openAI := &geminiOpenAISuccessStub{}
	h := &Handler{Store: testGeminiConfig{}, OpenAI: openAI}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("expected Gemini thinkingBudget=0 to disable OpenAI thinking, got %#v", openAI.seenReq)
	}
}

func TestGeminiProxyViaOpenAIEnablesPositiveThinkingBudget(t *testing.T) {
	openAI := &geminiOpenAISuccessStub{}
	h := &Handler{Store: testGeminiConfig{}, OpenAI: openAI}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	body := `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":1024}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("expected Gemini positive thinkingBudget to enable OpenAI thinking, got %#v", openAI.seenReq)
	}
}

func TestGenerateContentOpenAIProxyErrorUsesGeminiEnvelope(t *testing.T) {
	h := &Handler{
		Store: testGeminiConfig{},
		OpenAI: geminiOpenAIErrorStub{
			status: http.StatusUnauthorized,
			body:   `{"error":{"message":"invalid api key"}}`,
			headers: map[string]string{
				"WWW-Authenticate":      `Bearer realm="example"`,
				"Retry-After":           "30",
				"X-RateLimit-Remaining": "0",
			},
		},
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json body: %v", err)
	}
	errObj, _ := out["error"].(map[string]any)
	if errObj["status"] != "UNAUTHENTICATED" {
		t.Fatalf("expected Gemini status UNAUTHENTICATED, got=%v", errObj["status"])
	}
	if errObj["message"] != "invalid api key" {
		t.Fatalf("expected parsed error message, got=%v", errObj["message"])
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("expected WWW-Authenticate header to be preserved")
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("expected Retry-After header 30, got=%q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("expected X-RateLimit-Remaining header 0, got=%q", got)
	}
}

func extractGeminiSSEFrames(t *testing.T, body string) []map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(body))
	out := make([]map[string]any, 0, 4)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		raw := line
		if strings.HasPrefix(line, "data: ") {
			raw = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
		if raw == "" {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			continue
		}
		out = append(out, frame)
	}
	return out
}

func geminiPartsFromFrame(frame map[string]any) []map[string]any {
	candidates, _ := frame["candidates"].([]any)
	if len(candidates) == 0 {
		return nil
	}
	c0, _ := candidates[0].(map[string]any)
	content, _ := c0["content"].(map[string]any)
	rawParts, _ := content["parts"].([]any)
	parts := make([]map[string]any, 0, len(rawParts))
	for _, raw := range rawParts {
		part, _ := raw.(map[string]any)
		if part != nil {
			parts = append(parts, part)
		}
	}
	return parts
}
