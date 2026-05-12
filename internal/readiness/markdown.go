package readiness

import (
	"fmt"
	"strings"
	"time"
)

func RenderMarkdown(report Report) string {
	var b strings.Builder

	writeLine(&b, "# Release Readiness Report")
	writeLine(&b, "")
	writeLine(&b, "Version / Branch: %s / %s", display(report.Version), display(report.Branch))
	writeLine(&b, "Generated at: %s", formatTime(report.GeneratedAt))
	writeLine(&b, "Scope: %s", display(report.Scope))
	writeLine(&b, "Decision owner: %s", display(report.DecisionOwner))
	writeLine(&b, "")

	renderGateSummary(&b, report.Gates)
	renderFeatureReadiness(&b, report.Features)
	renderAnalyzerFindings(&b, report.Analyzer)
	renderShadowEvidence(&b, report.Shadow)
	renderDecision(&b, report.Decision, report.FollowUps)

	return b.String()
}

func renderGateSummary(b *strings.Builder, gates []GateResult) {
	writeLine(b, "## Gate Summary")
	writeLine(b, "")
	writeLine(b, "| Gate | Result | Evidence | Notes |")
	writeLine(b, "|---|---|---|---|")
	if len(gates) == 0 {
		writeLine(b, "| _none_ | unknown | | |")
		writeLine(b, "")
		return
	}
	for _, gate := range gates {
		writeLine(b, "| %s | %s | %s | %s |", cell(gate.Name), cell(string(gate.Result)), cell(gate.Evidence), cell(gate.Notes))
	}
	writeLine(b, "")
}

func renderFeatureReadiness(b *strings.Builder, features []FeatureReadiness) {
	writeLine(b, "## Feature Flag Readiness")
	writeLine(b, "")
	writeLine(b, "| Feature | Current | Target | Decision | Evidence | Missing Evidence |")
	writeLine(b, "|---|---|---|---|---|---|")
	if len(features) == 0 {
		writeLine(b, "| _none_ | off | off | hold | | |")
		writeLine(b, "")
		return
	}
	for _, feature := range features {
		writeLine(
			b,
			"| %s | %s | %s | %s | %s | %s |",
			cell(feature.Feature),
			cell(string(feature.CurrentMode)),
			cell(string(feature.TargetMode)),
			cell(string(feature.Decision)),
			cell(renderEvidence(feature.Evidence, feature.Reason)),
			cell(strings.Join(feature.MissingEvidence, "; ")),
		)
	}
	writeLine(b, "")
}

func renderAnalyzerFindings(b *strings.Builder, findings []AnalyzerFinding) {
	writeLine(b, "## Analyzer Findings")
	writeLine(b, "")
	writeLine(b, "| Category | Critical | High | Warning | Top rule |")
	writeLine(b, "|---|---|---|---|---|")
	if len(findings) == 0 {
		writeLine(b, "| _none_ | 0 | 0 | 0 | |")
		writeLine(b, "")
		return
	}
	for _, finding := range findings {
		writeLine(
			b,
			"| %s | %d | %d | %d | %s |",
			cell(finding.Category),
			finding.Critical,
			finding.High,
			finding.Warning,
			cell(finding.TopRule),
		)
	}
	writeLine(b, "")
}

func renderShadowEvidence(b *strings.Builder, shadow []ShadowEvidence) {
	writeLine(b, "## Shadow Evidence")
	writeLine(b, "")
	writeLine(b, "| Source | Status | Samples | Summary |")
	writeLine(b, "|---|---|---|---|")
	if len(shadow) == 0 {
		writeLine(b, "| _none_ | pending | 0 | |")
		writeLine(b, "")
		return
	}
	for _, evidence := range shadow {
		writeLine(
			b,
			"| %s | %s | %d | %s |",
			cell(evidence.Source),
			cell(string(evidence.Status)),
			evidence.Samples,
			cell(evidence.Summary),
		)
	}
	writeLine(b, "")
}

func renderDecision(b *strings.Builder, decision ReleaseDecision, followUps []FollowUp) {
	value := decision.Decision
	if value == "" {
		value = DecisionUndetermined
	}

	writeLine(b, "## Release Decision")
	writeLine(b, "")
	writeLine(b, "Decision: %s", value)
	writeLine(b, "Reason: %s", display(decision.Reason))
	writeLine(b, "Required follow-ups:")
	if len(followUps) == 0 {
		writeLine(b, "- None")
		return
	}
	for _, item := range followUps {
		writeLine(b, "- %s", renderFollowUp(item))
	}
}

func renderEvidence(evidence []EvidenceRef, reason string) string {
	parts := make([]string, 0, len(evidence)+1)
	if reason != "" {
		parts = append(parts, reason)
	}
	for _, item := range evidence {
		piece := item.Source
		if item.Ref != "" {
			piece += " " + item.Ref
		}
		if item.Summary != "" {
			piece += ": " + item.Summary
		}
		parts = append(parts, piece)
	}
	return strings.Join(parts, "; ")
}

func renderFollowUp(item FollowUp) string {
	parts := []string{item.Action}
	if item.Owner != "" {
		parts = append(parts, "owner: "+item.Owner)
	}
	if item.Due != "" {
		parts = append(parts, "due: "+item.Due)
	}
	return strings.Join(parts, "; ")
}

func writeLine(b *strings.Builder, format string, args ...any) {
	if len(args) == 0 {
		b.WriteString(format)
	} else {
		_, _ = fmt.Fprintf(b, format, args...)
	}
	b.WriteByte('\n')
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func display(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func cell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
