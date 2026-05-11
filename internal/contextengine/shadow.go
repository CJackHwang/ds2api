package contextengine

import (
	"log/slog"
	"strings"

	"ds2api/internal/util"
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
	// Warnings are produced by Compile and may incidentally echo segment
	// content (e.g. "segment {id=...} exceeded budget by N tokens"). Since
	// the PlanBuffer is exposed to admin clients via GET /admin/context-plans,
	// scrub sensitive fields before persisting. The in-flight Plan passed to
	// logging below is scrubbed by the same call.
	plan.Warnings = redactWarnings(plan.Warnings)
	GlobalPlanBuffer().Push(plan)
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

// redactWarnings runs each warning string through util.RedactSensitiveFields
// so that JSON-shaped fragments embedded in Compile diagnostics (api_key /
// Authorization Bearer / email / mobile / token-family) never surface through
// admin debug APIs or structured logs. Returns the input unchanged when it
// contains no entries, preserving nil vs empty-slice semantics for the
// PlanSummary JSON `omitempty` behaviour.
func redactWarnings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, len(in))
	for i, w := range in {
		out[i] = util.RedactSensitiveFields(w)
	}
	return out
}
