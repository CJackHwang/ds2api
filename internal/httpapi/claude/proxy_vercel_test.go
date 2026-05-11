package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type claudeProxyStoreStub struct {
	aliases map[string]string
}

func (s claudeProxyStoreStub) ModelAliases() map[string]string { return s.aliases }

func (claudeProxyStoreStub) CurrentInputFileEnabled() bool { return true }
func (claudeProxyStoreStub) CurrentInputFileMinChars() int { return 0 }
func (claudeProxyStoreStub) ContextEngineMode() string     { return "off" }
func (claudeProxyStoreStub) ParserV2Mode() string          { return "off" }

type openAIProxyStub struct {
	status int
	body   string
}

func TestClaudeProxyViaOpenAIPrefersGlobalAliasMapping(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store: claudeProxyStoreStub{
			aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"},
		},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(openAI.seenModel); got != "deepseek-v4-flash" {
		t.Fatalf("expected global alias mapped proxy model deepseek-v4-flash, got %q", got)
	}
}

func (s openAIProxyStub) ChatCompletions(w http.ResponseWriter, _ *http.Request) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

type openAIProxyCaptureStub struct {
	seenModel string
	seenReq   map[string]any
}

func (s *openAIProxyCaptureStub) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.seenReq = req
	if m, ok := req["model"].(string); ok {
		s.seenModel = m
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
}

func TestClaudeProxyViaOpenAIVercelPreparePassthrough(t *testing.T) {
	h := &Handler{OpenAI: openAIProxyStub{status: 200, body: `{"lease_id":"lease_123","payload":{"a":1}}`}}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json response, got err=%v body=%s", err, rec.Body.String())
	}
	if _, ok := out["lease_id"]; !ok {
		t.Fatalf("expected lease_id in prepare passthrough, got=%v", out)
	}
}

func TestClaudeProxyViaOpenAIUsesGlobalAliasMapping(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-3-opus": "deepseek-v4-pro"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-3-opus","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(openAI.seenModel); got != "deepseek-v4-pro" {
		t.Fatalf("expected mapped proxy model deepseek-v4-pro, got %q", got)
	}
}

func TestClaudeProxyViaOpenAIPreservesThinkingOverride(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled"},"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("expected translated OpenAI request to preserve disabled thinking, got %#v", openAI.seenReq)
	}
}

func TestClaudeProxyViaOpenAIEnablesThinkingInternallyByDefaultForNonStream(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("expected Claude non-stream default to enable downstream thinking internally, got %#v", openAI.seenReq)
	}
}

func TestClaudeProxyViaOpenAIEnablesThinkingWhenRequested(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","budget_tokens":1024},"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("expected Claude explicit thinking to enable downstream thinking, got %#v", openAI.seenReq)
	}
}

func TestClaudeProxyViaOpenAIEnablesStreamThinkingByDefault(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("expected Claude stream default to enable downstream thinking, got %#v", openAI.seenReq)
	}
}

func TestClaudeProxyViaOpenAIExposesThinkingBlocksByDefault(t *testing.T) {
	body := `{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"claude-sonnet-4-5","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"internal reasoning","tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	h := &Handler{OpenAI: openAIProxyStub{status: 200, body: body}}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, `"type":"thinking"`) {
		t.Fatalf("expected converted Claude response to expose thinking block, got %s", got)
	}
	if !strings.Contains(got, `"tool_use"`) {
		t.Fatalf("expected converted Claude response to preserve tool_use, got %s", got)
	}
}

func TestClaudeProxyViaOpenAIStripsThinkingBlocksWhenDisabled(t *testing.T) {
	body := `{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"claude-sonnet-4-5","choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_content":"internal reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	h := &Handler{OpenAI: openAIProxyStub{status: 200, body: body}}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled"},"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if strings.Contains(got, `"type":"thinking"`) {
		t.Fatalf("expected disabled thinking to strip thinking block, got %s", got)
	}
}

func TestClaudeProxyTranslatesInlineImageToOpenAIDataURL(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{OpenAI: openAI}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJDRA=="}}]}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
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
