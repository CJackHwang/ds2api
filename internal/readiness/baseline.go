package readiness

import "time"

type BaselineOptions struct {
	Version       string
	Branch        string
	GeneratedAt   time.Time
	Scope         string
	DecisionOwner string
	Gates         []GateResult
	ShadowInputs  []ShadowInput
}

type ShadowInput struct {
	Source string
	Path   string
}

func BuildBaselineReport(opts BaselineOptions) Report {
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	gates := opts.Gates
	if len(gates) == 0 {
		gates = DefaultGateResults()
	}

	shadow := defaultShadowEvidence()
	for _, input := range opts.ShadowInputs {
		if input.Source == "" || input.Path == "" {
			continue
		}
		markShadowInput(shadow, input.Source, input.Path)
	}

	decision, reason := baselineDecision(gates)

	return Report{
		Version:       opts.Version,
		Branch:        opts.Branch,
		GeneratedAt:   generatedAt,
		Scope:         opts.Scope,
		DecisionOwner: opts.DecisionOwner,
		Gates:         gates,
		Features:      defaultFeatureReadiness(),
		Analyzer:      defaultAnalyzerFindings(),
		Shadow:        shadow,
		Decision: ReleaseDecision{
			Decision: decision,
			Reason:   reason,
		},
		FollowUps: defaultFollowUps(shadow),
		Metadata: map[string]string{
			"schema": "readiness/v1",
		},
	}
}

func DefaultGateResults() []GateResult {
	return []GateResult{
		{Name: "lint", Result: GateUnknown, Evidence: "./scripts/lint.sh"},
		{Name: "refactor line gate", Result: GateUnknown, Evidence: "./tests/scripts/check-refactor-line-gate.sh"},
		{Name: "unit all", Result: GateUnknown, Evidence: "./tests/scripts/run-unit-all.sh"},
		{Name: "webui build", Result: GateUnknown, Evidence: "npm run build --prefix webui"},
		{Name: "live", Result: GateSkip, Evidence: "not required unless high-risk live path changes"},
	}
}

func defaultFeatureReadiness() []FeatureReadiness {
	return []FeatureReadiness{
		{
			Feature:         "parser_v2",
			CurrentMode:     ModeOff,
			TargetMode:      ModeShadow,
			Decision:        FeatureHold,
			Reason:          "waiting for parser shadow evidence",
			MissingEvidence: []string{"parser shadow report", "manual diff review"},
		},
		{
			Feature:         "context_engine",
			CurrentMode:     ModeOff,
			TargetMode:      ModeShadow,
			Decision:        FeatureHold,
			Reason:          "waiting for context shadow evidence",
			MissingEvidence: []string{"context shadow report", "tool pair review"},
		},
		{
			Feature:         "auto_continue",
			CurrentMode:     ModeOff,
			TargetMode:      ModeShadow,
			Decision:        FeatureHold,
			Reason:          "waiting for detector shadow traces",
			MissingEvidence: []string{"auto continue shadow traces", "non-stream smoke"},
		},
		{
			Feature:         "capability_router",
			CurrentMode:     ModeOff,
			TargetMode:      ModeShadow,
			Decision:        FeatureHold,
			Reason:          "waiting for capability profile and conflict samples",
			MissingEvidence: []string{"capability matrix", "conflict warning samples"},
		},
	}
}

func defaultAnalyzerFindings() []AnalyzerFinding {
	return []AnalyzerFinding{
		{Category: "tool", TopRule: "pending"},
		{Category: "context", TopRule: "pending"},
		{Category: "continue", TopRule: "pending"},
		{Category: "capability", TopRule: "pending"},
		{Category: "account_runtime", TopRule: "pending"},
	}
}

func defaultShadowEvidence() []ShadowEvidence {
	return []ShadowEvidence{
		{Source: "history analyzer", Status: SourcePending, Summary: "M4.1 will provide analyzer findings"},
		{Source: "parser shadow", Status: SourcePending, Summary: "M4.2 will provide parser shadow data"},
		{Source: "context shadow", Status: SourcePending, Summary: "M4.2 will provide context shadow data"},
		{Source: "auto continue shadow", Status: SourcePending, Summary: "M4.3 will provide detector traces"},
		{Source: "capability router shadow", Status: SourcePending, Summary: "M4.4 will provide conflict warnings"},
	}
}

func markShadowInput(shadow []ShadowEvidence, source, path string) {
	for i := range shadow {
		if shadow[i].Source == source {
			shadow[i].Status = SourcePass
			shadow[i].Summary = "input file provided: " + path
			return
		}
	}
}

func baselineDecision(gates []GateResult) (ReleaseDecisionValue, string) {
	hasUnknown := false
	for _, gate := range gates {
		switch gate.Result {
		case GateFail:
			return DecisionNoGo, "one or more required local gates failed"
		case GateUnknown:
			hasUnknown = true
		}
	}
	if hasUnknown {
		return DecisionGoFlagsOff, "some local gates are unknown; keep high-risk features off or shadow-only"
	}
	return DecisionGoFlagsOff, "baseline can ship while high-risk features remain off or shadow-only"
}

func defaultFollowUps(shadow []ShadowEvidence) []FollowUp {
	followUps := make([]FollowUp, 0, len(shadow))
	for _, item := range shadow {
		if item.Status != SourcePending {
			continue
		}
		followUps = append(followUps, FollowUp{
			Owner:  item.Source,
			Action: "provide " + item.Source + " evidence for future readiness decisions",
		})
	}
	return followUps
}
