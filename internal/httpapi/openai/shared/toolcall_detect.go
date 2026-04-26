package shared

import (
	"strings"

	"ds2api/internal/toolcall"
)

// DetectToolCallsWithThinkingFallback checks visible text first, then falls back
// to the thinking channel source when the model emitted canonical tool XML there.
// thinkingSource is used only for detection; thinkingExposed is what may be sent
// back to the client and will be cleaned only after a valid tool call is found.
func DetectToolCallsWithThinkingFallback(finalText, thinkingSource, thinkingExposed string, toolNames []string) (toolcall.ToolCallParseResult, string) {
	detected := toolcall.ParseStandaloneToolCallsDetailed(finalText, toolNames)
	if len(detected.Calls) > 0 {
		return detected, thinkingExposed
	}
	if !strings.Contains(thinkingSource, "<tool_calls") {
		return detected, thinkingExposed
	}
	thinkingDetected := toolcall.ParseStandaloneToolCallsDetailed(thinkingSource, toolNames)
	if len(thinkingDetected.Calls) == 0 {
		return detected, thinkingExposed
	}
	return thinkingDetected, CleanToolCallXML(thinkingExposed)
}
