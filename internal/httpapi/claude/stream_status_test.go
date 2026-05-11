package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type streamStatusClaudeOpenAIStub struct{}

func (streamStatusClaudeOpenAIStub) ChatCompletions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

type streamStatusClaudeStoreStub struct{}

func (streamStatusClaudeStoreStub) ModelAliases() map[string]string { return nil }

func (streamStatusClaudeStoreStub) CurrentInputFileEnabled() bool { return true }
func (streamStatusClaudeStoreStub) CurrentInputFileMinChars() int { return 0 }
func (streamStatusClaudeStoreStub) ContextEngineMode() string     { return "off" }
func (streamStatusClaudeStoreStub) ParserV2Mode() string          { return "off" }

func captureClaudeStatusMiddleware(statuses *[]int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			*statuses = append(*statuses, ww.Status())
		})
	}
}

func TestClaudeMessagesStreamStatusCapturedAs200(t *testing.T) {
	statuses := make([]int, 0, 1)
	h := &Handler{
		Store:  streamStatusClaudeStoreStub{},
		OpenAI: streamStatusClaudeOpenAIStub{},
	}
	r := chi.NewRouter()
	r.Use(captureClaudeStatusMiddleware(&statuses))
	RegisterRoutes(r, h)

	reqBody := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(reqBody))
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
