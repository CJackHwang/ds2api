package contextengine

import (
	"log/slog"
	"strings"
)

// MaybeShadow compiles a ContextPlan from messages and logs a structured
// summary when mode is "shadow". It is a strict no-op for any other mode so
// callers can always invoke it unconditionally.
//
// The function never modifies or blocks the normal prompt-build path: it runs
// after NormalizeOpenAIMessagesForPrompt returns and the original prompt
// string has already been produced. Any error during compilation is logged at
// Warn level and suppressed.
func MaybeShadow(mode string, messages []map[string]any, logger *slog.Logger) {
	if strings.ToLower(strings.TrimSpace(mode)) != "shadow" {
		return
	}
	if logger == nil {
		return
	}
	plan, err := Compile(CompileInput{Messages: messages})
	if err != nil {
		logger.Warn("[context_engine_shadow] compile error", "error", err)
		return
	}
	warnings := strings.Join(plan.Warnings, "; ")
	if warnings == "" {
		warnings = "none"
	}
	logger.Info("[context_engine_shadow]",
		"plan_id", plan.PlanID,
		"segments_included", len(plan.SegmentsIncluded),
		"segments_trimmed", len(plan.SegmentsTrimmed),
		"token_budget_used", plan.TokenBudget.Used,
		"token_budget_overflow", plan.TokenBudget.Overflow,
		"warnings", warnings,
	)
}
