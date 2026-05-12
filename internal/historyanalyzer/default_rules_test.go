package historyanalyzer

import (
	"strings"
	"testing"
)

func TestDefaultRulesCoverKnownRuleSpecs(t *testing.T) {
	rules := DefaultRules()
	specs := KnownRuleSpecs()
	if len(rules) != len(specs) {
		t.Fatalf("default rules = %d, specs = %d", len(rules), len(specs))
	}

	seen := map[RuleID]bool{}
	for _, rule := range rules {
		seen[rule.ID()] = true
	}
	for _, spec := range specs {
		if !seen[spec.ID] {
			t.Fatalf("missing default rule for %s", spec.ID)
		}
	}
}

func TestDefaultRulesDetectSyntheticSamples(t *testing.T) {
	tests := []struct {
		name   string
		ruleID RuleID
		record AnalysisRecord
	}{
		{
			name:   "tool marker leak",
			ruleID: RuleToolMarkerLeak,
			record: AnalysisRecord{
				RequestID: "tool-leak",
				Text: map[string]string{
					"content": `<|DSML|tool_calls><|DSML|invoke name="search"></|DSML|invoke></|DSML|tool_calls>`,
				},
			},
		},
		{
			name:   "tool call as text",
			ruleID: RuleToolCallAsText,
			record: AnalysisRecord{
				RequestID: "tool-text",
				Text: map[string]string{
					"upstream_text": `<tool_calls><invoke name="search"><parameter name="q">weather</parameter></invoke></tool_calls>`,
				},
			},
		},
		{
			name:   "tool false positive",
			ruleID: RuleToolFalsePositive,
			record: AnalysisRecord{
				RequestID: "false-positive",
				Flags:     map[string]string{"parser_false_positive": "true"},
			},
		},
		{
			name:   "context tool pair orphan",
			ruleID: RuleContextToolPairOrphan,
			record: AnalysisRecord{
				RequestID: "orphan",
				Flags:     map[string]string{"orphan_tool_result": "true"},
			},
		},
		{
			name:   "context reasoning bloat",
			ruleID: RuleContextReasoningBloat,
			record: AnalysisRecord{
				RequestID: "reasoning-bloat",
				Text: map[string]string{
					"reasoning": strings.Repeat("x", reasoningBloatRuneThreshold),
				},
			},
		},
		{
			name:   "context current input mismatch",
			ruleID: RuleContextCurrentInputMismatch,
			record: AnalysisRecord{
				RequestID: "current-input",
				Flags:     map[string]string{"current_input_hash_mismatch": "true"},
			},
		},
		{
			name:   "continue candidate",
			ruleID: RuleContinueCandidate,
			record: AnalysisRecord{
				RequestID:    "continue",
				FinishReason: "length",
			},
		},
		{
			name:   "capability search thinking conflict",
			ruleID: RuleCapabilitySearchThinking,
			record: AnalysisRecord{
				RequestID: "capability",
				Flags: map[string]string{
					"capability_search_enabled":   "true",
					"capability_thinking_enabled": "true",
				},
			},
		},
		{
			name:   "account retry recovered",
			ruleID: RuleAccountRetryRecovered,
			record: AnalysisRecord{
				RequestID:  "retry-recovered",
				StatusCode: 200,
				Metrics:    RuntimeMetrics{RetryCount: 1},
			},
		},
		{
			name:   "account retry exhausted",
			ruleID: RuleAccountRetryExhausted,
			record: AnalysisRecord{
				RequestID:  "retry-exhausted",
				Status:     "failed",
				StatusCode: 429,
				Metrics:    RuntimeMetrics{RetryCount: 2, AccountSwitch: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := reportForRule(t, tt.ruleID, tt.record)
			if len(report.Findings) != 1 {
				t.Fatalf("findings = %d, want 1: %#v", len(report.Findings), report.Findings)
			}
			finding := report.Findings[0]
			if finding.RuleID != tt.ruleID {
				t.Fatalf("rule id = %q, want %q", finding.RuleID, tt.ruleID)
			}
			if finding.SuggestedAction == "" {
				t.Fatal("suggested action was not filled")
			}
			if finding.FixtureHint == nil || !finding.FixtureHint.NeedsReview {
				t.Fatalf("fixture hint missing review marker: %#v", finding.FixtureHint)
			}
			if len(finding.Evidence) != 1 || finding.Evidence[0].Excerpt == "" {
				t.Fatalf("evidence missing excerpt: %#v", finding.Evidence)
			}
		})
	}
}

func TestToolMarkerLeakIgnoresFencedExamples(t *testing.T) {
	report := reportForRule(t, RuleToolMarkerLeak, AnalysisRecord{
		RequestID: "fenced-example",
		Text: map[string]string{
			"content": "Use this example:\n```xml\n<tool_calls><invoke name=\"demo\"></invoke></tool_calls>\n```\nDone.",
		},
	})
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %#v, want none", report.Findings)
	}
}

func TestToolFalsePositiveDetectsParsedFencedExample(t *testing.T) {
	report := reportForRule(t, RuleToolFalsePositive, AnalysisRecord{
		RequestID:    "parsed-example",
		FinishReason: "tool_calls",
		Text: map[string]string{
			"content": "Example only:\n```xml\n<tool_calls><invoke name=\"demo\"></invoke></tool_calls>\n```",
		},
	})
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %#v, want one", report.Findings)
	}
}

func TestContinueCandidateDetectsIncompleteShapes(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "unclosed fence", text: "```go\nfmt.Println(1)\n"},
		{name: "unbalanced json", text: `{"items":[{"name":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := reportForRule(t, RuleContinueCandidate, AnalysisRecord{
				RequestID: tt.name,
				Text:      map[string]string{"content": tt.text},
			})
			if len(report.Findings) != 1 {
				t.Fatalf("findings = %#v, want one", report.Findings)
			}
		})
	}
}

func TestAccountRetryFlagsRespectTerminalState(t *testing.T) {
	tests := []struct {
		name     string
		record   AnalysisRecord
		wantOnly RuleID
	}{
		{
			name: "stale recovered flag on failure is exhausted",
			record: AnalysisRecord{
				RequestID:  "stale-recovered",
				StatusCode: 429,
				Flags:      map[string]string{"account_retry_recovered": "true"},
				Metrics:    RuntimeMetrics{RetryCount: 1},
			},
			wantOnly: RuleAccountRetryExhausted,
		},
		{
			name: "stale exhausted flag on success is recovered",
			record: AnalysisRecord{
				RequestID:  "stale-exhausted",
				StatusCode: 200,
				Flags:      map[string]string{"account_retry_exhausted": "true"},
				Metrics:    RuntimeMetrics{RetryCount: 1},
			},
			wantOnly: RuleAccountRetryRecovered,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := New(DefaultRules()...).Analyze([]AnalysisRecord{tt.record}, AnalyzeOptions{})
			if len(report.Findings) != 1 {
				t.Fatalf("findings = %#v, want exactly one", report.Findings)
			}
			if report.Findings[0].RuleID != tt.wantOnly {
				t.Fatalf("rule id = %q, want %q", report.Findings[0].RuleID, tt.wantOnly)
			}
		})
	}
}

func TestFenceParsingTracksFullDelimiterLength(t *testing.T) {
	text := strings.Join([]string{
		"````markdown",
		"Document a nested example:",
		"```xml",
		`<tool_calls><invoke name="demo"></invoke></tool_calls>`,
		"```",
		"````",
		"Visible prose.",
	}, "\n")

	leakReport := reportForRule(t, RuleToolMarkerLeak, AnalysisRecord{
		RequestID: "four-backtick-example",
		Text:      map[string]string{"content": text},
	})
	if len(leakReport.Findings) != 0 {
		t.Fatalf("marker leak findings = %#v, want none", leakReport.Findings)
	}

	continueReport := reportForRule(t, RuleContinueCandidate, AnalysisRecord{
		RequestID: "four-backtick-example",
		Text:      map[string]string{"content": text},
	})
	if len(continueReport.Findings) != 0 {
		t.Fatalf("continue findings = %#v, want none", continueReport.Findings)
	}

	falsePositiveReport := reportForRule(t, RuleToolFalsePositive, AnalysisRecord{
		RequestID:    "four-backtick-example",
		FinishReason: "tool_calls",
		Text:         map[string]string{"content": text},
	})
	if len(falsePositiveReport.Findings) != 1 {
		t.Fatalf("false-positive findings = %#v, want one", falsePositiveReport.Findings)
	}
}

func reportForRule(t *testing.T, id RuleID, record AnalysisRecord) Report {
	t.Helper()
	for _, rule := range DefaultRules() {
		if rule.ID() == id {
			return New(rule).Analyze([]AnalysisRecord{record}, AnalyzeOptions{})
		}
	}
	t.Fatalf("rule %s not found", id)
	return Report{}
}
