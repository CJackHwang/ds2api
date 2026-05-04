package promptcompat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type readToolCacheEntry struct {
	Name       string
	Path       string
	CallID     string
	HasContent bool
}

func buildReadToolCacheHints(messagesRaw []any, toolNames []string) string {
	if len(messagesRaw) == 0 || !hasReadLikeTool(toolNames) {
		return ""
	}
	entries := collectReadToolCacheEntries(messagesRaw)
	if len(entries) == 0 {
		return ""
	}

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("Read-tool cache state:\n")
	b.WriteString("The files below were already read earlier in this conversation. If their tool result contains the full file body, use that existing content to make edits or analysis instead of calling the same read tool for the same path again. Only re-read when the user asks to refresh, the previous result explicitly lacked file content, or the path/content may have changed.\n")
	for _, key := range keys {
		entry := entries[key]
		if entry.Path == "" {
			continue
		}
		contentState := "content-present"
		if !entry.HasContent {
			contentState = "content-missing-or-empty"
		}
		line := fmt.Sprintf("- %s path=%q status=%s", entry.Name, entry.Path, contentState)
		if entry.CallID != "" {
			line += fmt.Sprintf(" tool_call_id=%q", entry.CallID)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func collectReadToolCacheEntries(messagesRaw []any) map[string]readToolCacheEntry {
	callsByID := map[string]readToolCacheEntry{}
	out := map[string]readToolCacheEntry{}
	for _, item := range messagesRaw {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		switch role {
		case "assistant":
			for _, entry := range readToolEntriesFromAssistant(msg["tool_calls"]) {
				if entry.CallID != "" {
					callsByID[entry.CallID] = entry
				}
				if entry.Path != "" {
					out[readToolCacheKey(entry.Name, entry.Path)] = entry
				}
			}
		case "tool", "function":
			callID := strings.TrimSpace(asString(msg["tool_call_id"]))
			entry, ok := callsByID[callID]
			if !ok {
				name := strings.TrimSpace(asString(msg["name"]))
				if !isReadLikeToolName(name) {
					continue
				}
				entry = readToolCacheEntry{Name: name, CallID: callID}
			}
			if entry.Path == "" {
				entry.Path = firstNonEmptyString(msg, "path", "file_path", "filename")
			}
			if entry.Path == "" {
				continue
			}
			entry.HasContent = strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"])) != ""
			out[readToolCacheKey(entry.Name, entry.Path)] = entry
		}
	}
	return out
}

func readToolEntriesFromAssistant(raw any) []readToolCacheEntry {
	calls, ok := raw.([]any)
	if !ok || len(calls) == 0 {
		return nil
	}
	out := make([]readToolCacheEntry, 0, len(calls))
	for _, item := range calls {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := readToolEntryFromCall(call)
		if entry.Name == "" || !isReadLikeToolName(entry.Name) || entry.Path == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func readToolEntryFromCall(call map[string]any) readToolCacheEntry {
	entry := readToolCacheEntry{CallID: strings.TrimSpace(asString(call["id"]))}
	name := strings.TrimSpace(asString(call["name"]))
	argsRaw := call["arguments"]
	if argsRaw == nil {
		argsRaw = call["input"]
	}
	if fn, _ := call["function"].(map[string]any); fn != nil {
		if name == "" {
			name = strings.TrimSpace(asString(fn["name"]))
		}
		if argsRaw == nil {
			argsRaw = fn["arguments"]
			if argsRaw == nil {
				argsRaw = fn["input"]
			}
		}
	}
	entry.Name = name
	entry.Path = extractReadToolPath(argsRaw)
	return entry
}

func extractReadToolPath(raw any) string {
	switch v := raw.(type) {
	case map[string]any:
		return firstNonEmptyString(v, "path", "file_path", "filename")
	case string:
		var parsed map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(v)), &parsed); err == nil {
			return firstNonEmptyString(parsed, "path", "file_path", "filename")
		}
	}
	return ""
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(asString(m[key])); value != "" {
			return value
		}
	}
	return ""
}

func isReadLikeToolName(name string) bool {
	switch normalizeToolNameForGuard(name) {
	case "read", "readfile":
		return true
	default:
		return false
	}
}

func readToolCacheKey(name, path string) string {
	return normalizeToolNameForGuard(name) + "\x00" + strings.TrimSpace(path)
}
