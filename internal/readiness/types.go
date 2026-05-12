package readiness

import "time"

type GateResultValue string

const (
	GatePass    GateResultValue = "pass"
	GateFail    GateResultValue = "fail"
	GateSkip    GateResultValue = "skip"
	GateUnknown GateResultValue = "unknown"
)

type SourceStatus string

const (
	SourcePass    SourceStatus = "pass"
	SourceFail    SourceStatus = "fail"
	SourcePending SourceStatus = "pending"
)

type FeatureMode string

const (
	ModeOff     FeatureMode = "off"
	ModeShadow  FeatureMode = "shadow"
	ModeEnforce FeatureMode = "enforce"
)

type FeatureDecision string

const (
	FeatureHold    FeatureDecision = "hold"
	FeaturePromote FeatureDecision = "promote"
	FeatureDisable FeatureDecision = "disable"
)

type ReleaseDecisionValue string

const (
	DecisionGo           ReleaseDecisionValue = "GO"
	DecisionNoGo         ReleaseDecisionValue = "NO-GO"
	DecisionGoFlagsOff   ReleaseDecisionValue = "GO-WITH-FLAGS-OFF"
	DecisionUndetermined ReleaseDecisionValue = "UNDETERMINED"
)

type Report struct {
	Version       string             `json:"version,omitempty"`
	Branch        string             `json:"branch,omitempty"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Scope         string             `json:"scope,omitempty"`
	DecisionOwner string             `json:"decision_owner,omitempty"`
	Gates         []GateResult       `json:"gates,omitempty"`
	Features      []FeatureReadiness `json:"features,omitempty"`
	Analyzer      []AnalyzerFinding  `json:"analyzer,omitempty"`
	Shadow        []ShadowEvidence   `json:"shadow,omitempty"`
	Decision      ReleaseDecision    `json:"decision"`
	FollowUps     []FollowUp         `json:"follow_ups,omitempty"`
	Metadata      map[string]string  `json:"metadata,omitempty"`
}

type GateResult struct {
	Name     string          `json:"name"`
	Result   GateResultValue `json:"result"`
	Evidence string          `json:"evidence,omitempty"`
	Notes    string          `json:"notes,omitempty"`
}

type FeatureReadiness struct {
	Feature         string          `json:"feature"`
	CurrentMode     FeatureMode     `json:"current_mode"`
	TargetMode      FeatureMode     `json:"target_mode"`
	Decision        FeatureDecision `json:"decision"`
	Evidence        []EvidenceRef   `json:"evidence,omitempty"`
	MissingEvidence []string        `json:"missing_evidence,omitempty"`
	Reason          string          `json:"reason,omitempty"`
}

type EvidenceRef struct {
	Source  string `json:"source"`
	Ref     string `json:"ref,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type AnalyzerFinding struct {
	Category string `json:"category"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Warning  int    `json:"warning"`
	TopRule  string `json:"top_rule,omitempty"`
}

type ShadowEvidence struct {
	Source  string       `json:"source"`
	Status  SourceStatus `json:"status"`
	Samples int          `json:"samples,omitempty"`
	Summary string       `json:"summary,omitempty"`
}

type ReleaseDecision struct {
	Decision ReleaseDecisionValue `json:"decision"`
	Reason   string               `json:"reason,omitempty"`
}

type FollowUp struct {
	Owner  string `json:"owner,omitempty"`
	Action string `json:"action"`
	Due    string `json:"due,omitempty"`
}
