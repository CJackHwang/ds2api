package openai

import (
	"ds2api/internal/toolcall"
	"encoding/json"
	"strings"
)

func parsedToolCallKey(tc toolcall.ParsedToolCall) string {
	name := strings.TrimSpace(tc.Name)
	if name == "" {
		return ""
	}
	argsBytes, _ := json.Marshal(tc.Input)
	return name + "::" + string(argsBytes)
}

func filterNewParsedToolCalls(calls []toolcall.ParsedToolCall, seen map[string]struct{}) []toolcall.ParsedToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]toolcall.ParsedToolCall, 0, len(calls))
	for _, tc := range calls {
		key := parsedToolCallKey(tc)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, tc)
	}
	return out
}
