package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ds2api/internal/usagestats"
)

func TestGetUsageStatsReturnsSnapshot(t *testing.T) {
	stats := usagestats.New()
	stats.Record(usagestats.Event{
		Surface:        "openai_chat",
		AccountID:      "tester@example.com",
		AccountType:    "managed",
		RequestedModel: "deepseek-chat",
		ResolvedModel:  "deepseek-chat",
		ResponseModel:  "deepseek-chat",
	})
	h := &Handler{Stats: stats}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/stats/usage", nil)
	h.getUsageStats(rec, req)

	if rec.Code != 200 {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	rows, ok := body["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("unexpected rows payload: %#v", body["rows"])
	}
}
