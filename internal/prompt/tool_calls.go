package prompt

import (
	"encoding/json"
	"sort"
	"strings"
)

func FormatToolCallsForPrompt(raw any) string {
	calls, ok := raw.([]any)
	if !ok || len(calls) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(calls))
	for _, item := range calls {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := formatToolCallForPrompt(call)
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return "<|DSML|tool_calls>\n" + strings.Join(blocks, "\n") + "\n</|DSML|tool_calls>"
}

func StringifyToolCallArguments(v any) string {
	switch x := v.(type) {
	case nil:
		return "{}"
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "{}"
		}
		s = normalizeToolArgumentString(s)
		if s == "" {
			return "{}"
		}
		return s
	default:
		b, err := json.Marshal(x)
		if err != nil || len(b) == 0 {
			return "{}"
		}
		return string(b)
	}
}

func formatToolCallForPrompt(call map[string]any) string {
	if call == nil {
		return ""
	}

	name := strings.TrimSpace(asString(call["name"]))
	fn, _ := call["function"].(map[string]any)
	if name == "" && fn != nil {
		name = strings.TrimSpace(asString(fn["name"]))
	}
	if name == "" {
		return ""
	}

	argsRaw := call["arguments"]
	if argsRaw == nil {
		argsRaw = call["input"]
	}
	if argsRaw == nil && fn != nil {
		argsRaw = fn["arguments"]
		if argsRaw == nil {
			argsRaw = fn["input"]
		}
	}

	parameters := formatToolCallParametersForPrompt(argsRaw)
	if parameters == "" {
		return `  <|DSML|invoke name="` + escapeDSMLAttribute(name) + `"></|DSML|invoke>`
	}

	return "  <|DSML|invoke name=\"" + escapeDSMLAttribute(name) + "\">\n" +
		parameters + "\n" +
		"  </|DSML|invoke>"
}

func formatToolCallParametersForPrompt(raw any) string {
	value := normalizePromptToolCallValue(raw)
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return ""
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, renderPromptParameterNode(key, v[key], "    "))
		}
		return strings.Join(lines, "\n")
	case []any:
		return `    <|DSML|parameter name="items" string="false">` + mustMarshalJSON(v) + `</|DSML|parameter>`
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		return `    <|DSML|parameter name="content" string="true">` + escapeDSMLText(v) + `</|DSML|parameter>`
	case nil:
		return ""
	default:
		return `    <|DSML|parameter name="value" string="false">` + mustMarshalJSON(v) + `</|DSML|parameter>`
	}
}

func renderPromptParameterNode(name string, value any, indent string) string {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return ""
	}

	switch v := value.(type) {
	case nil:
		return indent + `<|DSML|parameter name="` + escapeDSMLAttribute(trimmedName) + `" string="false">null</|DSML|parameter>`
	case string:
		return indent + `<|DSML|parameter name="` + escapeDSMLAttribute(trimmedName) + `" string="true">` + escapeDSMLText(v) + `</|DSML|parameter>`
	default:
		return indent + `<|DSML|parameter name="` + escapeDSMLAttribute(trimmedName) + `" string="false">` + mustMarshalJSON(v) + `</|DSML|parameter>`
	}
}

func normalizePromptToolCallValue(raw any) any {
	switch x := raw.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return ""
		}
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
		return x
	default:
		return x
	}
}

func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(b)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func escapeDSMLText(v string) string {
	if v == "" {
		return ""
	}
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(v)
}

func escapeDSMLAttribute(v string) string {
	if v == "" {
		return ""
	}
	return strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(v)
}

func normalizeToolArgumentString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if looksLikeConcatenatedJSON(trimmed) {
		return raw
	}
	return trimmed
}

func looksLikeConcatenatedJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "}{") || strings.Contains(trimmed, "][") {
		return true
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	var first any
	if err := dec.Decode(&first); err != nil {
		return false
	}
	var second any
	return dec.Decode(&second) == nil
}
