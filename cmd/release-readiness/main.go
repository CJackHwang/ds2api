package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ds2api/internal/readiness"
)

func main() {
	var opts cliOptions
	flag.StringVar(&opts.version, "version", "", "Version or release candidate label")
	flag.StringVar(&opts.branch, "branch", "current", "Branch name, or 'current' to read from git")
	flag.StringVar(&opts.scope, "scope", "M4.0 release readiness baseline", "Report scope")
	flag.StringVar(&opts.owner, "owner", "", "Decision owner")
	flag.StringVar(&opts.outMarkdown, "out", "artifacts/release-readiness/report.md", "Markdown output path")
	flag.StringVar(&opts.outJSON, "json", "artifacts/release-readiness/report.json", "JSON output path")
	flag.StringVar(&opts.lintResult, "lint-result", "unknown", "lint gate result: pass, fail, unknown")
	flag.StringVar(&opts.refactorResult, "refactor-result", "unknown", "refactor line gate result: pass, fail, unknown")
	flag.StringVar(&opts.unitResult, "unit-result", "unknown", "unit gate result: pass, fail, unknown")
	flag.StringVar(&opts.webuiBuildResult, "webui-build-result", "unknown", "WebUI build gate result: pass, fail, unknown")
	flag.StringVar(&opts.liveResult, "live-result", "skip", "live gate result: pass, fail, skip, unknown")
	flag.StringVar(&opts.liveSkipReason, "live-skip-reason", "not required unless high-risk live path changes", "Reason when live gate is skipped")
	flag.StringVar(&opts.historyAnalyzerJSON, "history-analyzer-json", "", "Future input: History Analyzer JSON report")
	flag.StringVar(&opts.parserShadowJSON, "parser-shadow-json", "", "Future input: parser shadow JSON report")
	flag.StringVar(&opts.contextShadowJSON, "context-shadow-json", "", "Future input: context shadow JSON report")
	flag.StringVar(&opts.autoContinueJSON, "auto-continue-json", "", "Future input: auto continue shadow JSON report")
	flag.StringVar(&opts.capabilityRouterJSON, "capability-router-json", "", "Future input: capability router shadow JSON report")
	flag.Parse()

	if err := run(opts); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

type cliOptions struct {
	version              string
	branch               string
	scope                string
	owner                string
	outMarkdown          string
	outJSON              string
	lintResult           string
	refactorResult       string
	unitResult           string
	webuiBuildResult     string
	liveResult           string
	liveSkipReason       string
	historyAnalyzerJSON  string
	parserShadowJSON     string
	contextShadowJSON    string
	autoContinueJSON     string
	capabilityRouterJSON string
}

func run(opts cliOptions) error {
	branch, err := resolveBranch(opts.branch)
	if err != nil {
		return err
	}

	gates, err := buildGates(opts)
	if err != nil {
		return err
	}

	shadowInputs, err := buildShadowInputs(opts)
	if err != nil {
		return err
	}

	report := readiness.BuildBaselineReport(readiness.BaselineOptions{
		Version:       opts.version,
		Branch:        branch,
		GeneratedAt:   time.Now().UTC(),
		Scope:         opts.scope,
		DecisionOwner: opts.owner,
		Gates:         gates,
		ShadowInputs:  shadowInputs,
	})

	if err := writeJSON(opts.outJSON, report); err != nil {
		return err
	}
	if err := writeText(opts.outMarkdown, readiness.RenderMarkdown(report)); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "wrote %s and %s\n", opts.outMarkdown, opts.outJSON)
	return nil
}

func buildGates(opts cliOptions) ([]readiness.GateResult, error) {
	lint, err := parseRequiredGateResult(opts.lintResult)
	if err != nil {
		return nil, fmt.Errorf("lint-result: %w", err)
	}
	refactor, err := parseRequiredGateResult(opts.refactorResult)
	if err != nil {
		return nil, fmt.Errorf("refactor-result: %w", err)
	}
	unit, err := parseRequiredGateResult(opts.unitResult)
	if err != nil {
		return nil, fmt.Errorf("unit-result: %w", err)
	}
	webuiBuild, err := parseRequiredGateResult(opts.webuiBuildResult)
	if err != nil {
		return nil, fmt.Errorf("webui-build-result: %w", err)
	}
	live, err := parseLiveGateResult(opts.liveResult)
	if err != nil {
		return nil, fmt.Errorf("live-result: %w", err)
	}

	return []readiness.GateResult{
		{Name: "lint", Result: lint, Evidence: "./scripts/lint.sh"},
		{Name: "refactor line gate", Result: refactor, Evidence: "./tests/scripts/check-refactor-line-gate.sh"},
		{Name: "unit all", Result: unit, Evidence: "./tests/scripts/run-unit-all.sh"},
		{Name: "webui build", Result: webuiBuild, Evidence: "npm run build --prefix webui"},
		{Name: "live", Result: live, Evidence: liveEvidence(live, opts.liveSkipReason)},
	}, nil
}

func buildShadowInputs(opts cliOptions) ([]readiness.ShadowInput, error) {
	inputs := []readiness.ShadowInput{
		{Source: "history analyzer", Path: opts.historyAnalyzerJSON},
		{Source: "parser shadow", Path: opts.parserShadowJSON},
		{Source: "context shadow", Path: opts.contextShadowJSON},
		{Source: "auto continue shadow", Path: opts.autoContinueJSON},
		{Source: "capability router shadow", Path: opts.capabilityRouterJSON},
	}

	out := make([]readiness.ShadowInput, 0, len(inputs))
	for _, input := range inputs {
		if input.Path == "" {
			continue
		}
		if err := requireFile(input.Path); err != nil {
			return nil, fmt.Errorf("%s input: %w", input.Source, err)
		}
		out = append(out, input)
	}
	return out, nil
}

func parseRequiredGateResult(value string) (readiness.GateResultValue, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pass":
		return readiness.GatePass, nil
	case "fail":
		return readiness.GateFail, nil
	case "unknown", "":
		return readiness.GateUnknown, nil
	default:
		return "", fmt.Errorf("invalid result %q", value)
	}
}

func parseLiveGateResult(value string) (readiness.GateResultValue, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "skip":
		return readiness.GateSkip, nil
	default:
		return parseRequiredGateResult(value)
	}
}

func liveEvidence(result readiness.GateResultValue, skipReason string) string {
	if result == readiness.GateSkip && strings.TrimSpace(skipReason) != "" {
		return skipReason
	}
	return "./tests/scripts/run-live.sh"
}

func resolveBranch(branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch != "" && branch != "current" {
		return branch, nil
	}
	cmd := exec.Command("git", "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve current branch: %w", err)
	}
	resolved := strings.TrimSpace(string(out))
	if resolved == "" {
		return "", errors.New("current branch is empty; pass --branch explicitly")
	}
	return resolved, nil
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("path is a directory")
	}
	return nil
}

func writeJSON(path string, report readiness.Report) error {
	body, err := json.MarshalIndent(report, "", "  ")
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
	if path == "-" {
		_, err := os.Stdout.Write(body)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
