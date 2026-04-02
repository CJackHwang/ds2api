package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestGeminiProxyViaOpenAIVercelReleasePassthrough(t *testing.T) {
	h := &Handler{OpenAI: openAIProxyStub{status: 200, body: `{"success":true}`}}
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?__stream_release=1", strings.NewReader(`{"lease_id":"lease_123"}`))
	rec := httptest.NewRecorder()

	h.StreamGenerateContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json response, got err=%v body=%s", err, rec.Body.String())
	}
	if v, ok := out["success"].(bool); !ok || !v {
		t.Fatalf("expected success=true passthrough, got=%v", out)
	}
}

func TestGeminiProxyViaOpenAIConvertsErrorEnvelope(t *testing.T) {
	h := &Handler{OpenAI: openAIProxyStub{status: http.StatusUnauthorized, body: `{"error":{"message":"invalid api key"}}`}}
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	rec := httptest.NewRecorder()

	h.GenerateContent(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json response, got err=%v body=%s", err, rec.Body.String())
	}
	errMap, _ := out["error"].(map[string]any)
	if got, _ := errMap["status"].(string); got != "UNAUTHENTICATED" {
		t.Fatalf("expected gemini-style status, got %q response=%v", got, out)
	}
	if got, _ := errMap["message"].(string); got != "invalid api key" {
		t.Fatalf("expected extracted error message, got %q response=%v", got, out)
	}
}
