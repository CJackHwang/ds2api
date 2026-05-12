package readiness

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSampleReportJSONRoundTrip(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/readiness/sample-report.json")
	if err != nil {
		t.Fatal(err)
	}

	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}

	if report.Decision.Decision != DecisionGoFlagsOff {
		t.Fatalf("decision = %q, want %q", report.Decision.Decision, DecisionGoFlagsOff)
	}
	if len(report.Features) != 2 {
		t.Fatalf("features = %d, want 2", len(report.Features))
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "token") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("sample report contains sensitive-looking field: %s", encoded)
	}
}

func TestRenderMarkdown(t *testing.T) {
	report := Report{
		Version:       "m4.0-test",
		Branch:        "feat/m4-readiness-model",
		GeneratedAt:   time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		Scope:         "unit test",
		DecisionOwner: "release owner",
		Gates: []GateResult{
			{Name: "lint", Result: GatePass, Evidence: "./scripts/lint.sh"},
			{Name: "live", Result: GateSkip, Evidence: "no credentials"},
		},
		Features: []FeatureReadiness{
			{
				Feature:         "parser_v2",
				CurrentMode:     ModeOff,
				TargetMode:      ModeShadow,
				Decision:        FeatureHold,
				Reason:          "waiting for shadow data",
				MissingEvidence: []string{"parser shadow report", "manual diff review"},
			},
		},
		Analyzer: []AnalyzerFinding{
			{Category: "tool", Critical: 0, High: 1, Warning: 2, TopRule: "HA_TOOL_MARKER_LEAK"},
		},
		Shadow: []ShadowEvidence{
			{Source: "history analyzer", Status: SourcePending, Samples: 0, Summary: "M4.1 pending"},
		},
		Decision: ReleaseDecision{
			Decision: DecisionGoFlagsOff,
			Reason:   "baseline only",
		},
		FollowUps: []FollowUp{
			{Owner: "M4.1", Action: "wire analyzer summary"},
		},
	}

	got := RenderMarkdown(report)
	wantFragments := []string{
		"# Release Readiness Report",
		"Generated at: 2026-05-12T00:00:00Z",
		"| lint | pass | ./scripts/lint.sh |",
		"| parser_v2 | off | shadow | hold | waiting for shadow data | parser shadow report; manual diff review |",
		"| tool | 0 | 1 | 2 | HA_TOOL_MARKER_LEAK |",
		"| history analyzer | pending | 0 | M4.1 pending |",
		"Decision: GO-WITH-FLAGS-OFF",
		"- wire analyzer summary; owner: M4.1",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Fatalf("rendered markdown missing %q:\n%s", fragment, got)
		}
	}
}

func TestRenderMarkdownEscapesTables(t *testing.T) {
	report := Report{
		Gates: []GateResult{
			{Name: "lint|unit", Result: GatePass, Evidence: "line one\nline two"},
		},
		Decision: ReleaseDecision{Decision: DecisionUndetermined},
	}

	got := RenderMarkdown(report)
	if !strings.Contains(got, "lint\\|unit") {
		t.Fatalf("table pipe was not escaped:\n%s", got)
	}
	if !strings.Contains(got, "line one<br>line two") {
		t.Fatalf("table newline was not normalized:\n%s", got)
	}
}
