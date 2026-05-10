package observe

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMiddlewareEmitsLogForCompletionRoute(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetModel(r.Context(), "deepseek-chat")
		SetAccount(r.Context(), "user@example.com")
		SetSurface(r.Context(), "openai.chat")
		SetUpstreamResponseAt(r.Context(), time.Now())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logOutput := buf.String()
	for _, field := range []string{"completion_request", "model_alias=deepseek-chat", "account_id=user***", "surface=openai.chat", "retry_count=0", "account_switch_count=0"} {
		if !strings.Contains(logOutput, field) {
			t.Errorf("log missing field %q\nlog output:\n%s", field, logOutput)
		}
	}
}

func TestMiddlewareSkipsNonCompletionRoute(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No SetModel call — simulates a healthz/models/admin route
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() > 0 {
		t.Errorf("expected no log for non-completion route, got:\n%s", buf.String())
	}
}

func TestMiddlewareRecordsRetryAndAccountSwitch(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetModel(r.Context(), "deepseek-chat")
		IncrRetry(r.Context())
		IncrRetry(r.Context())
		IncrAccountSwitch(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logOutput := buf.String()
	if !strings.Contains(logOutput, "retry_count=2") {
		t.Errorf("expected retry_count=2, got:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "account_switch_count=1") {
		t.Errorf("expected account_switch_count=1, got:\n%s", logOutput)
	}
}
