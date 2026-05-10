package contextengine

// SegmentType classifies a context segment by its conversational role.
type SegmentType string

const (
	SegSystem           SegmentType = "system"
	SegUser             SegmentType = "user"
	SegAssistant        SegmentType = "assistant"
	SegToolCall         SegmentType = "tool_call"
	SegToolResult       SegmentType = "tool_result"
	SegFileSnapshot     SegmentType = "file_snapshot"
	SegReasoningSummary SegmentType = "reasoning_summary"
)

// ContextSegment represents one logical unit of context derived from a message.
type ContextSegment struct {
	ID       string
	Type     SegmentType
	Source   string // "request" | "history" | "tool" | "file"
	Priority int
	// TokenCost is an estimated token count for this segment's content.
	TokenCost int
	// Digest is the SHA-256 hex digest of Content (used for dedup / cache).
	Digest   string
	Content  string
	Metadata map[string]any
}

// TrimmedSegment records a segment that was excluded from the plan and why.
type TrimmedSegment struct {
	ID     string
	Type   SegmentType
	Reason string // e.g. "orphan_tool_call" | "token_budget"
}

// TokenBudgetReport summarises how the budget was consumed.
type TokenBudgetReport struct {
	Budget   int
	Used     int
	Overflow bool
}

// ContextPlan is the output of Compile: the set of segments to include,
// those trimmed, a budget report, and any diagnostic warnings.
type ContextPlan struct {
	PlanID           string
	SegmentsIncluded []ContextSegment
	SegmentsTrimmed  []TrimmedSegment
	TokenBudget      TokenBudgetReport
	ReusedFiles      []string // digest refs kept for future dedup
	Warnings         []string
}
