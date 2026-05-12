package readiness

import (
	"testing"
	"time"
)

func TestBuildBaselineReportDefaultsPendingEvidence(t *testing.T) {
	report := BuildBaselineReport(BaselineOptions{
		Version:     "m4.0-test",
		Branch:      "feat/m4-readiness-cli",
		GeneratedAt: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		Scope:       "baseline",
	})

	if report.Decision.Decision != DecisionGoFlagsOff {
		t.Fatalf("decision = %q, want %q", report.Decision.Decision, DecisionGoFlagsOff)
	}
	if len(report.Features) != 4 {
		t.Fatalf("features = %d, want 4", len(report.Features))
	}
	if len(report.Shadow) != 5 {
		t.Fatalf("shadow evidence = %d, want 5", len(report.Shadow))
	}
	for _, shadow := range report.Shadow {
		if shadow.Status != SourcePending {
			t.Fatalf("shadow %q status = %q, want pending", shadow.Source, shadow.Status)
		}
	}
	if len(report.FollowUps) != len(report.Shadow) {
		t.Fatalf("followups = %d, want %d", len(report.FollowUps), len(report.Shadow))
	}
}

func TestBuildBaselineReportMarksProvidedShadowInput(t *testing.T) {
	report := BuildBaselineReport(BaselineOptions{
		Gates: DefaultGateResults(),
		ShadowInputs: []ShadowInput{
			{Source: "history analyzer", Path: "artifacts/history/report.json"},
		},
	})

	var found bool
	for _, shadow := range report.Shadow {
		if shadow.Source != "history analyzer" {
			continue
		}
		found = true
		if shadow.Status != SourcePass {
			t.Fatalf("history analyzer status = %q, want pass", shadow.Status)
		}
		if shadow.Summary == "" {
			t.Fatal("history analyzer summary is empty")
		}
	}
	if !found {
		t.Fatal("history analyzer shadow evidence not found")
	}
	if len(report.FollowUps) != 4 {
		t.Fatalf("followups = %d, want 4", len(report.FollowUps))
	}
}

func TestBuildBaselineReportFailsOnFailedGate(t *testing.T) {
	report := BuildBaselineReport(BaselineOptions{
		Gates: []GateResult{
			{Name: "lint", Result: GatePass},
			{Name: "unit all", Result: GateFail},
		},
	})

	if report.Decision.Decision != DecisionNoGo {
		t.Fatalf("decision = %q, want %q", report.Decision.Decision, DecisionNoGo)
	}
}
