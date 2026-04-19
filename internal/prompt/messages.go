package prompt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var markdownImagePattern = regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)

const (
	beginSentenceMarker   = "<｜begin▁of▁sentence｜>"
	systemMarker          = "<｜System｜>"
	userMarker            = "<｜User｜>"
	assistantMarker       = "<｜Assistant｜>"
	toolMarker            = "<｜Tool｜>"
	endSentenceMarker     = "<｜end▁of▁sentence｜>"
	endToolResultsMarker  = "<｜end▁of▁toolresults｜>"
	endInstructionsMarker = "<｜end▁of▁instructions｜>"
)

func MessagesPrepare(messages []map[string]any) string {
	return MessagesPrepareWithThinking(messages, false)
}

func MessagesPrepareWithThinking(messages []map[string]any, thinkingEnabled bool) string {
	type block struct {
		Role string
		Text string
	}
	processed := make([]block, 0, len(messages))
	if thinkingEnabled {
		if instruction := buildConversationContinuityInstructions(thinkingEnabled); strings.TrimSpace(instruction) != "" {
			processed = append(processed, block{Role: "system", Text: instruction})
		}
	}
	for _, m := range messages {
		role, _ := m["role"].(string)
		text := NormalizeContent(m["content"])
		processed = append(processed, block{Role: role, Text: text})
	}
	if len(processed) == 0 {
		return ""
	}
	merged := make([]block, 0, len(processed))
	for _, msg := range processed {
		if len(merged) > 0 && merged[len(merged)-1].Role == msg.Role {
			merged[len(merged)-1].Text += "\n\n" + msg.Text
			continue
		}
		merged = append(merged, msg)
	}
	parts := make([]string, 0, len(merged)+2)
	parts = append(parts, beginSentenceMarker)
	lastRole := ""
	for _, m := range merged {
		lastRole = m.Role
		switch m.Role {
		case "assistant":
			parts = append(parts, formatRoleBlock(assistantMarker, m.Text, endSentenceMarker))
		case "tool":
			if strings.TrimSpace(m.Text) != "" {
				parts = append(parts, formatRoleBlock(toolMarker, m.Text, endToolResultsMarker))
			}
		case "system":
			if text := strings.TrimSpace(m.Text); text != "" {
				parts = append(parts, formatRoleBlock(systemMarker, text, endInstructionsMarker))
			}
		case "user":
			parts = append(parts, formatRoleBlock(userMarker, m.Text, ""))
		default:
			if strings.TrimSpace(m.Text) != "" {
				parts = append(parts, m.Text)
			}
		}
	}
	if lastRole != "assistant" {
		parts = append(parts, assistantMarker)
	}
	out := strings.Join(parts, "")
	return markdownImagePattern.ReplaceAllString(out, `[${1}](${2})`)
}

// formatRoleBlock produces a single concatenated block: marker + text + endMarker.
// No whitespace is inserted between marker and text so role boundaries stay
// compact and predictable for downstream parsers.
func formatRoleBlock(marker, text, endMarker string) string {
	out := marker + text
	if strings.TrimSpace(endMarker) != "" {
		out += endMarker
	}
	return out
}

func buildConversationContinuityInstructions(thinkingEnabled bool) string {
	lines := []string{
		"Continue the conversation from the full prior context and the latest tool results.",
		"Treat earlier messages as binding context; answer the user's current request as a continuation, not a restart.",
	}
	if thinkingEnabled {
		lines = append(lines, "Keep reasoning internal. Do not leave the final user-facing answer only in reasoning; always provide the answer in visible assistant content.")
	}
	return strings.Join(lines, "\n")
}

func NormalizeContent(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := m["type"].(string)
			typeStr = strings.ToLower(strings.TrimSpace(typeStr))
			if typeStr == "text" || typeStr == "output_text" || typeStr == "input_text" {
				if txt, ok := m["text"].(string); ok && txt != "" {
					parts = append(parts, txt)
					continue
				}
				if txt, ok := m["content"].(string); ok && txt != "" {
					parts = append(parts, txt)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
