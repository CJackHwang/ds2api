package util

import "strings"

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
	text = stripFencedCodeBlocks(text)
	if strings.TrimSpace(text) == "" {
		return result
	}
	result.SawToolCallSyntax = looksLikeToolCallSyntax(text)

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
	if looksLikeToolExampleContext(trimmed) {
		return result
	}
	result.SawToolCallSyntax = looksLikeToolCallSyntax(trimmed)
	for _, candidate := range []string{trimmed} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed := parseToolCallsPayload(candidate)
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
			result.SawToolCallSyntax = true
			calls, rejectedNames := filterToolCallsDetailed(parsed, availableToolNames)
			result.Calls = calls
			result.RejectedToolNames = rejectedNames
			result.RejectedByPolicy = len(rejectedNames) > 0 && len(calls) == 0
			return result
		}
	}
	return result
}

func looksLikeToolCallSyntax(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "tool_calls") ||
		strings.Contains(lower, "<tool_call") ||
		strings.Contains(lower, "<function_call") ||
		strings.Contains(lower, "<invoke") ||
		strings.Contains(lower, "function.name:")
}

func filterToolCallsDetailed(parsed []ParsedToolCall, availableToolNames []string) ([]ParsedToolCall, []string) {
	allowed := map[string]struct{}{}
	allowedCanonical := map[string]string{}
	for _, name := range availableToolNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
		lower := strings.ToLower(trimmed)
		if _, exists := allowedCanonical[lower]; !exists {
			allowedCanonical[lower] = trimmed
		}
	}
	if len(allowed) == 0 {
		rejectedSet := map[string]struct{}{}
		rejected := make([]string, 0, len(parsed))
		for _, tc := range parsed {
			if tc.Name == "" {
				continue
			}
			if _, ok := rejectedSet[tc.Name]; ok {
				continue
			}
			rejectedSet[tc.Name] = struct{}{}
			rejected = append(rejected, tc.Name)
		}
		return nil, rejected
	}
	out := make([]ParsedToolCall, 0, len(parsed))
	rejectedSet := map[string]struct{}{}
	rejected := make([]string, 0)
	for _, tc := range parsed {
		if tc.Name == "" {
			continue
		}
		matchedName := resolveAllowedToolName(tc.Name, allowed, allowedCanonical)
		if matchedName == "" {
			if _, ok := rejectedSet[tc.Name]; !ok {
				rejectedSet[tc.Name] = struct{}{}
				rejected = append(rejected, tc.Name)
			}
			continue
		}
		tc.Name = matchedName
		if tc.Input == nil {
			tc.Input = map[string]any{}
		}
		out = append(out, tc)
	}
	return out, rejected
}

func resolveAllowedToolName(name string, allowed map[string]struct{}, allowedCanonical map[string]string) string {
	return resolveAllowedToolNameWithLooseMatch(name, allowed, allowedCanonical)
}
