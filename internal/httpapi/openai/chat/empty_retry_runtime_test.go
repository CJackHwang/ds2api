package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/stream"
)

func TestConsumeChatStreamAttemptMarksContextCancelledState(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		Stream:    true,
		UserInput: "hello",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now(),
		finalPrompt: "prompt",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	streamRuntime := newChatStreamRuntime(
		rec,
		http.NewResponseController(rec),
		true,
		"",
		"cid-cancelled",
		time.Now().Unix(),
		"deepseek-v4-flash",
		"prompt",
		false,
		false,
		true,
		nil,
		nil,
		promptcompat.DefaultToolChoicePolicy(),
		false,
		false,
	)
	resp := makeOpenAISSEHTTPResponse(
		`data: {"p":"response/content","v":"hello"}`,
		`data: [DONE]`,
	)

	h := &Handler{}
	terminalWritten, retryable := h.consumeChatStreamAttempt(req, resp, streamRuntime, "text", false, session, true)
	if !terminalWritten || retryable {
		t.Fatalf("expected cancelled attempt to terminate without retry, got terminalWritten=%v retryable=%v", terminalWritten, retryable)
	}
	if got, want := streamRuntime.finalErrorCode, string(stream.StopReasonContextCancelled); got != want {
		t.Fatalf("expected cancelled final error code %q, got %q", want, got)
	}
	if streamRuntime.finalErrorMessage == "" {
		t.Fatalf("expected cancelled final error message to be preserved")
	}

	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	full, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Status != "stopped" {
		t.Fatalf("expected stopped status, got %#v", full)
	}
}
