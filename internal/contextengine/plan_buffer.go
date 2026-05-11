package contextengine

import (
	"sync"
	"time"
)

const (
	defaultPlanBufferCap = 50
	maxPlanBufferCap     = 200
)

// PlanSummary is a lightweight, JSON-serialisable record stored in PlanBuffer.
// It intentionally omits segment content to keep memory usage bounded.
type PlanSummary struct {
	PlanID            string           `json:"plan_id"`
	CapturedAt        int64            `json:"captured_at"`
	SegmentsIncluded  int              `json:"segments_included"`
	SegmentsTrimmed   int              `json:"segments_trimmed"`
	TokenBudgetUsed   int              `json:"token_budget_used"`
	TokenBudgetLimit  int              `json:"token_budget_limit"`
	TokenBudgetOverflow bool           `json:"token_budget_overflow"`
	Warnings          []string         `json:"warnings,omitempty"`
}

// PlanBuffer is a thread-safe fixed-capacity LIFO ring buffer of PlanSummary
// entries. When full, the oldest entry is silently dropped.
type PlanBuffer struct {
	mu    sync.Mutex
	cap   int
	items []PlanSummary
}

var (
	globalPlanBufOnce sync.Once
	globalPlanBuf     *PlanBuffer
)

// GlobalPlanBuffer returns the process-wide singleton PlanBuffer.
func GlobalPlanBuffer() *PlanBuffer {
	globalPlanBufOnce.Do(func() {
		globalPlanBuf = NewPlanBuffer(defaultPlanBufferCap)
	})
	return globalPlanBuf
}

// NewPlanBuffer creates a PlanBuffer with the given capacity (clamped to
// [1, maxPlanBufferCap]).
func NewPlanBuffer(cap int) *PlanBuffer {
	if cap < 1 {
		cap = defaultPlanBufferCap
	}
	if cap > maxPlanBufferCap {
		cap = maxPlanBufferCap
	}
	return &PlanBuffer{cap: cap, items: make([]PlanSummary, 0, cap)}
}

// Push inserts a new entry at the front (newest first). If the buffer is at
// capacity, the oldest entry (last element) is dropped.
func (b *PlanBuffer) Push(plan ContextPlan) {
	if b == nil {
		return
	}
	summary := PlanSummary{
		PlanID:              plan.PlanID,
		CapturedAt:          time.Now().Unix(),
		SegmentsIncluded:    len(plan.SegmentsIncluded),
		SegmentsTrimmed:     len(plan.SegmentsTrimmed),
		TokenBudgetUsed:     plan.TokenBudget.Used,
		TokenBudgetLimit:    plan.TokenBudget.Budget,
		TokenBudgetOverflow: plan.TokenBudget.Overflow,
		Warnings:            plan.Warnings,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append([]PlanSummary{summary}, b.items...)
	if len(b.items) > b.cap {
		b.items = b.items[:b.cap]
	}
}

// Snapshot returns a copy of all stored summaries, newest first.
func (b *PlanBuffer) Snapshot() []PlanSummary {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]PlanSummary, len(b.items))
	copy(out, b.items)
	return out
}

// Len returns the number of stored entries.
func (b *PlanBuffer) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// Cap returns the configured capacity.
func (b *PlanBuffer) Cap() int {
	if b == nil {
		return 0
	}
	return b.cap
}

// Clear removes all stored entries.
func (b *PlanBuffer) Clear() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = b.items[:0]
}
