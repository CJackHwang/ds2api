package historyanalyzer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const reasoningBloatRuneThreshold = 12000

var (
	toolMarkerRe = regexp.MustCompile(`(?is)<\s*/?\s*(?:\|?dsml\|?|｜?dsml｜?|dsml[-_ ]*)?(?:tool_calls|invoke|parameter)\b|<\s*/?\s*(?:tool_calls|invoke|parameter)\b|<｜(?:begin▁of▁sentence|user|assistant|system|end▁of▁instructions)｜>`)
	toolCallRe   = regexp.MustCompile(`(?is)(<\s*/?\s*(?:\|?dsml\|?|｜?dsml｜?|dsml[-_ ]*)?tool_calls\b.*<\s*/?\s*(?:\|?dsml\|?|｜?dsml｜?|dsml[-_ ]*)?invoke\b)|("tool_calls"\s*:\s*\[.*"function"\s*:)`)
)

func DefaultRules() []Rule {
	return []Rule{
		RuleFunc{RuleIDValue: RuleToolMarkerLeak, CategoryValue: CategoryTool, Fn: analyzeToolMarkerLeak},
		RuleFunc{RuleIDValue: RuleToolCallAsText, CategoryValue: CategoryTool, Fn: analyzeToolCallAsText},
		RuleFunc{RuleIDValue: RuleToolFalsePositive, CategoryValue: CategoryTool, Fn: analyzeToolFalsePositive},
		RuleFunc{RuleIDValue: RuleContextToolPairOrphan, CategoryValue: CategoryContext, Fn: analyzeContextToolPairOrphan},
		RuleFunc{RuleIDValue: RuleContextReasoningBloat, CategoryValue: CategoryContext, Fn: analyzeContextReasoningBloat},
		RuleFunc{RuleIDValue: RuleContextCurrentInputMismatch, CategoryValue: CategoryContext, Fn: analyzeContextCurrentInputMismatch},
		RuleFunc{RuleIDValue: RuleContinueCandidate, CategoryValue: CategoryContinue, Fn: analyzeContinueCandidate},
		RuleFunc{RuleIDValue: RuleCapabilitySearchThinking, CategoryValue: CategoryCapability, Fn: analyzeCapabilitySearchThinkingConflict},
		RuleFunc{RuleIDValue: RuleAccountRetryRecovered, CategoryValue: CategoryAccountRuntime, Fn: analyzeAccountRetryRecovered},
		RuleFunc{RuleIDValue: RuleAccountRetryExhausted, CategoryValue: CategoryAccountRuntime, Fn: analyzeAccountRetryExhausted},
	}
}

func analyzeToolMarkerLeak(record AnalysisRecord, redactor Redactor) []Finding {
	for _, field := range []string{"content", "response_body"} {
		text := record.Text[field]
		if strings.TrimSpace(text) == "" {
			continue
		}
		visible := stripFencedBlocks(text)
		if toolMarkerRe.MatchString(visible) {
			return []Finding{newFinding(
				RuleToolMarkerLeak,
				CategoryTool,
				redactor.Evidence(recordSource(record), field, text, "visible response contains a tool or control marker"),
				fixtureHint("toolparser/leak", "tool-marker-leak", record),
			)}
		}
	}
	return nil
}

func analyzeToolCallAsText(record AnalysisRecord, redactor Redactor) []Finding {
	if hasStructuredToolCalls(record) {
		return nil
	}
	for _, field := range []string{"upstream_text", "raw_assistant", "response_text", "reasoning", "content", "response_body"} {
		text := record.Text[field]
		if strings.TrimSpace(text) == "" {
			continue
		}
		if looksLikeToolCall(text) {
			return []Finding{newFinding(
				RuleToolCallAsText,
				CategoryTool,
				redactor.Evidence(recordSource(record), field, text, "tool-like upstream text was not rendered as structured tool_calls"),
				fixtureHint("toolparser/true_positive", "tool-call-as-text", record),
			)}
		}
	}
	return nil
}

func analyzeToolFalsePositive(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor,
		"parser_false_positive",
		"tool_false_positive",
		"dropped_visible_text",
		"tool_text_dropped",
		"visible_text_dropped",
	); ok {
		return []Finding{newFinding(
			RuleToolFalsePositive,
			CategoryTool,
			evidence,
			fixtureHint("toolparser/false_positive", "tool-false-positive", record),
		)}
	}

	if hasStructuredToolCalls(record) {
		for _, field := range []string{"content", "response_body", "upstream_text", "raw_assistant"} {
			text := record.Text[field]
			if fencedBlockContainsToolSyntax(text) {
				return []Finding{newFinding(
					RuleToolFalsePositive,
					CategoryTool,
					redactor.Evidence(recordSource(record), field, text, "tool syntax appears inside a fenced example that was parsed as a tool call"),
					fixtureHint("toolparser/false_positive", "tool-example-false-positive", record),
				)}
			}
		}
	}
	return nil
}

func analyzeContextToolPairOrphan(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor,
		"context_tool_pair_orphan",
		"tool_pair_orphan",
		"orphan_tool_call",
		"orphan_tool_result",
	); ok {
		return []Finding{newFinding(
			RuleContextToolPairOrphan,
			CategoryContext,
			evidence,
			fixtureHint("context/tool_pair", "tool-pair-orphan", record),
		)}
	}

	if evidence, ok := textContainsAnyEvidence(record, redactor, []string{"history_text", "final_prompt", "messages", "context_plan"}, []string{
		"orphan_tool_call",
		"orphan_tool_result",
		"orphan tool_call",
		"orphan tool_result",
		"tool pair orphan",
	}); ok {
		return []Finding{newFinding(
			RuleContextToolPairOrphan,
			CategoryContext,
			evidence,
			fixtureHint("context/tool_pair", "tool-pair-orphan", record),
		)}
	}
	return nil
}

func analyzeContextReasoningBloat(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor, "reasoning_bloat", "context_reasoning_bloat"); ok {
		return []Finding{newFinding(
			RuleContextReasoningBloat,
			CategoryContext,
			evidence,
			fixtureHint("context/reasoning", "reasoning-bloat", record),
		)}
	}

	if tokens, ok := firstMetricNumber(record.Metrics.Extra, "reasoning_tokens", "history_reasoning_tokens"); ok && tokens >= 4096 {
		return []Finding{newFinding(
			RuleContextReasoningBloat,
			CategoryContext,
			metricEvidence(redactor, "metrics."+metricNameFor(record.Metrics.Extra, "reasoning_tokens", "history_reasoning_tokens"), fmt.Sprintf("%.0f", tokens), "reasoning token count exceeds analyzer threshold"),
			fixtureHint("context/reasoning", "reasoning-bloat", record),
		)}
	}

	reasoning := record.Text["reasoning"]
	if utf8.RuneCountInString(reasoning) >= reasoningBloatRuneThreshold {
		return []Finding{newFinding(
			RuleContextReasoningBloat,
			CategoryContext,
			redactor.Evidence(recordSource(record), "reasoning", reasoning, "reasoning text is large enough to crowd out current input"),
			fixtureHint("context/reasoning", "reasoning-bloat", record),
		)}
	}
	return nil
}

func analyzeContextCurrentInputMismatch(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor,
		"current_input_mismatch",
		"current_input_missing",
		"current_input_history_mismatch",
		"current_input_tools_mismatch",
		"current_input_hash_mismatch",
		"current_input_file_error",
	); ok {
		return []Finding{newFinding(
			RuleContextCurrentInputMismatch,
			CategoryContext,
			evidence,
			fixtureHint("context/current_input", "current-input-mismatch", record),
		)}
	}

	for _, field := range []string{"error", "final_prompt", "messages"} {
		text := record.Text[field]
		lower := strings.ToLower(text)
		if containsAny(lower, "ds2api_history", "ds2api_tools") &&
			containsAny(lower, "missing", "mismatch", "hash", "not found") {
			return []Finding{newFinding(
				RuleContextCurrentInputMismatch,
				CategoryContext,
				redactor.Evidence(recordSource(record), field, text, "current-input text contains missing or mismatched DS2API file evidence"),
				fixtureHint("context/current_input", "current-input-mismatch", record),
			)}
		}
	}
	return nil
}

func analyzeContinueCandidate(record AnalysisRecord, redactor Redactor) []Finding {
	if isContinueFinishReason(record.FinishReason) {
		return []Finding{newFinding(
			RuleContinueCandidate,
			CategoryContinue,
			redactor.Evidence(recordSource(record), "finish_reason", record.FinishReason, "finish reason suggests truncation"),
			fixtureHint("auto_continue/shadow", "continue-candidate", record),
		)}
	}
	if evidence, ok := trueFlagEvidence(record, redactor,
		"response_truncated",
		"continue_candidate",
		"auto_continue_candidate",
		"stream_interrupted",
		"sse_incomplete",
		"incomplete",
		"truncated",
	); ok {
		return []Finding{newFinding(
			RuleContinueCandidate,
			CategoryContinue,
			evidence,
			fixtureHint("auto_continue/shadow", "continue-candidate", record),
		)}
	}
	if status := strings.ToLower(record.Status); strings.Contains(status, "incomplete") || strings.Contains(status, "auto_continue") {
		return []Finding{newFinding(
			RuleContinueCandidate,
			CategoryContinue,
			redactor.Evidence(recordSource(record), "status", record.Status, "status suggests incomplete output"),
			fixtureHint("auto_continue/shadow", "continue-candidate", record),
		)}
	}

	for _, field := range []string{"content", "response_body"} {
		text := record.Text[field]
		if strings.TrimSpace(text) == "" {
			continue
		}
		switch {
		case hasUnclosedFence(text):
			return []Finding{newFinding(
				RuleContinueCandidate,
				CategoryContinue,
				redactor.Evidence(recordSource(record), field, text, "output has an unclosed Markdown fence"),
				fixtureHint("auto_continue/shadow", "continue-candidate", record),
			)}
		case likelyUnbalancedJSON(text):
			return []Finding{newFinding(
				RuleContinueCandidate,
				CategoryContinue,
				redactor.Evidence(recordSource(record), field, text, "output looks like an unfinished JSON object or array"),
				fixtureHint("auto_continue/shadow", "continue-candidate", record),
			)}
		}
	}
	return nil
}

func analyzeCapabilitySearchThinkingConflict(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor,
		"capability_conflict",
		"search_thinking_conflict",
		"current_input_search_conflict",
		"capability_search_thinking_conflict",
	); ok {
		return []Finding{newFinding(
			RuleCapabilitySearchThinking,
			CategoryCapability,
			evidence,
			fixtureHint("capability/search_thinking", "search-thinking-conflict", record),
		)}
	}

	search := boolFlag(record.Flags, "capability_search_enabled", "search_enabled", "search")
	thinking := boolFlag(record.Flags, "capability_thinking_enabled", "thinking_enabled", "thinking")
	currentInput := boolFlag(record.Flags, "capability_current_input_enabled", "current_input_enabled", "current_input_file", "current_input_file_enabled")
	warnings := strings.ToLower(flagValue(record.Flags, "capability_warnings", "capability_warning"))
	if search && (thinking || currentInput || strings.Contains(warnings, "conflict")) {
		return []Finding{newFinding(
			RuleCapabilitySearchThinking,
			CategoryCapability,
			flagsEvidence(redactor, "capability_search_enabled", "true", "search was enabled with thinking/current-input conflict signals"),
			fixtureHint("capability/search_thinking", "search-thinking-conflict", record),
		)}
	}
	return nil
}

func analyzeAccountRetryRecovered(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor, "account_retry_recovered", "retry_recovered", "empty_output_recovered"); ok {
		if retryOrSwitch(record) && isSuccessfulRecord(record) {
			return []Finding{newFinding(
				RuleAccountRetryRecovered,
				CategoryAccountRuntime,
				evidence,
				fixtureHint("account/retry", "retry-recovered", record),
			)}
		}
		return nil
	}
	if retryOrSwitch(record) && isSuccessfulRecord(record) {
		return []Finding{newFinding(
			RuleAccountRetryRecovered,
			CategoryAccountRuntime,
			retryEvidence(record, redactor, "retry or account switch recovered before the final response"),
			fixtureHint("account/retry", "retry-recovered", record),
		)}
	}
	return nil
}

func analyzeAccountRetryExhausted(record AnalysisRecord, redactor Redactor) []Finding {
	if evidence, ok := trueFlagEvidence(record, redactor, "account_retry_exhausted", "retry_exhausted", "account_switch_exhausted"); ok {
		if retryOrSwitch(record) && isFailedRecord(record) {
			return []Finding{newFinding(
				RuleAccountRetryExhausted,
				CategoryAccountRuntime,
				evidence,
				fixtureHint("account/retry", "retry-exhausted", record),
			)}
		}
		return nil
	}
	if retryOrSwitch(record) && isFailedRecord(record) {
		return []Finding{newFinding(
			RuleAccountRetryExhausted,
			CategoryAccountRuntime,
			retryEvidence(record, redactor, "retry or account switch was exhausted before a successful response"),
			fixtureHint("account/retry", "retry-exhausted", record),
		)}
	}
	return nil
}

func newFinding(ruleID RuleID, category Category, evidence Evidence, hint *FixtureHint) Finding {
	return Finding{
		RuleID:      ruleID,
		Category:    category,
		Evidence:    []Evidence{evidence},
		FixtureHint: hint,
	}
}

func fixtureHint(suite, name string, record AnalysisRecord) *FixtureHint {
	refs := make([]string, 0, len(record.Sources))
	for _, source := range record.Sources {
		switch {
		case source.Path != "":
			refs = append(refs, source.Path)
		case source.ID != "":
			refs = append(refs, source.ID)
		}
	}
	return &FixtureHint{
		Suite:       suite,
		Name:        name,
		SourceRefs:  refs,
		NeedsReview: true,
	}
}

func recordSource(record AnalysisRecord) string {
	for _, source := range record.Sources {
		if strings.TrimSpace(source.Kind) != "" {
			return source.Kind
		}
	}
	if record.Protocol != "" {
		return record.Protocol
	}
	if record.Surface != "" {
		return record.Surface
	}
	return "analysis_record"
}

func looksLikeToolCall(text string) bool {
	return toolCallRe.MatchString(stripFencedBlocks(text))
}

func hasStructuredToolCalls(record AnalysisRecord) bool {
	if strings.EqualFold(strings.TrimSpace(record.FinishReason), "tool_calls") {
		return true
	}
	return boolFlag(record.Flags,
		"structured_tool_calls",
		"has_structured_tool_calls",
		"tool_calls_rendered",
		"rendered_tool_calls",
		"tool_calls",
	)
}

func boolFlag(flags map[string]string, keys ...string) bool {
	for _, key := range keys {
		if isTruthy(flagValue(flags, key)) {
			return true
		}
	}
	return false
}

func flagValue(flags map[string]string, keys ...string) string {
	for _, key := range keys {
		for actual, value := range flags {
			if strings.EqualFold(strings.TrimSpace(actual), key) {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "yes", "y", "on", "enabled", "enable", "shadow", "enforce":
		return true
	default:
		return false
	}
}

func trueFlagEvidence(record AnalysisRecord, redactor Redactor, keys ...string) (Evidence, bool) {
	for _, key := range keys {
		value := flagValue(record.Flags, key)
		if isTruthy(value) {
			return flagsEvidence(redactor, key, value, "normalized analyzer flag matched"), true
		}
	}
	return Evidence{}, false
}

func flagsEvidence(redactor Redactor, key, value, note string) Evidence {
	return redactor.Evidence("analysis_record", "flags."+key, key+"="+value, note)
}

func metricEvidence(redactor Redactor, field, value, note string) Evidence {
	return redactor.Evidence("analysis_record", field, value, note)
}

func textContainsAnyEvidence(record AnalysisRecord, redactor Redactor, fields []string, needles []string) (Evidence, bool) {
	for _, field := range fields {
		text := record.Text[field]
		lower := strings.ToLower(text)
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				return redactor.Evidence(recordSource(record), field, text, "text contains "+needle), true
			}
		}
	}
	return Evidence{}, false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func firstMetricNumber(extra map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if n, ok := metricNumber(extra, key); ok {
			return n, true
		}
	}
	return 0, false
}

func metricNameFor(extra map[string]any, keys ...string) string {
	for _, key := range keys {
		if _, ok := metricNumber(extra, key); ok {
			return key
		}
	}
	if len(keys) == 0 {
		return "unknown"
	}
	return keys[0]
}

func metricNumber(extra map[string]any, key string) (float64, bool) {
	if len(extra) == 0 {
		return 0, false
	}
	for actual, value := range extra {
		if !strings.EqualFold(strings.TrimSpace(actual), key) {
			continue
		}
		switch typed := value.(type) {
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case float64:
			return typed, true
		case json.Number:
			n, err := strconv.ParseFloat(string(typed), 64)
			return n, err == nil
		case string:
			n, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			return n, err == nil
		default:
			return 0, false
		}
	}
	return 0, false
}

func stripFencedBlocks(text string) string {
	if !strings.Contains(text, "```") && !strings.Contains(text, "~~~") {
		return text
	}
	var b strings.Builder
	inFence := false
	var fenceMarker byte
	fenceLength := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if marker, length, ok := fenceDelimiter(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
				fenceLength = length
			} else if marker == fenceMarker && length >= fenceLength {
				inFence = false
				fenceMarker = 0
				fenceLength = 0
			}
			continue
		}
		if inFence {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func fencedBlockContainsToolSyntax(text string) bool {
	if !strings.Contains(text, "```") && !strings.Contains(text, "~~~") {
		return false
	}
	inFence := false
	var fenceMarker byte
	fenceLength := 0
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if marker, length, ok := fenceDelimiter(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
				fenceLength = length
				b.Reset()
				continue
			}
			if marker == fenceMarker && length >= fenceLength {
				if toolMarkerRe.MatchString(b.String()) || toolCallRe.MatchString(b.String()) {
					return true
				}
				inFence = false
				fenceMarker = 0
				fenceLength = 0
				b.Reset()
				continue
			}
		}
		if inFence {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return false
}

func hasUnclosedFence(text string) bool {
	inFence := false
	var fenceMarker byte
	fenceLength := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if marker, length, ok := fenceDelimiter(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
				fenceLength = length
			} else if marker == fenceMarker && length >= fenceLength {
				inFence = false
				fenceMarker = 0
				fenceLength = 0
			}
		}
	}
	return inFence
}

func fenceDelimiter(trimmed string) (byte, int, bool) {
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == marker {
		length++
	}
	return marker, length, length >= 3
}

func likelyUnbalancedJSON(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return false
	}
	stack := make([]rune, 0, 8)
	inString := false
	escaped := false
	for _, r := range trimmed {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, r)
		case '}', ']':
			if len(stack) == 0 {
				return false
			}
			top := stack[len(stack)-1]
			if (top == '{' && r != '}') || (top == '[' && r != ']') {
				return true
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) > 0 || inString
}

func isContinueFinishReason(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "length", "max_tokens", "incomplete", "stream_incomplete":
		return true
	default:
		return false
	}
}

func retryOrSwitch(record AnalysisRecord) bool {
	return record.Metrics.RetryCount > 0 || record.Metrics.AccountSwitch > 0
}

func isSuccessfulRecord(record AnalysisRecord) bool {
	if isFailedRecord(record) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(record.Status))
	if status == "ok" || status == "success" || status == "succeeded" || status == "completed" {
		return true
	}
	return record.StatusCode >= 200 && record.StatusCode < 300
}

func isFailedRecord(record AnalysisRecord) bool {
	status := strings.ToLower(strings.TrimSpace(record.Status))
	if strings.Contains(status, "error") || strings.Contains(status, "fail") || strings.Contains(status, "exhausted") {
		return true
	}
	if record.StatusCode >= 400 {
		return true
	}
	return strings.TrimSpace(record.Text["error"]) != ""
}

func retryEvidence(record AnalysisRecord, redactor Redactor, note string) Evidence {
	return redactor.Evidence(
		"analysis_record",
		"metrics.retry_count",
		fmt.Sprintf("retry_count=%d account_switch=%d status=%s status_code=%d", record.Metrics.RetryCount, record.Metrics.AccountSwitch, record.Status, record.StatusCode),
		note,
	)
}
