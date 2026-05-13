package promptcompat

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/google/uuid"
)

var historySummaries = []string{
	"Prior conversation history and tool progress.",
	"Conversation context and previous interactions.",
	"Full discussion history for reference.",
	"Complete chat history with tool results.",
	"Previous conversation and tool usage.",
}

var separatorStyles = []string{
	"=== %d. %s ===",
	"--- %d. %s ---",
	"[%d. %s]",
	">>> %d. %s <<<",
}

func GenerateHistoryFilename() string {
	return uuid.New().String() + ".txt"
}

func BuildOpenAIHistoryTranscript(messages []any) string {
	return buildOpenAIHistoryTranscript(messages, "")
}

func BuildOpenAICurrentUserInputTranscript(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return buildOpenAIHistoryTranscript([]any{
		map[string]any{"role": "user", "content": text},
	}, "")
}

func BuildOpenAICurrentInputContextTranscript(messages []any, historyFilename string) string {
	return buildOpenAIHistoryTranscript(messages, historyFilename)
}

func buildOpenAIHistoryTranscript(messages []any, historyFilename string) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	if historyFilename != "" {
		b.WriteString("# " + historyFilename)
	} else {
		b.WriteString("# context")
	}
	b.WriteString("\n")
	b.WriteString(historySummaries[rand.Intn(len(historySummaries))])
	b.WriteString("\n\n")

	entry := 0
	separatorStyle := separatorStyles[rand.Intn(len(separatorStyles))]
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := normalizeOpenAIRoleForPrompt(strings.ToLower(strings.TrimSpace(asString(msg["role"]))))
		content := strings.TrimSpace(buildOpenAIHistoryEntry(role, msg))
		if content == "" {
			continue
		}
		entry++
		fmt.Fprintf(&b, separatorStyle+"\n%s\n\n", entry, strings.ToUpper(roleLabelForHistory(role)), content)
	}

	transcript := strings.TrimSpace(b.String())
	if transcript == "" {
		return ""
	}
	return transcript + "\n"
}

func buildOpenAIHistoryEntry(role string, msg map[string]any) string {
	switch role {
	case "assistant":
		return strings.TrimSpace(buildAssistantContentForPrompt(msg))
	case "tool", "function":
		return strings.TrimSpace(buildToolHistoryContent(msg))
	case "system", "user":
		return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	default:
		return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	}
}

func buildToolHistoryContent(msg map[string]any) string {
	content := strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	parts := make([]string, 0, 2)
	if name := strings.TrimSpace(asString(msg["name"])); name != "" {
		parts = append(parts, "name="+name)
	}
	if callID := strings.TrimSpace(asString(msg["tool_call_id"])); callID != "" {
		parts = append(parts, "tool_call_id="+callID)
	}
	header := ""
	if len(parts) > 0 {
		header = "[" + strings.Join(parts, " ") + "]"
	}
	switch {
	case header != "" && content != "":
		return header + "\n" + content
	case header != "":
		return header
	default:
		return content
	}
}

func roleLabelForHistory(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "function":
		return "tool"
	case "":
		return "unknown"
	default:
		return role
	}
}
