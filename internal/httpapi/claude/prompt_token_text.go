package claude

import "ds2api/internal/promptcompat"

func buildClaudePromptTokenText(messages []any, thinkingEnabled bool) string {
	return promptcompat.BuildOpenAIMessagesOnlyPrompt(messages, "", thinkingEnabled, "off")
}
