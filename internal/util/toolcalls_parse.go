package util

import (
	"encoding/json"
	"strings"
)

type ParsedToolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolCallParseResult struct {
	Calls             []ParsedToolCall
	SawToolCallSyntax bool
	RejectedByPolicy  bool
	RejectedToolNames []string
}

func ParseToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseToolCallsDetailed(text, availableToolNames).Calls
}

func ParseToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	result := ToolCallParseResult{}
	if strings.TrimSpace(text) == "" {
		return result
	}
	result.SawToolCallSyntax = looksLikeToolCallSyntax(text)
	if shouldSkipToolCallParsingForCodeFenceExample(text) {
		return result
	}

	candidates := buildToolCallCandidates(text)
	var parsed []ParsedToolCall
	for _, candidate := range candidates {
		tc := parseToolCallsPayload(candidate)
		if len(tc) == 0 {
			tc = parseXMLToolCalls(candidate)
		}
		if len(tc) == 0 {
			tc = parseMarkupToolCalls(candidate)
		}
		if len(tc) == 0 {
			tc = parseTextKVToolCalls(candidate)
		}
		if len(tc) > 0 {
			parsed = tc
			result.SawToolCallSyntax = true
			break
		}
	}
	if len(parsed) == 0 {
		parsed = parseXMLToolCalls(text)
		if len(parsed) == 0 {
			parsed = parseTextKVToolCalls(text)
			if len(parsed) == 0 {
				return result
			}
		}
		result.SawToolCallSyntax = true
	}

	calls, rejectedNames := filterToolCallsDetailed(parsed, availableToolNames)
	result.Calls = calls
	result.RejectedToolNames = rejectedNames
	result.RejectedByPolicy = len(rejectedNames) > 0 && len(calls) == 0
	return result
}
func ParseStandaloneToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	return ParseStandaloneToolCallsDetailed(text, availableToolNames).Calls
}

func ParseStandaloneToolCallsDetailed(text string, availableToolNames []string) ToolCallParseResult {
	result := ToolCallParseResult{}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return result
	}
	result.SawToolCallSyntax = looksLikeToolCallSyntax(trimmed)
	if shouldSkipToolCallParsingForCodeFenceExample(trimmed) {
		return result
	}
	candidates := buildToolCallCandidates(trimmed)
	var parsed []ParsedToolCall
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed = parseToolCallsPayload(candidate)
		if len(parsed) == 0 {
			parsed = parseXMLToolCalls(candidate)
		}
		if len(parsed) == 0 {
			parsed = parseMarkupToolCalls(candidate)
		}
		if len(parsed) == 0 {
			parsed = parseTextKVToolCalls(candidate)
		}
		if len(parsed) > 0 {
			break
		}
	}
	if len(parsed) == 0 {
		parsed = parseXMLToolCalls(trimmed)
		if len(parsed) == 0 {
			parsed = parseTextKVToolCalls(trimmed)
			if len(parsed) == 0 {
				return result
			}
		}
	}
	result.SawToolCallSyntax = true
	calls, rejectedNames := filterToolCallsDetailed(parsed, availableToolNames)
	result.Calls = calls
	result.RejectedToolNames = rejectedNames
	result.RejectedByPolicy = len(rejectedNames) > 0 && len(calls) == 0
	return result
}

func filterToolCallsDetailed(parsed []ParsedToolCall, availableToolNames []string) ([]ParsedToolCall, []string) {
	out := make([]ParsedToolCall, 0, len(parsed))
	for _, tc := range parsed {
		if tc.Name == "" {
			continue
		}
		if tc.Input == nil {
			tc.Input = map[string]any{}
		}
		out = append(out, tc)
	}
	return out, nil
}

func resolveAllowedToolName(name string, allowed map[string]struct{}, allowedCanonical map[string]string) string {
	return resolveAllowedToolNameWithLooseMatch(name, allowed, allowedCanonical)
}

func parseToolCallsPayload(payload string) []ParsedToolCall {
	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		// Try to repair backslashes first! Because LLMs often mix these two problems.
		repaired := repairInvalidJSONBackslashes(payload)
		// Try loose repair on top of that
		repaired = RepairLooseJSON(repaired)
		if err := json.Unmarshal([]byte(repaired), &decoded); err != nil {
			return nil
		}
	}
	switch v := decoded.(type) {
	case map[string]any:
		if tc, ok := v["tool_calls"]; ok {
			if isLikelyChatMessageEnvelope(v) {
				return nil
			}
			return parseToolCallList(tc)
		}
		if parsed, ok := parseToolCallItem(v); ok {
			return []ParsedToolCall{parsed}
		}
	case []any:
		return parseToolCallList(v)
	}
	return nil
}

func isLikelyChatMessageEnvelope(v map[string]any) bool {
	if v == nil {
		return false
	}
	if _, ok := v["tool_calls"]; !ok {
		return false
	}
	if role, ok := v["role"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "assistant", "tool", "user", "system":
			return true
		}
	}
	if _, ok := v["tool_call_id"]; ok {
		return true
	}
	if _, ok := v["content"]; ok {
		return true
	}
	return false
}

func looksLikeToolCallSyntax(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "tool_calls") ||
		strings.Contains(lower, "<tool_call") ||
		strings.Contains(lower, "<function_call") ||
		strings.Contains(lower, "<invoke") ||
		strings.Contains(lower, "function.name:")
}

func parseToolCallList(v any) []ParsedToolCall {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]ParsedToolCall, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if tc, ok := parseToolCallItem(m); ok {
			out = append(out, tc)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseToolCallItem(m map[string]any) (ParsedToolCall, bool) {
	name, _ := m["name"].(string)
	inputRaw, hasInput := m["input"]
	if fn, ok := m["function"].(map[string]any); ok {
		if name == "" {
			name, _ = fn["name"].(string)
		}
		if !hasInput {
			if v, ok := fn["arguments"]; ok {
				inputRaw = v
				hasInput = true
			}
		}
	}
	if !hasInput {
		for _, key := range []string{"arguments", "args", "parameters", "params"} {
			if v, ok := m[key]; ok {
				inputRaw = v
				hasInput = true
				break
			}
		}
	}
	if strings.TrimSpace(name) == "" {
		return ParsedToolCall{}, false
	}
	return ParsedToolCall{
		Name:  strings.TrimSpace(name),
		Input: parseToolCallInput(inputRaw),
	}, true
}
