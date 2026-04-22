package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/config"
)

func TestQueueStatusIncludesCallMetrics(t *testing.T) {
	h := newAdminTestHandler(t, `{"keys":["k1"]}`)
	store, ok := h.Store.(*config.Store)
	if !ok {
		t.Fatalf("unexpected store type: %T", h.Store)
	}
	store.RecordCall(true)
	store.RecordCall(false)

	req := httptest.NewRequest(http.MethodGet, "/admin/queue/status", nil)
	rec := httptest.NewRecorder()
	h.queueStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	calls, _ := body["calls"].(map[string]any)
	if intFrom(calls["total"]) != 2 {
		t.Fatalf("calls.total=%d want=2 body=%v", intFrom(calls["total"]), body)
	}
	if intFrom(calls["success"]) != 1 {
		t.Fatalf("calls.success=%d want=1 body=%v", intFrom(calls["success"]), body)
	}
	if intFrom(calls["failed"]) != 1 {
		t.Fatalf("calls.failed=%d want=1 body=%v", intFrom(calls["failed"]), body)
	}
}
