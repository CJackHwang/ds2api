package historyanalyzer

type RuleSpec struct {
	ID              RuleID   `json:"id"`
	Category        Category `json:"category"`
	DefaultSeverity Severity `json:"default_severity"`
	Description     string   `json:"description"`
	SuggestedAction string   `json:"suggested_action"`
}

const (
	RuleToolMarkerLeak              RuleID = "HA_TOOL_MARKER_LEAK"
	RuleToolCallAsText              RuleID = "HA_TOOL_CALL_AS_TEXT"
	RuleToolFalsePositive           RuleID = "HA_TOOL_FALSE_POSITIVE"
	RuleContextToolPairOrphan       RuleID = "HA_CONTEXT_TOOL_PAIR_ORPHAN"
	RuleContextReasoningBloat       RuleID = "HA_CONTEXT_REASONING_BLOAT"
	RuleContextCurrentInputMismatch RuleID = "HA_CONTEXT_CURRENT_INPUT_MISMATCH"
	RuleContinueCandidate           RuleID = "HA_CONTINUE_CANDIDATE"
	RuleCapabilitySearchThinking    RuleID = "HA_CAPABILITY_SEARCH_THINKING_CONFLICT"
	RuleAccountRetryRecovered       RuleID = "HA_ACCOUNT_RETRY_RECOVERED"
	RuleAccountRetryExhausted       RuleID = "HA_ACCOUNT_RETRY_EXHAUSTED"
)

var coreRuleSpecs = []RuleSpec{
	{
		ID:              RuleToolMarkerLeak,
		Category:        CategoryTool,
		DefaultSeverity: SeverityHigh,
		Description:     "Visible output contains DSML, tool wrapper, or DeepSeek control marker text.",
		SuggestedAction: "Add the sample to the parser leak corpus.",
	},
	{
		ID:              RuleToolCallAsText,
		Category:        CategoryTool,
		DefaultSeverity: SeverityHigh,
		Description:     "Upstream text looks like a tool call, but the rendered response has no structured tool_calls.",
		SuggestedAction: "Add a parser true-positive fixture.",
	},
	{
		ID:              RuleToolFalsePositive,
		Category:        CategoryTool,
		DefaultSeverity: SeverityHigh,
		Description:     "Markdown or XML example was likely mistaken for a tool call, or visible prose was dropped.",
		SuggestedAction: "Add a parser false-positive fixture.",
	},
	{
		ID:              RuleContextToolPairOrphan,
		Category:        CategoryContext,
		DefaultSeverity: SeverityHigh,
		Description:     "History or ContextPlan indicates a tool_call / tool_result pair is disconnected.",
		SuggestedAction: "Review the Context Engine tool pair policy.",
	},
	{
		ID:              RuleContextReasoningBloat,
		Category:        CategoryContext,
		DefaultSeverity: SeverityWarning,
		Description:     "Reasoning history appears large enough to crowd out the current request.",
		SuggestedAction: "Tune reasoning summary and retention policy.",
	},
	{
		ID:              RuleContextCurrentInputMismatch,
		Category:        CategoryContext,
		DefaultSeverity: SeverityHigh,
		Description:     "DS2API_HISTORY or DS2API_TOOLS current-input file evidence is missing or mismatched.",
		SuggestedAction: "Inspect current-input file injection and hash tracking.",
	},
	{
		ID:              RuleContinueCandidate,
		Category:        CategoryContinue,
		DefaultSeverity: SeverityWarning,
		Description:     "Output shows truncation, unclosed structures, or DeepSeek incomplete / auto-continue status.",
		SuggestedAction: "Feed the sample into Auto Continue shadow analysis.",
	},
	{
		ID:              RuleCapabilitySearchThinking,
		Category:        CategoryCapability,
		DefaultSeverity: SeverityWarning,
		Description:     "Search request includes thinking/current-input conflict signals.",
		SuggestedAction: "Review the Capability Router policy for this model profile.",
	},
	{
		ID:              RuleAccountRetryRecovered,
		Category:        CategoryAccountRuntime,
		DefaultSeverity: SeverityInfo,
		Description:     "Empty output or 429 recovered after retry or account switch.",
		SuggestedAction: "Track account health; no immediate failure required.",
	},
	{
		ID:              RuleAccountRetryExhausted,
		Category:        CategoryAccountRuntime,
		DefaultSeverity: SeverityHigh,
		Description:     "Retry or account switch was exhausted and the request still failed.",
		SuggestedAction: "Inspect account pool health and rate limits.",
	},
}

func KnownRuleSpecs() []RuleSpec {
	out := make([]RuleSpec, len(coreRuleSpecs))
	copy(out, coreRuleSpecs)
	return out
}

func RuleSpecByID(id RuleID) (RuleSpec, bool) {
	for _, spec := range coreRuleSpecs {
		if spec.ID == id {
			return spec, true
		}
	}
	return RuleSpec{}, false
}
