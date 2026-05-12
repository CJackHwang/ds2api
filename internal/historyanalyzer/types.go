package historyanalyzer

import "time"

type Category string

const (
	CategoryTool           Category = "tool"
	CategoryContext        Category = "context"
	CategoryContinue       Category = "continue"
	CategoryCapability     Category = "capability"
	CategoryAccountRuntime Category = "account_runtime"
	CategoryProtocol       Category = "protocol"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type RuleID string

type SourceRef struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
	ID   string `json:"id,omitempty"`
	Note string `json:"note,omitempty"`
}

type TextSnapshot struct {
	Value string `json:"value,omitempty"`
	Hash  string `json:"hash,omitempty"`
}

type RuntimeMetrics struct {
	ElapsedMs     int64          `json:"elapsed_ms,omitempty"`
	TTFTMs        int64          `json:"ttft_ms,omitempty"`
	RetryCount    int            `json:"retry_count,omitempty"`
	AccountSwitch int            `json:"account_switch,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

type AnalysisRecord struct {
	RequestID     string                  `json:"request_id,omitempty"`
	SessionID     string                  `json:"session_id,omitempty"`
	CreatedAt     time.Time               `json:"created_at,omitempty"`
	Surface       string                  `json:"surface,omitempty"`
	Protocol      string                  `json:"protocol,omitempty"`
	Model         string                  `json:"model,omitempty"`
	ResolvedModel string                  `json:"resolved_model,omitempty"`
	Stream        bool                    `json:"stream,omitempty"`
	Status        string                  `json:"status,omitempty"`
	StatusCode    int                     `json:"status_code,omitempty"`
	FinishReason  string                  `json:"finish_reason,omitempty"`
	Text          map[string]string       `json:"text,omitempty"`
	Snapshots     map[string]TextSnapshot `json:"snapshots,omitempty"`
	Flags         map[string]string       `json:"flags,omitempty"`
	Metrics       RuntimeMetrics          `json:"metrics,omitempty"`
	Sources       []SourceRef             `json:"sources,omitempty"`
}

type Evidence struct {
	Source  string `json:"source"`
	Field   string `json:"field,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Note    string `json:"note,omitempty"`
}

type FixtureHint struct {
	Suite       string   `json:"suite,omitempty"`
	Name        string   `json:"name,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	SourceRefs  []string `json:"source_refs,omitempty"`
	NeedsReview bool     `json:"needs_review,omitempty"`
}

type Finding struct {
	RuleID          RuleID       `json:"rule_id"`
	Category        Category     `json:"category"`
	Severity        Severity     `json:"severity"`
	RequestID       string       `json:"request_id,omitempty"`
	SessionID       string       `json:"session_id,omitempty"`
	Evidence        []Evidence   `json:"evidence,omitempty"`
	SuggestedAction string       `json:"suggested_action,omitempty"`
	FixtureHint     *FixtureHint `json:"fixture_hint,omitempty"`
}

type ReportScope struct {
	Name      string            `json:"name,omitempty"`
	Sources   []SourceRef       `json:"sources,omitempty"`
	StartedAt time.Time         `json:"started_at,omitempty"`
	EndedAt   time.Time         `json:"ended_at,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
}

type SeverityCounts struct {
	Info     int `json:"info,omitempty"`
	Warning  int `json:"warning,omitempty"`
	High     int `json:"high,omitempty"`
	Critical int `json:"critical,omitempty"`
}

type CategorySummary struct {
	Category Category       `json:"category"`
	Counts   SeverityCounts `json:"counts"`
	TopRule  RuleID         `json:"top_rule,omitempty"`
}

type Summary struct {
	TotalRecords  int               `json:"total_records"`
	TotalFindings int               `json:"total_findings"`
	ByCategory    []CategorySummary `json:"by_category,omitempty"`
}

type ReadinessSummary struct {
	Decision         string   `json:"decision,omitempty"`
	BlockingFindings int      `json:"blocking_findings,omitempty"`
	Reasons          []string `json:"reasons,omitempty"`
}

type Report struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Scope       ReportScope       `json:"scope"`
	Summary     Summary           `json:"summary"`
	Findings    []Finding         `json:"findings,omitempty"`
	Readiness   *ReadinessSummary `json:"readiness,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
