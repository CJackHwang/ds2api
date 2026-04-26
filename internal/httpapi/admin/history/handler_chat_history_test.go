package history

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/chathistory"
	"ds2api/internal/config"
)

func newChatHistoryAdminHarness(t *testing.T) (*Handler, *chathistory.Store) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	t.Setenv("DS2API_CONFIG_PATH", configPath)
	t.Setenv("DS2API_ADMIN_KEY", "admin")
	t.Setenv("DS2API_CONFIG_JSON", "")
	store, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatalf("load config store failed: %v", err)
	}
	historyStore := chathistory.New(filepath.Join(dir, "chat_history.json"))
	return &Handler{Store: store, ChatHistory: historyStore}, historyStore
}

func TestGetChatHistoryAndUpdateSettings(t *testing.T) {
	h, historyStore := newChatHistoryAdminHarness(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		AccountID: "user@example.com",
		Model:     "deepseek-v4-flash",
		UserInput: "hello",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	if _, err := historyStore.Update(entry.ID, chathistory.UpdateParams{
		Status:    "success",
		Content:   "world",
		Completed: true,
	}); err != nil {
		t.Fatalf("update history failed: %v", err)
	}

	r := chi.NewRouter()
	RegisterRoutes(r, h)

	req := httptest.NewRequest(http.MethodGet, "/chat-history", nil)
	req.Header.Set("Authorization", "Bearer admin")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	items, _ := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one history item, got %#v", payload)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatalf("expected list etag header")
	}

	notModifiedReq := httptest.NewRequest(http.MethodGet, "/chat-history", nil)
	notModifiedReq.Header.Set("Authorization", "Bearer admin")
	notModifiedReq.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	notModifiedRec := httptest.NewRecorder()
	r.ServeHTTP(notModifiedRec, notModifiedReq)
	if notModifiedRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d body=%s", notModifiedRec.Code, notModifiedRec.Body.String())
	}

	itemReq := httptest.NewRequest(http.MethodGet, "/chat-history/"+entry.ID, nil)
	itemReq.Header.Set("Authorization", "Bearer admin")
	itemRec := httptest.NewRecorder()
	r.ServeHTTP(itemRec, itemReq)
	if itemRec.Code != http.StatusOK {
		t.Fatalf("expected item 200, got %d body=%s", itemRec.Code, itemRec.Body.String())
	}
	if itemRec.Header().Get("ETag") == "" {
		t.Fatalf("expected detail etag header")
	}
	var detailPayload map[string]any
	if err := json.Unmarshal(itemRec.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("decode item payload failed: %v", err)
	}
	itemData, _ := detailPayload["item"].(map[string]any)
	if itemData == nil {
		t.Fatalf("expected item payload, got %#v", detailPayload)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/chat-history/settings", bytes.NewReader([]byte(`{"limit":10}`)))
	updateReq.Header.Set("Authorization", "Bearer admin")
	updateRec := httptest.NewRecorder()
	r.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from settings update, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if snapshot.Limit != 10 {
		t.Fatalf("expected limit=10, got %d", snapshot.Limit)
	}

	disableReq := httptest.NewRequest(http.MethodPut, "/chat-history/settings", bytes.NewReader([]byte(`{"limit":0}`)))
	disableReq.Header.Set("Authorization", "Bearer admin")
	disableRec := httptest.NewRecorder()
	r.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from disable update, got %d body=%s", disableRec.Code, disableRec.Body.String())
	}
	snapshot, err = historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot after disable failed: %v", err)
	}
	if snapshot.Limit != chathistory.DisabledLimit {
		t.Fatalf("expected limit=0, got %d", snapshot.Limit)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected history preserved when disabled, got %d", len(snapshot.Items))
	}
}

func TestGetChatHistoryItemIncludesDiagnostics(t *testing.T) {
	h, historyStore := newChatHistoryAdminHarness(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		UserInput: "hello",
		Diagnostics: &chathistory.Diagnostics{
			CurrentInputUpload: &chathistory.FileUpload{
				Filename: "CURRENT_INPUT.txt",
				Bytes:    12345,
				FileID:   "file-current",
			},
			HistoryUpload: &chathistory.FileUpload{
				Filename: "HISTORY.txt",
				Bytes:    234,
				FileID:   "file-history",
			},
			RefFileIDs: []string{"file-history", "file-current"},
		},
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}

	r := chi.NewRouter()
	RegisterRoutes(r, h)

	itemReq := httptest.NewRequest(http.MethodGet, "/chat-history/"+entry.ID, nil)
	itemReq.Header.Set("Authorization", "Bearer admin")
	itemRec := httptest.NewRecorder()
	r.ServeHTTP(itemRec, itemReq)

	if itemRec.Code != http.StatusOK {
		t.Fatalf("expected item 200, got %d body=%s", itemRec.Code, itemRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(itemRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode item payload failed: %v", err)
	}
	itemData, _ := payload["item"].(map[string]any)
	if itemData == nil {
		t.Fatalf("expected item payload, got %#v", payload)
	}
	diag, _ := itemData["diagnostics"].(map[string]any)
	if diag == nil {
		t.Fatalf("expected diagnostics in payload, got %#v", itemData)
	}
	if _, ok := diag["current_input_upload"]; !ok {
		t.Fatalf("expected current_input_upload in payload, got %#v", diag)
	}
	if _, ok := diag["history_upload"]; !ok {
		t.Fatalf("expected history_upload in payload, got %#v", diag)
	}
}

func TestDeleteAndClearChatHistory(t *testing.T) {
	h, historyStore := newChatHistoryAdminHarness(t)
	entryA, err := historyStore.Start(chathistory.StartParams{UserInput: "a"})
	if err != nil {
		t.Fatalf("start A failed: %v", err)
	}
	if _, err := historyStore.Start(chathistory.StartParams{UserInput: "b"}); err != nil {
		t.Fatalf("start B failed: %v", err)
	}

	r := chi.NewRouter()
	RegisterRoutes(r, h)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/chat-history/"+entryA.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer admin")
	deleteRec := httptest.NewRecorder()
	r.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one item after delete, got %d", len(snapshot.Items))
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/chat-history", nil)
	clearReq.Header.Set("Authorization", "Bearer admin")
	clearRec := httptest.NewRecorder()
	r.ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d body=%s", clearRec.Code, clearRec.Body.String())
	}

	snapshot, err = historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 0 {
		t.Fatalf("expected empty items after clear, got %d", len(snapshot.Items))
	}
}
