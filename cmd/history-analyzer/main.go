package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ds2api/internal/historyanalyzer"
)

func main() {
	var opts cliOptions
	flag.StringVar(&opts.historyPath, "history", "", "Chat history index/export path")
	flag.StringVar(&opts.responseHistoryPath, "response-history", "", "Response history path in chat-history-compatible format")
	flag.StringVar(&opts.devCapturePath, "devcapture", "", "Dev capture JSON path")
	flag.StringVar(&opts.rawSamplePath, "rawsample", "", "Raw sample meta.json, sample dir, or root dir")
	flag.StringVar(&opts.scope, "scope", "history analyzer", "Report scope")
	flag.StringVar(&opts.outMarkdown, "out", "artifacts/history-analyzer/report.md", "Markdown output path")
	flag.StringVar(&opts.outJSON, "json", "artifacts/history-analyzer/report.json", "JSON output path")
	flag.StringVar(&opts.outFixtures, "fixtures", "artifacts/history-analyzer/fixture-candidates.json", "Fixture candidate JSON output path; empty disables")
	flag.IntVar(&opts.maxExcerptRunes, "max-excerpt-runes", 240, "Maximum evidence excerpt length")
	flag.Parse()

	if err := run(opts); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

type cliOptions struct {
	historyPath         string
	responseHistoryPath string
	devCapturePath      string
	rawSamplePath       string
	scope               string
	outMarkdown         string
	outJSON             string
	outFixtures         string
	maxExcerptRunes     int
}

func run(opts cliOptions) error {
	if err := validateOutputTargets(opts); err != nil {
		return err
	}

	redactor := historyanalyzer.DefaultRedactor()
	if opts.maxExcerptRunes > 0 {
		redactor.MaxExcerptRunes = opts.maxExcerptRunes
	}

	records, scope, err := loadRecords(opts, redactor)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("no records loaded")
	}
	if strings.TrimSpace(opts.scope) != "" {
		scope.Name = strings.TrimSpace(opts.scope)
	}
	fillScopeTimes(&scope, records)

	report := historyanalyzer.New(historyanalyzer.DefaultRules()...).Analyze(records, historyanalyzer.AnalyzeOptions{
		GeneratedAt: time.Now().UTC(),
		Scope:       scope,
		Redactor:    redactor,
		Metadata: map[string]string{
			"schema":           "history-analyzer/v1",
			"record_count":     strconv.Itoa(len(records)),
			"source_count":     strconv.Itoa(len(scope.Sources)),
			"max_excerpt_rune": strconv.Itoa(redactor.MaxExcerptRunes),
		},
	})
	report.Readiness = historyanalyzer.BuildReadinessSummary(report.Findings)

	if err := writeJSON(opts.outJSON, report); err != nil {
		return err
	}
	if err := writeText(opts.outMarkdown, historyanalyzer.RenderMarkdown(report)); err != nil {
		return err
	}
	if strings.TrimSpace(opts.outFixtures) != "" {
		if err := writeJSON(opts.outFixtures, historyanalyzer.FixtureCandidates(report)); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(os.Stderr, "wrote %s, %s, and %s\n", opts.outMarkdown, opts.outJSON, opts.outFixtures)
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "wrote %s and %s\n", opts.outMarkdown, opts.outJSON)
	return nil
}

func validateOutputTargets(opts cliOptions) error {
	stdoutTargets := 0
	for _, path := range []string{opts.outMarkdown, opts.outJSON, opts.outFixtures} {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if path == "-" {
			stdoutTargets++
		}
	}
	if stdoutTargets > 1 {
		return errors.New("only one output target may be '-'")
	}
	return nil
}

func loadRecords(opts cliOptions, redactor historyanalyzer.Redactor) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error) {
	type input struct {
		label string
		path  string
		load  func(string) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error)
	}
	inputs := []input{
		{
			label: "history",
			path:  opts.historyPath,
			load: func(path string) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error) {
				return historyanalyzer.LoadChatHistory(historyanalyzer.ChatHistoryLoadOptions{Path: path, Redactor: redactor})
			},
		},
		{
			label: "response-history",
			path:  opts.responseHistoryPath,
			load: func(path string) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error) {
				records, scope, err := historyanalyzer.LoadChatHistory(historyanalyzer.ChatHistoryLoadOptions{Path: path, Redactor: redactor})
				if err != nil {
					return nil, historyanalyzer.ReportScope{}, err
				}
				relabelSourceKind(records, &scope, "response_history")
				scope.Name = "response history"
				return records, scope, nil
			},
		},
		{
			label: "devcapture",
			path:  opts.devCapturePath,
			load: func(path string) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error) {
				return historyanalyzer.LoadDevCaptures(historyanalyzer.DevCaptureLoadOptions{Path: path, Redactor: redactor})
			},
		},
		{
			label: "rawsample",
			path:  opts.rawSamplePath,
			load: func(path string) ([]historyanalyzer.AnalysisRecord, historyanalyzer.ReportScope, error) {
				return historyanalyzer.LoadRawSamples(historyanalyzer.RawSampleLoadOptions{Path: path, Redactor: redactor})
			},
		},
	}

	records := make([]historyanalyzer.AnalysisRecord, 0)
	scope := historyanalyzer.ReportScope{Name: strings.TrimSpace(opts.scope)}
	loaded := false
	for _, item := range inputs {
		path := strings.TrimSpace(item.path)
		if path == "" {
			continue
		}
		loaded = true
		next, nextScope, err := item.load(path)
		if err != nil {
			return nil, historyanalyzer.ReportScope{}, fmt.Errorf("%s input: %w", item.label, err)
		}
		records = append(records, next...)
		scope.Sources = append(scope.Sources, nextScope.Sources...)
	}
	if !loaded {
		return nil, historyanalyzer.ReportScope{}, errors.New("at least one input is required: --history, --response-history, --devcapture, or --rawsample")
	}
	return records, scope, nil
}

func relabelSourceKind(records []historyanalyzer.AnalysisRecord, scope *historyanalyzer.ReportScope, kind string) {
	for i := range records {
		for j := range records[i].Sources {
			records[i].Sources[j].Kind = kind
		}
	}
	for i := range scope.Sources {
		scope.Sources[i].Kind = kind
	}
}

func fillScopeTimes(scope *historyanalyzer.ReportScope, records []historyanalyzer.AnalysisRecord) {
	for _, record := range records {
		if record.CreatedAt.IsZero() {
			continue
		}
		if scope.StartedAt.IsZero() || record.CreatedAt.Before(scope.StartedAt) {
			scope.StartedAt = record.CreatedAt
		}
		if scope.EndedAt.IsZero() || record.CreatedAt.After(scope.EndedAt) {
			scope.EndedAt = record.CreatedAt
		}
	}
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeBytes(path, body)
}

func writeText(path string, text string) error {
	return writeBytes(path, []byte(text))
}

func writeBytes(path string, body []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("output path is required")
	}
	if path == "-" {
		_, err := os.Stdout.Write(body)
		return err
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, body, 0o644)
}
