package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"ds2api/internal/util"
)

func injectToolPrompt(messages []map[string]any, tools []any, policy util.ToolChoicePolicy) ([]map[string]any, []string) {
	if policy.IsNone() {
		return messages, nil
	}
	toolSchemas := make([]string, 0, len(tools))
	names := make([]string, 0, len(tools))
	isAllowed := func(name string) bool {
		if strings.TrimSpace(name) == "" {
			return false
		}
		if len(policy.Allowed) == 0 {
			return true
		}
		_, ok := policy.Allowed[name]
		return ok
	}

	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if len(fn) == 0 {
			fn = tool
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		schema, _ := fn["parameters"].(map[string]any)
		name = strings.TrimSpace(name)
		if !isAllowed(name) {
			continue
		}
		names = append(names, name)
		if desc == "" {
			desc = "No description available"
		}
		b, _ := json.Marshal(schema)
		toolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, string(b)))
	}
	if len(toolSchemas) == 0 {
		return messages, names
	}
	toolPrompt := "You have access to these tools:\n\n" + strings.Join(toolSchemas, "\n\n") + "\n\nWhen you need to use tools, output ONLY this JSON object format:\n{\"tool_calls\": [{\"name\": \"tool_name\", \"input\": {\"param\": \"value\"}}]}\n\n【EXAMPLE】\nUser: Please check the weather in Beijing and Shanghai, and update my todo list.\nAssistant:\n{\"tool_calls\": [\n  {\"name\": \"get_weather\", \"input\": {\"city\": \"Beijing\"}},\n  {\"name\": \"get_weather\", \"input\": {\"city\": \"Shanghai\"}},\n  {\"name\": \"update_todo\", \"input\": {\"todos\": [{\"content\": \"Buy milk\"}, {\"content\": \"Write report\"}]}}\n]}\n\nIMPORTANT:\n1) If calling tools, output ONLY the JSON object above. Do NOT include any extra text.\n2) Do NOT wrap tool-call JSON in markdown/code fences (for example, do not use triple backticks).\n3) After receiving a tool result, you MUST use it to produce the final answer.\n4) Only call another tool when the previous result is missing required data or returned an error.\n5) JSON SYNTAX STRICTLY REQUIRED: All property names MUST be enclosed in double quotes (e.g., \"name\", not name).\n6) ARRAY FORMAT: If providing a list of items, you MUST enclose them in square brackets `[]` (e.g., \"todos\": [{\"item\": \"a\"}, {\"item\": \"b\"}]). DO NOT output comma-separated objects without brackets."
	if policy.Mode == util.ToolChoiceRequired {
		toolPrompt += "\n7) For this response, you MUST call at least one tool from the allowed list."
	}
	if policy.Mode == util.ToolChoiceForced && strings.TrimSpace(policy.ForcedName) != "" {
		toolPrompt += "\n7) For this response, you MUST call exactly this tool name: " + strings.TrimSpace(policy.ForcedName)
		toolPrompt += "\n8) Do not call any other tool."
	}

	for i := range messages {
		if messages[i]["role"] == "system" {
			old, _ := messages[i]["content"].(string)
			messages[i]["content"] = strings.TrimSpace(old + "\n\n" + toolPrompt)
			return messages, names
		}
	}
	messages = append([]map[string]any{{"role": "system", "content": toolPrompt}}, messages...)
	return messages, names
}

func formatIncrementalStreamToolCallDeltas(deltas []toolCallDelta, ids map[int]string) []map[string]any {
	if len(deltas) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(deltas))
	for _, d := range deltas {
		if d.Name == "" && d.Arguments == "" {
			continue
		}
		callID, ok := ids[d.Index]
		if !ok || callID == "" {
			callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			ids[d.Index] = callID
		}
		item := map[string]any{
			"index": d.Index,
			"id":    callID,
			"type":  "function",
		}
		fn := map[string]any{}
		if d.Name != "" {
			fn["name"] = d.Name
		}
		if d.Arguments != "" {
			fn["arguments"] = d.Arguments
		}
		if len(fn) > 0 {
			item["function"] = fn
		}
		out = append(out, item)
	}
	return out
}

func filterIncrementalToolCallDeltasByAllowed(deltas []toolCallDelta, allowedNames []string, seenNames map[int]string) []toolCallDelta {
	if len(deltas) == 0 {
		return nil
	}
	out := make([]toolCallDelta, 0, len(deltas))
	for _, d := range deltas {
		if d.Name != "" {
			if seenNames != nil {
				seenNames[d.Index] = d.Name
			}
			out = append(out, d)
			continue
		}
		if seenNames == nil {
			out = append(out, d)
			continue
		}
		name := strings.TrimSpace(seenNames[d.Index])
		if name == "" {
			continue
		}
		out = append(out, d)
	}
	return out
}

func formatFinalStreamToolCallsWithStableIDs(calls []util.ParsedToolCall, ids map[int]string) []map[string]any {
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for i, c := range calls {
		callID := ""
		if ids != nil {
			callID = strings.TrimSpace(ids[i])
		}
		if callID == "" {
			callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if ids != nil {
				ids[i] = callID
			}
		}
		args, _ := json.Marshal(c.Input)
		out = append(out, map[string]any{
			"index": i,
			"id":    callID,
			"type":  "function",
			"function": map[string]any{
				"name":      c.Name,
				"arguments": string(args),
			},
		})
	}
	return out
}
