package contextengine

import (
	"testing"
)

func makePlan(id string, included, trimmed, budgetUsed int, overflow bool, warnings []string) ContextPlan {
	segs := make([]ContextSegment, included)
	trims := make([]TrimmedSegment, trimmed)
	return ContextPlan{
		PlanID:           id,
		SegmentsIncluded: segs,
		SegmentsTrimmed:  trims,
		TokenBudget: TokenBudgetReport{
			Budget:   budgetUsed + 10,
			Used:     budgetUsed,
			Overflow: overflow,
		},
		Warnings: warnings,
	}
}

func TestPlanBuffer_PushAndSnapshot(t *testing.T) {
	buf := NewPlanBuffer(3)

	if buf.Len() != 0 {
		t.Fatalf("want 0 len, got %d", buf.Len())
	}

	buf.Push(makePlan("plan_a", 2, 0, 10, false, nil))
	buf.Push(makePlan("plan_b", 3, 1, 20, false, []string{"w1"}))

	snap := buf.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 items, got %d", len(snap))
	}
	// newest first
	if snap[0].PlanID != "plan_b" {
		t.Errorf("want plan_b first, got %s", snap[0].PlanID)
	}
	if snap[1].PlanID != "plan_a" {
		t.Errorf("want plan_a second, got %s", snap[1].PlanID)
	}
}

func TestPlanBuffer_CapEnforced(t *testing.T) {
	buf := NewPlanBuffer(2)

	buf.Push(makePlan("plan_1", 1, 0, 5, false, nil))
	buf.Push(makePlan("plan_2", 1, 0, 5, false, nil))
	buf.Push(makePlan("plan_3", 1, 0, 5, false, nil))

	if buf.Len() != 2 {
		t.Fatalf("want cap 2, got %d", buf.Len())
	}
	snap := buf.Snapshot()
	if snap[0].PlanID != "plan_3" {
		t.Errorf("want plan_3 newest, got %s", snap[0].PlanID)
	}
	if snap[1].PlanID != "plan_2" {
		t.Errorf("want plan_2 oldest, got %s", snap[1].PlanID)
	}
}

func TestPlanBuffer_Clear(t *testing.T) {
	buf := NewPlanBuffer(5)
	buf.Push(makePlan("plan_x", 1, 0, 1, false, nil))
	buf.Clear()
	if buf.Len() != 0 {
		t.Fatalf("want 0 after clear, got %d", buf.Len())
	}
}

func TestPlanBuffer_SummaryFields(t *testing.T) {
	buf := NewPlanBuffer(5)
	buf.Push(makePlan("plan_z", 4, 2, 100, true, []string{"warn1", "warn2"}))
	snap := buf.Snapshot()
	s := snap[0]

	if s.PlanID != "plan_z" {
		t.Errorf("PlanID: got %s", s.PlanID)
	}
	if s.SegmentsIncluded != 4 {
		t.Errorf("SegmentsIncluded: got %d", s.SegmentsIncluded)
	}
	if s.SegmentsTrimmed != 2 {
		t.Errorf("SegmentsTrimmed: got %d", s.SegmentsTrimmed)
	}
	if s.TokenBudgetUsed != 100 {
		t.Errorf("TokenBudgetUsed: got %d", s.TokenBudgetUsed)
	}
	if !s.TokenBudgetOverflow {
		t.Error("TokenBudgetOverflow: want true")
	}
	if len(s.Warnings) != 2 {
		t.Errorf("Warnings len: got %d", len(s.Warnings))
	}
	if s.CapturedAt == 0 {
		t.Error("CapturedAt must be set")
	}
}

func TestPlanBuffer_NilSafe(t *testing.T) {
	var buf *PlanBuffer
	buf.Push(makePlan("plan_nil", 1, 0, 1, false, nil))
	snap := buf.Snapshot()
	if snap != nil {
		t.Error("expected nil snapshot from nil buffer")
	}
	if buf.Len() != 0 {
		t.Error("Len of nil buf should be 0")
	}
	if buf.Cap() != 0 {
		t.Error("Cap of nil buf should be 0")
	}
	buf.Clear()
}

func TestNewPlanBuffer_ClampCap(t *testing.T) {
	b0 := NewPlanBuffer(0)
	if b0.Cap() != defaultPlanBufferCap {
		t.Errorf("cap 0 should clamp to default, got %d", b0.Cap())
	}
	bMax := NewPlanBuffer(9999)
	if bMax.Cap() != maxPlanBufferCap {
		t.Errorf("cap 9999 should clamp to max, got %d", bMax.Cap())
	}
}
