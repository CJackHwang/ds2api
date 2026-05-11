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
	PlanID              string   `json:"plan_id"`
	CapturedAt          int64    `json:"captured_at"`
	SegmentsIncluded    int      `json:"segments_included"`
	SegmentsTrimmed     int      `json:"segments_trimmed"`
	TokenBudgetUsed     int      `json:"token_budget_used"`
	TokenBudgetLimit    int      `json:"token_budget_limit"`
	TokenBudgetOverflow bool     `json:"token_budget_overflow"`
	Warnings            []string `json:"warnings,omitempty"`
}

// PlanBuffer is a thread-safe fixed-capacity ring buffer of PlanSummary
// entries. Push is O(1). When full, the oldest entry is silently overwritten.
// Snapshot and Len hold a read lock and do not block concurrent reads.
type PlanBuffer struct {
	mu     sync.RWMutex
	bufcap int // fixed capacity; immutable after construction
	buf    []PlanSummary
	head   int // index of the next-write slot (wraps around)
	count  int // number of valid entries (0 ≤ count ≤ bufcap)
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
	return &PlanBuffer{bufcap: cap, buf: make([]PlanSummary, cap)}
}

// Push inserts a new entry. O(1): writes directly into the ring slot and
// advances head. When the buffer is full the oldest entry is overwritten.
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
	b.buf[b.head] = summary
	b.head = (b.head + 1) % b.bufcap
	if b.count < b.bufcap {
		b.count++
	}
}

// Snapshot returns a copy of all stored summaries, newest first.
func (b *PlanBuffer) Snapshot() []PlanSummary {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.count == 0 {
		return []PlanSummary{}
	}
	out := make([]PlanSummary, b.count)
	for i := 0; i < b.count; i++ {
		// head points to the next-write slot, so (head-1) is the newest entry.
		idx := ((b.head-1-i)%b.bufcap + b.bufcap) % b.bufcap
		out[i] = b.buf[idx]
	}
	return out
}

// Len returns the number of stored entries.
func (b *PlanBuffer) Len() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Cap returns the configured capacity.
func (b *PlanBuffer) Cap() int {
	if b == nil {
		return 0
	}
	return b.bufcap // immutable after construction; no lock needed
}

// Clear removes all stored entries.
func (b *PlanBuffer) Clear() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.count = 0
}
