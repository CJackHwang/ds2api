package historyanalyzer

import (
	"strings"
	"testing"
	"time"
)

func TestRenderMarkdownIncludesSummaryFindingsAndCandidates(t *testing.T) {
	report := Report{
		GeneratedAt: time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC),
		Scope:       ReportScope{Name: "unit"},
		Summary: Summary{
			TotalRecords:  1,
			TotalFindings: 1,
			ByCategory: []CategorySummary{
				{
					Category: CategoryTool,
					Counts:   SeverityCounts{High: 1},
					TopRule:  RuleToolMarkerLeak,
				},
			},
		},
		Findings: []Finding{
			{
				RuleID:    RuleToolMarkerLeak,
				Category:  CategoryTool,
				Severity:  SeverityHigh,
				RequestID: "req-1",
				Evidence: []Evidence{
					{Source: "chat_history", Field: "content", Excerpt: "Bearer <redacted>", Note: "visible marker"},
				},
				SuggestedAction: "Add the sample to the parser leak corpus.",
				FixtureHint: &FixtureHint{
					Suite:       "toolparser/leak",
					Name:        "tool-marker-leak",
					SourceRefs:  []string{"history.json"},
					NeedsReview: true,
				},
			},
		},
		Readiness: &ReadinessSummary{Decision: "NO-GO", BlockingFindings: 1},
	}

	md := RenderMarkdown(report)
	for _, fragment := range []string{
		"# History Analyzer Report",
		"HA_TOOL_MARKER_LEAK",
		"toolparser/leak",
		"NO-GO",
		"&lt;redacted&gt;",
	} {
		if !strings.Contains(md, fragment) {
			t.Fatalf("markdown missing %q:\n%s", fragment, md)
		}
	}
	if strings.Contains(md, "abcdefghijklmnop") {
		t.Fatalf("markdown leaked raw secret:\n%s", md)
	}
}

func TestFixtureCandidatesReturnsReviewableFindingsOnly(t *testing.T) {
	report := Report{
		Findings: []Finding{
			{
				RuleID:    RuleToolMarkerLeak,
				Category:  CategoryTool,
				Severity:  SeverityHigh,
				RequestID: "with-hint",
				FixtureHint: &FixtureHint{
					Suite:      "toolparser/leak",
					Name:       "marker",
					SourceRefs: []string{"history.json"},
				},
			},
			{RuleID: RuleAccountRetryRecovered, Category: CategoryAccountRuntime, Severity: SeverityInfo},
		},
	}

	candidates := FixtureCandidates(report)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one", candidates)
	}
	if candidates[0].RuleID != RuleToolMarkerLeak || candidates[0].RequestID != "with-hint" {
		t.Fatalf("candidate mismatch: %#v", candidates[0])
	}
}

func TestBuildReadinessSummary(t *testing.T) {
	if got := BuildReadinessSummary(nil); got.Decision != "GO" {
		t.Fatalf("decision = %q, want GO", got.Decision)
	}
	if got := BuildReadinessSummary([]Finding{{Severity: SeverityWarning}}); got.Decision != "REVIEW" {
		t.Fatalf("decision = %q, want REVIEW", got.Decision)
	}
	if got := BuildReadinessSummary([]Finding{{Severity: SeverityInfo}}); got.Decision != "GO" || strings.Contains(got.Reasons[0], "no analyzer findings") {
		t.Fatalf("summary = %#v, want GO without zero-findings wording", got)
	}
	if got := BuildReadinessSummary([]Finding{{Severity: SeverityHigh}}); got.Decision != "NO-GO" || got.BlockingFindings != 1 {
		t.Fatalf("summary = %#v, want NO-GO with one blocker", got)
	}
}
