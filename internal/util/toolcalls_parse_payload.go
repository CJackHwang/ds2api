package util

import (
	"encoding/json"
	"strings"
)

func parseToolCallsPayload(payload string) []ParsedToolCall {
	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		repaired := repairInvalidJSONBackslashes(payload)
		repaired = RepairLooseJSON(repaired)
		if err := json.Unmarshal([]byte(repaired), &decoded); err != nil {
			return nil
		}
	}
	switch v := decoded.(type) {
	case map[string]any:
		if tc, ok := v["tool_calls"]; ok {
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

func parseToolCallInput(v any) map[string]any {
	switch x := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return x
	case string:
		raw := strings.TrimSpace(x)
		if raw == "" {
			return map[string]any{}
		}
		normalizedRaw := escapeLiteralControlsInJSONStrings(raw)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(normalizedRaw), &parsed); err == nil && parsed != nil {
			normalizePathLikeFields(parsed)
			if hasPathFieldControlChars(parsed) {
				if repairedPath, ok := repairPathLikeEscapesInJSON(normalizedRaw); ok {
					var reparsed map[string]any
					if err := json.Unmarshal([]byte(repairedPath), &reparsed); err == nil && reparsed != nil {
						normalizePathLikeFields(reparsed)
						return reparsed
					}
				}
			}
			return parsed
		}
		if repairedPath, ok := repairPathLikeEscapesInJSON(normalizedRaw); ok {
			if err := json.Unmarshal([]byte(repairedPath), &parsed); err == nil && parsed != nil {
				normalizePathLikeFields(parsed)
				return parsed
			}
		}
		repaired := repairInvalidJSONBackslashes(normalizedRaw)
		if repaired != normalizedRaw {
			if err := json.Unmarshal([]byte(repaired), &parsed); err == nil && parsed != nil {
				normalizePathLikeFields(parsed)
				return parsed
			}
		}
		repairedLoose := RepairLooseJSON(normalizedRaw)
		if repairedLoose != normalizedRaw {
			if err := json.Unmarshal([]byte(repairedLoose), &parsed); err == nil && parsed != nil {
				normalizePathLikeFields(parsed)
				return parsed
			}
		}
		return map[string]any{"_raw": raw}
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return map[string]any{}
		}
		var parsed map[string]any
		if err := json.Unmarshal(b, &parsed); err == nil && parsed != nil {
			return parsed
		}
		return map[string]any{}
	}
}
