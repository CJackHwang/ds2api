package historyanalyzer

import (
	"sort"
	"strings"
	"time"
)

type Rule interface {
	ID() RuleID
	Category() Category
	Analyze(record AnalysisRecord, redactor Redactor) []Finding
}

type RuleFunc struct {
	RuleIDValue   RuleID
	CategoryValue Category
	Fn            func(record AnalysisRecord, redactor Redactor) []Finding
}

func (r RuleFunc) ID() RuleID {
	return r.RuleIDValue
}

func (r RuleFunc) Category() Category {
	return r.CategoryValue
}

func (r RuleFunc) Analyze(record AnalysisRecord, redactor Redactor) []Finding {
	if r.Fn == nil {
		return nil
	}
	return r.Fn(record, redactor)
}

type AnalyzeOptions struct {
	GeneratedAt time.Time
	Scope       ReportScope
	Redactor    Redactor
	Metadata    map[string]string
}

type Analyzer struct {
	rules []Rule
}

func New(rules ...Rule) Analyzer {
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		out = append(out, rule)
	}
	return Analyzer{rules: out}
}

func (a Analyzer) Rules() []Rule {
	out := make([]Rule, len(a.rules))
	copy(out, a.rules)
	return out
}

func (a Analyzer) Analyze(records []AnalysisRecord, opts AnalyzeOptions) Report {
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	redactor := opts.Redactor
	if redactor.MaxExcerptRunes == 0 {
		redactor = DefaultRedactor()
	}

	findings := make([]Finding, 0)
	for _, record := range records {
		for _, rule := range a.rules {
			for _, finding := range rule.Analyze(record, redactor) {
				findings = append(findings, normalizeFinding(finding, record, rule))
			}
		}
	}
	sortFindings(findings)

	return Report{
		GeneratedAt: generatedAt,
		Scope:       cloneReportScope(opts.Scope),
		Summary:     summarize(records, findings),
		Findings:    findings,
		Metadata:    cloneStringMap(opts.Metadata),
	}
}

func normalizeFinding(f Finding, record AnalysisRecord, rule Rule) Finding {
	if f.RuleID == "" {
		f.RuleID = rule.ID()
	}
	if f.Category == "" {
		f.Category = rule.Category()
	}
	if f.Severity == "" {
		if spec, ok := RuleSpecByID(f.RuleID); ok {
			f.Severity = spec.DefaultSeverity
		} else {
			f.Severity = SeverityWarning
		}
	} else if !knownSeverity(f.Severity) {
		f.Severity = SeverityWarning
	}
	if f.RequestID == "" {
		f.RequestID = record.RequestID
	}
	if f.SessionID == "" {
		f.SessionID = record.SessionID
	}
	if f.SuggestedAction == "" {
		if spec, ok := RuleSpecByID(f.RuleID); ok {
			f.SuggestedAction = spec.SuggestedAction
		}
	}
	return f
}

func summarize(records []AnalysisRecord, findings []Finding) Summary {
	byCategory := map[Category]*CategorySummary{}
	ruleCounts := map[Category]map[RuleID]int{}
	for _, finding := range findings {
		summary := byCategory[finding.Category]
		if summary == nil {
			summary = &CategorySummary{Category: finding.Category}
			byCategory[finding.Category] = summary
		}
		incrementSeverity(&summary.Counts, finding.Severity)
		if ruleCounts[finding.Category] == nil {
			ruleCounts[finding.Category] = map[RuleID]int{}
		}
		ruleCounts[finding.Category][finding.RuleID]++
	}

	out := make([]CategorySummary, 0, len(byCategory))
	for category, summary := range byCategory {
		summary.TopRule = topRule(ruleCounts[category])
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Category < out[j].Category
	})

	return Summary{
		TotalRecords:  len(records),
		TotalFindings: len(findings),
		ByCategory:    out,
	}
}

func incrementSeverity(counts *SeverityCounts, severity Severity) {
	switch severity {
	case SeverityInfo:
		counts.Info++
	case SeverityWarning:
		counts.Warning++
	case SeverityHigh:
		counts.High++
	case SeverityCritical:
		counts.Critical++
	default:
		counts.Warning++
	}
}

func topRule(counts map[RuleID]int) RuleID {
	var top RuleID
	var topCount int
	for ruleID, count := range counts {
		if count > topCount || (count == topCount && strings.Compare(string(ruleID), string(top)) < 0) {
			top = ruleID
			topCount = count
		}
	}
	return top
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a := findings[i]
		b := findings[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.RequestID < b.RequestID
	})
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return severityRank(SeverityWarning)
	}
}

func knownSeverity(severity Severity) bool {
	switch severity {
	case SeverityInfo, SeverityWarning, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func cloneReportScope(in ReportScope) ReportScope {
	out := in
	if len(in.Sources) > 0 {
		out.Sources = make([]SourceRef, len(in.Sources))
		copy(out.Sources, in.Sources)
	}
	out.Filters = cloneStringMap(in.Filters)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
