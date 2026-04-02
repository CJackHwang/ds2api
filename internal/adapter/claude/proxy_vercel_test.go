package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type claudeProxyStoreStub struct {
	mapping map[string]string
}

func (s claudeProxyStoreStub) ClaudeMapping() map[string]string { return s.mapping }

type openAIProxyStub struct {
	status int
	body   string
}

func (s openAIProxyStub) ChatCompletions(w http.ResponseWriter, _ *http.Request) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

type capturingOpenAIProxyStub struct {
	status   int
	body     string
	lastBody []byte
}

func (s *capturingOpenAIProxyStub) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	s.lastBody, _ = io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

func TestClaudeProxyViaOpenAIVercelPreparePassthrough(t *testing.T) {
	h := &Handler{
		Store:  claudeProxyStoreStub{mapping: map[string]string{"fast": "deepseek-chat", "slow": "deepseek-reasoner"}},
		OpenAI: openAIProxyStub{status: 200, body: `{"lease_id":"lease_123","payload":{"a":1}}`},
	}
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

func TestClaudeProxyViaOpenAIResolvesModelUsingClaudeMapping(t *testing.T) {
	proxy := &capturingOpenAIProxyStub{status: 200, body: `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`}
	h := &Handler{
		Store:  claudeProxyStoreStub{mapping: map[string]string{"fast": "deepseek-chat", "slow": "deepseek-reasoner"}},
		OpenAI: proxy,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-opus-4-1","messages":[{"role":"user","content":"hi"}],"max_tokens":64}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json response, got err=%v body=%s", err, rec.Body.String())
	}
	var proxied map[string]any
	if err := json.Unmarshal(proxy.lastBody, &proxied); err != nil {
		t.Fatalf("expected proxied request json, got err=%v body=%s", err, string(proxy.lastBody))
	}
	if got, _ := proxied["model"].(string); got != "deepseek-reasoner" {
		t.Fatalf("expected proxied model mapped to deepseek-reasoner, got %q proxied=%v response=%v", got, proxied, out)
	}
}
