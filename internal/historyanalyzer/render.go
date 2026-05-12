package historyanalyzer

import (
	"fmt"
	"strings"
	"time"
)

type FixtureCandidate struct {
	RuleID          RuleID     `json:"rule_id"`
	Category        Category   `json:"category"`
	Severity        Severity   `json:"severity"`
	RequestID       string     `json:"request_id,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	Suite           string     `json:"suite,omitempty"`
	Name            string     `json:"name,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	SourceRefs      []string   `json:"source_refs,omitempty"`
	Evidence        []Evidence `json:"evidence,omitempty"`
	SuggestedAction string     `json:"suggested_action,omitempty"`
}

func RenderMarkdown(report Report) string {
	var b strings.Builder

	writeMarkdownLine(&b, "# History Analyzer Report")
	writeMarkdownLine(&b, "")
	writeMarkdownLine(&b, "Generated at: %s", markdownDisplay(formatMarkdownTime(report.GeneratedAt)))
	writeMarkdownLine(&b, "Scope: %s", markdownDisplay(report.Scope.Name))
	writeMarkdownLine(&b, "")

	renderMarkdownSummary(&b, report.Summary)
	renderMarkdownFindings(&b, report.Findings)
	renderMarkdownFixtureCandidates(&b, FixtureCandidates(report))
	renderMarkdownReadiness(&b, report.Readiness)

	return b.String()
}

func FixtureCandidates(report Report) []FixtureCandidate {
	out := make([]FixtureCandidate, 0)
	for _, finding := range report.Findings {
		if finding.FixtureHint == nil {
			continue
		}
		out = append(out, FixtureCandidate{
			RuleID:          finding.RuleID,
			Category:        finding.Category,
			Severity:        finding.Severity,
			RequestID:       finding.RequestID,
			SessionID:       finding.SessionID,
			Suite:           finding.FixtureHint.Suite,
			Name:            finding.FixtureHint.Name,
			Reason:          finding.FixtureHint.Reason,
			SourceRefs:      cloneStrings(finding.FixtureHint.SourceRefs),
			Evidence:        cloneEvidence(finding.Evidence),
			SuggestedAction: finding.SuggestedAction,
		})
	}
	return out
}

func BuildReadinessSummary(findings []Finding) *ReadinessSummary {
	var infos int
	var blocking int
	var warnings int
	for _, finding := range findings {
		switch finding.Severity {
		case SeverityCritical, SeverityHigh:
			blocking++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}

	switch {
	case blocking > 0:
		return &ReadinessSummary{
			Decision:         "NO-GO",
			BlockingFindings: blocking,
			Reasons:          []string{fmt.Sprintf("%d high or critical analyzer findings require review", blocking)},
		}
	case warnings > 0:
		return &ReadinessSummary{
			Decision: "REVIEW",
			Reasons:  []string{fmt.Sprintf("%d warning analyzer findings require triage", warnings)},
		}
	case infos > 0:
		return &ReadinessSummary{
			Decision: "GO",
			Reasons:  []string{fmt.Sprintf("%d info analyzer findings detected; no blocking or warning findings", infos)},
		}
	default:
		return &ReadinessSummary{
			Decision: "GO",
			Reasons:  []string{"no blocking or warning analyzer findings detected"},
		}
	}
}

func renderMarkdownSummary(b *strings.Builder, summary Summary) {
	writeMarkdownLine(b, "## Summary")
	writeMarkdownLine(b, "")
	writeMarkdownLine(b, "Total records: %d", summary.TotalRecords)
	writeMarkdownLine(b, "Total findings: %d", summary.TotalFindings)
	writeMarkdownLine(b, "")
	writeMarkdownLine(b, "| Category | Info | Warning | High | Critical | Top rule |")
	writeMarkdownLine(b, "|---|---|---|---|---|---|")
	if len(summary.ByCategory) == 0 {
		writeMarkdownLine(b, "| _none_ | 0 | 0 | 0 | 0 | |")
		writeMarkdownLine(b, "")
		return
	}
	for _, item := range summary.ByCategory {
		writeMarkdownLine(
			b,
			"| %s | %d | %d | %d | %d | %s |",
			markdownCell(string(item.Category)),
			item.Counts.Info,
			item.Counts.Warning,
			item.Counts.High,
			item.Counts.Critical,
			markdownCell(string(item.TopRule)),
		)
	}
	writeMarkdownLine(b, "")
}

func renderMarkdownFindings(b *strings.Builder, findings []Finding) {
	writeMarkdownLine(b, "## Findings")
	writeMarkdownLine(b, "")
	writeMarkdownLine(b, "| Severity | Category | Rule | Request | Evidence | Suggested action |")
	writeMarkdownLine(b, "|---|---|---|---|---|---|")
	if len(findings) == 0 {
		writeMarkdownLine(b, "| _none_ | | | | | |")
		writeMarkdownLine(b, "")
		return
	}
	for _, finding := range findings {
		writeMarkdownLine(
			b,
			"| %s | %s | %s | %s | %s | %s |",
			markdownCell(string(finding.Severity)),
			markdownCell(string(finding.Category)),
			markdownCell(string(finding.RuleID)),
			markdownCell(finding.RequestID),
			markdownCell(renderEvidenceSummary(finding.Evidence)),
			markdownCell(finding.SuggestedAction),
		)
	}
	writeMarkdownLine(b, "")
}

func renderMarkdownFixtureCandidates(b *strings.Builder, candidates []FixtureCandidate) {
	writeMarkdownLine(b, "## Fixture Candidates")
	writeMarkdownLine(b, "")
	writeMarkdownLine(b, "| Suite | Name | Rule | Request | Source refs |")
	writeMarkdownLine(b, "|---|---|---|---|---|")
	if len(candidates) == 0 {
		writeMarkdownLine(b, "| _none_ | | | | |")
		writeMarkdownLine(b, "")
		return
	}
	for _, candidate := range candidates {
		writeMarkdownLine(
			b,
			"| %s | %s | %s | %s | %s |",
			markdownCell(candidate.Suite),
			markdownCell(candidate.Name),
			markdownCell(string(candidate.RuleID)),
			markdownCell(candidate.RequestID),
			markdownCell(strings.Join(candidate.SourceRefs, "; ")),
		)
	}
	writeMarkdownLine(b, "")
}

func renderMarkdownReadiness(b *strings.Builder, readiness *ReadinessSummary) {
	writeMarkdownLine(b, "## Readiness")
	writeMarkdownLine(b, "")
	if readiness == nil {
		writeMarkdownLine(b, "Decision: UNKNOWN")
		writeMarkdownLine(b, "Blocking findings: 0")
		writeMarkdownLine(b, "Reasons: ")
		return
	}
	writeMarkdownLine(b, "Decision: %s", markdownDisplay(readiness.Decision))
	writeMarkdownLine(b, "Blocking findings: %d", readiness.BlockingFindings)
	writeMarkdownLine(b, "Reasons: %s", markdownDisplay(strings.Join(readiness.Reasons, "; ")))
}

func renderEvidenceSummary(evidence []Evidence) string {
	parts := make([]string, 0, len(evidence))
	for _, item := range evidence {
		piece := item.Source
		if item.Field != "" {
			if piece != "" {
				piece += "."
			}
			piece += item.Field
		}
		if item.Excerpt != "" {
			if piece != "" {
				piece += ": "
			}
			piece += item.Excerpt
		}
		if item.Note != "" {
			if piece != "" {
				piece += " "
			}
			piece += "(" + item.Note + ")"
		}
		parts = append(parts, piece)
	}
	return strings.Join(parts, "; ")
}

func writeMarkdownLine(b *strings.Builder, format string, args ...any) {
	if len(args) == 0 {
		b.WriteString(format)
	} else {
		_, _ = fmt.Fprintf(b, format, args...)
	}
	b.WriteByte('\n')
}

func formatMarkdownTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func markdownDisplay(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func markdownCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func cloneEvidence(in []Evidence) []Evidence {
	if len(in) == 0 {
		return nil
	}
	out := make([]Evidence, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
