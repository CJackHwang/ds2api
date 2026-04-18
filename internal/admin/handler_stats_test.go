package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockRequestStats struct {
	success int64
	failed  int64
}

func (m mockRequestStats) Snapshot() (int64, int64) {
	return m.success, m.failed
}

func TestGetStats(t *testing.T) {
	h := &Handler{Stats: mockRequestStats{success: 12, failed: 3}}
	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	rec := httptest.NewRecorder()

	h.getStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var data map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := data["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %#v", data["success"])
	}
	if got, _ := data["success_calls"].(float64); got != 12 {
		t.Fatalf("unexpected success_calls: %#v", data["success_calls"])
	}
	if got, _ := data["failed_calls"].(float64); got != 3 {
		t.Fatalf("unexpected failed_calls: %#v", data["failed_calls"])
	}
}

