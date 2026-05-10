package promptcompat

import (
	"os"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/contextengine"
	"ds2api/internal/prompt"
)

func buildOpenAIFinalPrompt(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
	return BuildOpenAIPrompt(messagesRaw, toolsRaw, traceID, DefaultToolChoicePolicy(), thinkingEnabled)
}

func BuildOpenAIPrompt(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool) (string, []string) {
	messages := NormalizeOpenAIMessagesForPrompt(messagesRaw, traceID)
	contextengine.MaybeShadow(contextEngineMode(), messages, config.Logger)
	toolNames := []string{}
	if tools, ok := toolsRaw.([]any); ok && len(tools) > 0 {
		messages, toolNames = injectToolPrompt(messages, tools, toolPolicy)
	}
	return prompt.MessagesPrepareWithThinking(messages, thinkingEnabled), toolNames
}

// contextEngineMode reads the context engine feature flag.
// DS2API_CONTEXT_ENGINE env var takes precedence (consistent with Store.ContextEngineMode).
func contextEngineMode() string {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_CONTEXT_ENGINE"))); raw != "" {
		switch raw {
		case "shadow", "enforce":
			return raw
		case "off":
			return "off"
		}
	}
	return "off"
}

// BuildOpenAIPromptForAdapter exposes the OpenAI-compatible prompt building flow so
// other protocol adapters (for example Gemini) can reuse the same tool/history
// normalization logic and remain behavior-compatible with chat/completions.
func BuildOpenAIPromptForAdapter(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
	return buildOpenAIFinalPrompt(messagesRaw, toolsRaw, traceID, thinkingEnabled)
}
