package history

import (
	"testing"
	"time"

	"ds2api/internal/promptcompat"
)

func TestSessionStoreGetSet(t *testing.T) {
	store := NewSessionStore(time.Minute)

	_, ok := store.Get("account1")
	if ok {
		t.Fatal("expected empty store Get to return false")
	}

	store.Set("account1", &SessionState{ChatSessionID: "session-1", ParentMessageID: 42, CreatedAt: time.Now()})
	state, ok := store.Get("account1")
	if !ok {
		t.Fatal("expected Get to find account1")
	}
	if state.ChatSessionID != "session-1" {
		t.Fatalf("expected chat_session_id=session-1, got %q", state.ChatSessionID)
	}
	if state.ParentMessageID != 42 {
		t.Fatalf("expected parent_message_id=42, got %d", state.ParentMessageID)
	}
}

func TestSessionStoreExpiry(t *testing.T) {
	store := NewSessionStore(50 * time.Millisecond)
	store.Set("account1", &SessionState{ChatSessionID: "session-1", CreatedAt: time.Now()})

	time.Sleep(100 * time.Millisecond)
	_, ok := store.Get("account1")
	if ok {
		t.Fatal("expected expired entry to be missed")
	}
}

func TestApplySessionContextFirstRequest(t *testing.T) {
	store := NewSessionStore(time.Minute)
	stdReq := promptcompat.StandardRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	result := store.ApplySessionContext(stdReq, "account1")
	if result.SessionIncremental {
		t.Fatal("expected first request to NOT be incremental")
	}
}

func TestApplySessionContextIncrementalRequest(t *testing.T) {
	store := NewSessionStore(time.Minute)

	// Simulate a previous request that sent 2 messages and got a response.
	store.StoreResponse("account1", 2, "session-abc", 10)

	// New request with 4 messages (2 previous + 1 assistant + 1 new user).
	newMessages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi there"},
		map[string]any{"role": "user", "content": "how are you?"},
	}
	stdReq := promptcompat.StandardRequest{
		Messages: newMessages,
	}
	result := store.ApplySessionContext(stdReq, "account1")

	if !result.SessionIncremental {
		t.Fatal("expected incremental request")
	}
	if result.SessionChatID != "session-abc" {
		t.Fatalf("expected session-abc, got %q", result.SessionChatID)
	}
	if result.SessionParentMsgID != 10 {
		t.Fatalf("expected parent_message_id=10, got %d", result.SessionParentMsgID)
	}
	// Messages should be trimmed to just the last user message.
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	msg, ok := result.Messages[0].(map[string]any)
	if !ok {
		t.Fatal("expected message to be map")
	}
	if msg["content"] != "how are you?" {
		t.Fatalf("expected last message content, got %q", msg["content"])
	}
}

func TestApplySessionContextSameCountNotIncremental(t *testing.T) {
	store := NewSessionStore(time.Minute)

	// Session exists with 3 messages.
	store.StoreResponse("account1", 3, "session-abc", 10)

	// New request with same count (3 messages) — should NOT be incremental.
	newMessages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "user", "content": "how are you?"},
	}
	stdReq := promptcompat.StandardRequest{
		Messages: newMessages,
	}
	result := store.ApplySessionContext(stdReq, "account1")
	if result.SessionIncremental {
		t.Fatal("expected non-incremental for same message count")
	}
}

func TestApplySessionContextDifferentAccount(t *testing.T) {
	store := NewSessionStore(time.Minute)

	store.StoreResponse("account1", 2, "session-abc", 10)

	newMessages := []any{
		map[string]any{"role": "system", "content": "You are helpful"},
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi there"},
		map[string]any{"role": "user", "content": "how are you?"},
	}
	stdReq := promptcompat.StandardRequest{
		Messages: newMessages,
	}
	result := store.ApplySessionContext(stdReq, "account2")
	if result.SessionIncremental {
		t.Fatal("expected non-incremental for different account")
	}
}
