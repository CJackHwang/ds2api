package contextplan

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/contextengine"
)

func TestListContextPlansEmpty(t *testing.T) {
	contextengine.GlobalPlanBuffer().Clear()
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/context-plans", nil)
	h.listContextPlans(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := out["count"]; !ok {
		t.Fatalf("expected count field, got %#v", out)
	}
	if _, ok := out["capacity"]; !ok {
		t.Fatalf("expected capacity field, got %#v", out)
	}
	if _, ok := out["items"]; !ok {
		t.Fatalf("expected items field, got %#v", out)
	}
	if out["count"] != float64(0) {
		t.Errorf("expected count=0, got %v", out["count"])
	}
}

func TestListContextPlansWithEntries(t *testing.T) {
	buf := contextengine.GlobalPlanBuffer()
	buf.Clear()
	buf.Push(contextengine.ContextPlan{
		PlanID:           "plan_test_1",
		SegmentsIncluded: []contextengine.ContextSegment{{ID: "s1"}},
		SegmentsTrimmed:  []contextengine.TrimmedSegment{},
		TokenBudget:      contextengine.TokenBudgetReport{Budget: 100, Used: 42, Overflow: false},
		Warnings:         []string{},
	})

	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/context-plans", nil)
	h.listContextPlans(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out["count"] != float64(1) {
		t.Errorf("expected count=1, got %v", out["count"])
	}
	items, ok := out["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item in items, got %#v", out["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not map: %T", items[0])
	}
	if item["plan_id"] != "plan_test_1" {
		t.Errorf("expected plan_id=plan_test_1, got %v", item["plan_id"])
	}
}

func TestClearContextPlans(t *testing.T) {
	buf := contextengine.GlobalPlanBuffer()
	buf.Push(contextengine.ContextPlan{PlanID: "plan_clear"})

	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/context-plans", nil)
	h.clearContextPlans(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out["success"] != true {
		t.Errorf("expected success=true, got %v", out["success"])
	}
	if buf.Len() != 0 {
		t.Errorf("buffer should be empty after clear, got len=%d", buf.Len())
	}
}
