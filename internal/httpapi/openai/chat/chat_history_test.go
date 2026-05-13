package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
)

func newTestChatHistoryStore(t *testing.T) *chathistory.Store {
	t.Helper()
	store := chathistory.New(filepath.Join(t.TempDir(), "chat_history.json"))
	if err := store.Err(); err != nil {
		t.Fatalf("chat history store unavailable: %v", err)
	}
	return store
}

func blockChatHistoryDetailDir(t *testing.T, detailDir string) func() {
	t.Helper()
	blockedDir := detailDir + ".blocked"
	if err := os.RemoveAll(blockedDir); err != nil {
		t.Fatalf("remove blocked detail dir failed: %v", err)
	}
	if err := os.Rename(detailDir, blockedDir); err != nil {
		t.Fatalf("move detail dir aside failed: %v", err)
	}
	if err := os.RemoveAll(detailDir); err != nil {
		t.Fatalf("remove blocked detail path failed: %v", err)
	}
	if err := os.WriteFile(detailDir, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("write blocked detail path failed: %v", err)
	}
	var once sync.Once
	return func() {
		t.Helper()
		once.Do(func() {
			if err := os.RemoveAll(detailDir); err != nil {
				t.Fatalf("remove blocking detail path failed: %v", err)
			}
			if err := os.Rename(blockedDir, detailDir); err != nil {
				t.Fatalf("restore detail dir failed: %v", err)
			}
		})
	}
}

func TestChatCompletionsNonStreamPersistsHistory(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		DS:          streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hi there"},{"role":"assistant","content":"previous answer"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.Status != "success" || item.UserInput != "hi there" {
		t.Fatalf("unexpected persisted history summary: %#v", item)
	}
	full, err := historyStore.Get(item.ID)
	if err != nil {
		t.Fatalf("expected detail item, got %v", err)
	}
	if full.Content != "hello world" {
		t.Fatalf("expected detail content persisted, got %#v", full)
	}
	if len(full.Messages) != 3 {
		t.Fatalf("expected all request messages persisted, got %#v", full.Messages)
	}
	if full.FinalPrompt == "" {
		t.Fatalf("expected final prompt to be persisted")
	}
	if item.CallerID != "caller:test" {
		t.Fatalf("expected caller hash persisted in summary, got %#v", item.CallerID)
	}
}

func TestChatHistoryNonStreamArchivesRawToolCallMarkup(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		UserInput: "call tool",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now().Add(-time.Second),
		finalPrompt: "call tool",
	}
	rawToolCall := `<tool_calls><invoke name="search"><parameter name="q">golang</parameter></invoke></tool_calls>`

	h := &Handler{}
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":`+strconv.Quote(rawToolCall)+`}`, `data: [DONE]`)
	h.handleNonStream(rec, resp, "cid-tool-history", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, session)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	full, err := historyStore.Get(entry.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Content != rawToolCall {
		t.Fatalf("expected raw tool markup archived, got %q", full.Content)
	}
	if full.FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %#v", full.FinishReason)
	}
}

func TestChatHistoryStreamArchivesRawToolCallMarkup(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		Stream:    true,
		UserInput: "call tool",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now().Add(-time.Second),
		finalPrompt: "call tool",
	}
	rawToolCall := `<tool_calls><invoke name="search"><parameter name="q">golang</parameter></invoke></tool_calls>`

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":`+strconv.Quote(rawToolCall)+`}`, `data: [DONE]`)
	h.handleStream(rec, req, resp, "cid-stream-tool-history", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, session)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	full, err := historyStore.Get(entry.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Content != rawToolCall {
		t.Fatalf("expected raw streamed tool markup archived, got %q", full.Content)
	}
	if full.FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %#v", full.FinishReason)
	}
}

func TestStartChatHistoryRecoversFromTransientWriteFailure(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	restore := blockChatHistoryDetailDir(t, historyStore.DetailDir())
	t.Cleanup(restore)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	a := &auth.RequestAuth{
		CallerID:  "caller:test",
		AccountID: "acct:test",
	}
	stdReq := promptcompat.StandardRequest{
		ResponseModel: "deepseek-v4-flash",
		Stream:        true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		FinalPrompt: "hello",
	}

	session := startChatHistory(historyStore, req, a, stdReq)
	if session == nil {
		t.Fatalf("expected session even when initial persistence fails")
		return
	}
	if session.disabled {
		t.Fatalf("expected session to remain active after transient start failure")
	}
	if session.entryID == "" {
		t.Fatalf("expected session entry id to be retained")
	}
	if err := historyStore.Err(); err != nil {
		t.Fatalf("transient start failure should not latch store error: %v", err)
	}

	session.lastPersist = time.Now().Add(-time.Second)
	session.progress("thinking", "partial")
	if session.disabled {
		t.Fatalf("expected session to remain active after transient update failure")
	}
	if session.entryID == "" {
		t.Fatalf("expected session entry id to remain set after update failure")
	}
	if err := historyStore.Err(); err != nil {
		t.Fatalf("transient update failure should not latch store error: %v", err)
	}

	restore()

	session.success(http.StatusOK, "thinking", "final answer", "stop", map[string]any{"total_tokens": 7})
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed after restore: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one persisted item after restore, got %#v", snapshot.Items)
	}
	full, err := historyStore.Get(session.entryID)
	if err != nil {
		t.Fatalf("get restored entry failed: %v", err)
	}
	if full.Status != "success" || full.Content != "final answer" {
		t.Fatalf("expected restored entry to persist final success, got %#v", full)
	}
}

func TestHandleStreamContextCancelledMarksHistoryStopped(t *testing.T) {
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
		finalPrompt: "hello",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, `data: [DONE]`)

	h.handleStream(rec, req, resp, "cid-stop", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, session)

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

func TestChatCompletionsRecordsAdminWebUISource(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		DS:          streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi there"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ds2-Source", "admin-webui-api-tester")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected admin webui source to be recorded, got %#v", snapshot.Items)
	}
}

func TestChatCompletionsSkipsHistoryWhenDisabled(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	if _, err := historyStore.SetLimit(chathistory.DisabledLimit); err != nil {
		t.Fatalf("disable history store failed: %v", err)
	}
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		DS:          streamStatusDSStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi there"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 0 {
		t.Fatalf("expected disabled history to stay empty, got %#v", snapshot.Items)
	}
}

func TestChatCompletionsCurrentInputFilePersistsNeutralPrompt(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	ds := &inlineUploadDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth:        streamStatusAuthStub{},
		DS:          ds,
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"system","content":"system instructions"},{"role":"user","content":"first user turn"},{"role":"assistant","content":"","reasoning_content":"hidden reasoning","tool_calls":[{"name":"search","arguments":{"query":"docs"}}]},{"role":"tool","name":"search","tool_call_id":"call-1","content":"tool result"},{"role":"user","content":"latest user turn"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
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
		t.Fatalf("expected detail item, got %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected current input upload to happen, got %d", len(ds.uploadCalls))
	}
	historyFilename := ds.uploadCalls[0].Filename
	if !strings.HasSuffix(historyFilename, ".txt") || len(historyFilename) < 10 {
		t.Fatalf("unexpected upload filename: %q", historyFilename)
	}
	if full.HistoryText != string(ds.uploadCalls[0].Data) {
		t.Fatalf("expected uploaded current input file to be persisted in history text")
	}
	if len(full.Messages) != 1 {
		t.Fatalf("expected continuation prompt to be the only persisted message, got %#v", full.Messages)
	}
	if !strings.Contains(full.Messages[0].Content, "Continue from the latest state in the attached "+historyFilename+" context.") {
		t.Fatalf("expected continuation prompt to be persisted, got %#v", full.Messages[0])
	}
}
