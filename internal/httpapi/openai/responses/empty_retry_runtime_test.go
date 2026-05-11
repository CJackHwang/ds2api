package responses

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/stream"
)

func makeResponsesOpenAISSEHTTPResponse(lines ...string) *http.Response {
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

func TestConsumeResponsesStreamAttemptMarksContextCancelledState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	streamRuntime := newResponsesStreamRuntime(
		rec,
		http.NewResponseController(rec),
		true,
		"resp-cancelled",
		"deepseek-v4-flash",
		"prompt",
		false,
		false,
		true,
		"",
		nil,
		nil,
		false,
		false,
		promptcompat.DefaultToolChoicePolicy(),
		"",
		nil,
		nil,
	)
	resp := makeResponsesOpenAISSEHTTPResponse(
		`data: {"p":"response/content","v":"hello"}`,
		`data: [DONE]`,
	)

	h := &Handler{}
	terminalWritten, retryable := h.consumeResponsesStreamAttempt(req, resp, streamRuntime, "text", false, true)
	if !terminalWritten || retryable {
		t.Fatalf("expected cancelled attempt to terminate without retry, got terminalWritten=%v retryable=%v", terminalWritten, retryable)
	}
	if !streamRuntime.failed {
		t.Fatalf("expected cancelled response stream to be marked failed")
	}
	if got, want := streamRuntime.finalErrorCode, string(stream.StopReasonContextCancelled); got != want {
		t.Fatalf("expected cancelled final error code %q, got %q", want, got)
	}
	if streamRuntime.finalErrorMessage == "" {
		t.Fatalf("expected cancelled final error message to be preserved")
	}
}
