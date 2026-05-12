package historyanalyzer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAnalyzerNormalizesFindingsAndSummarizes(t *testing.T) {
	rule := RuleFunc{
		RuleIDValue:   RuleToolMarkerLeak,
		CategoryValue: CategoryTool,
		Fn: func(record AnalysisRecord, redactor Redactor) []Finding {
			return []Finding{
				{
					Evidence: []Evidence{
						redactor.Evidence("chat_history", "content", record.Text["content"], "visible leak"),
					},
				},
			}
		},
	}
	analyzer := New(rule)

	report := analyzer.Analyze([]AnalysisRecord{
		{
			RequestID: "req-1",
			SessionID: "sess-1",
			Text: map[string]string{
				"content": "token leaked? Bearer abcdefghijklmnop",
			},
		},
	}, AnalyzeOptions{
		GeneratedAt: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		Scope:       ReportScope{Name: "unit"},
	})

	if report.Summary.TotalRecords != 1 {
		t.Fatalf("total records = %d, want 1", report.Summary.TotalRecords)
	}
	if report.Summary.TotalFindings != 1 {
		t.Fatalf("total findings = %d, want 1", report.Summary.TotalFindings)
	}
	if len(report.Summary.ByCategory) != 1 {
		t.Fatalf("category summaries = %d, want 1", len(report.Summary.ByCategory))
	}
	if report.Summary.ByCategory[0].Counts.High != 1 {
		t.Fatalf("high count = %d, want 1", report.Summary.ByCategory[0].Counts.High)
	}
	if report.Summary.ByCategory[0].TopRule != RuleToolMarkerLeak {
		t.Fatalf("top rule = %q, want %q", report.Summary.ByCategory[0].TopRule, RuleToolMarkerLeak)
	}

	finding := report.Findings[0]
	if finding.RuleID != RuleToolMarkerLeak {
		t.Fatalf("rule id = %q, want %q", finding.RuleID, RuleToolMarkerLeak)
	}
	if finding.Severity != SeverityHigh {
		t.Fatalf("severity = %q, want %q", finding.Severity, SeverityHigh)
	}
	if finding.RequestID != "req-1" || finding.SessionID != "sess-1" {
		t.Fatalf("request/session not propagated: %#v", finding)
	}
	if strings.Contains(finding.Evidence[0].Excerpt, "abcdefghijklmnop") {
		t.Fatalf("evidence leaked token: %s", finding.Evidence[0].Excerpt)
	}
	if finding.SuggestedAction == "" {
		t.Fatal("suggested action was not filled from rule spec")
	}
}

func TestAnalyzerSortsFindingsBySeverity(t *testing.T) {
	analyzer := New(RuleFunc{
		RuleIDValue:   RuleContinueCandidate,
		CategoryValue: CategoryContinue,
		Fn: func(record AnalysisRecord, redactor Redactor) []Finding {
			return []Finding{
				{RuleID: RuleContinueCandidate, Severity: SeverityInfo, RequestID: "b"},
				{RuleID: RuleToolMarkerLeak, Category: CategoryTool, Severity: SeverityCritical, RequestID: "a"},
			}
		},
	})

	report := analyzer.Analyze([]AnalysisRecord{{RequestID: "parent"}}, AnalyzeOptions{})
	if report.Findings[0].Severity != SeverityCritical {
		t.Fatalf("first severity = %q, want critical", report.Findings[0].Severity)
	}
}

func TestAnalyzerClonesScope(t *testing.T) {
	sources := []SourceRef{{Kind: "chat_history", Path: "history.json"}}
	filters := map[string]string{"surface": "chat"}
	report := New().Analyze(nil, AnalyzeOptions{
		Scope: ReportScope{
			Name:    "scope",
			Sources: sources,
			Filters: filters,
		},
	})

	sources[0].Path = "mutated.json"
	filters["surface"] = "mutated"

	if report.Scope.Sources[0].Path != "history.json" {
		t.Fatalf("scope source mutated: %#v", report.Scope.Sources[0])
	}
	if report.Scope.Filters["surface"] != "chat" {
		t.Fatalf("scope filter mutated: %#v", report.Scope.Filters)
	}
}

func TestAnalyzerNormalizesUnknownSeverity(t *testing.T) {
	analyzer := New(RuleFunc{
		RuleIDValue:   RuleContinueCandidate,
		CategoryValue: CategoryContinue,
		Fn: func(record AnalysisRecord, redactor Redactor) []Finding {
			return []Finding{{Severity: "typo"}}
		},
	})

	report := analyzer.Analyze([]AnalysisRecord{{RequestID: "req"}}, AnalyzeOptions{})
	if report.Findings[0].Severity != SeverityWarning {
		t.Fatalf("severity = %q, want warning", report.Findings[0].Severity)
	}
	if report.Summary.ByCategory[0].Counts.Warning != 1 {
		t.Fatalf("warning count = %d, want 1", report.Summary.ByCategory[0].Counts.Warning)
	}
}

func TestReportJSONShape(t *testing.T) {
	report := New().Analyze(nil, AnalyzeOptions{
		GeneratedAt: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		Metadata:    map[string]string{"schema": "history-analyzer/v1"},
	})

	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, fragment := range []string{`"generated_at"`, `"summary"`, `"total_records"`, `"metadata"`} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("report JSON missing %s: %s", fragment, got)
		}
	}
}
