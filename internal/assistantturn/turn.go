package assistantturn

import (
	"net/http"
	"strings"

	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"ds2api/internal/util"
)

type StopReason string

const (
	StopReasonStop          StopReason = "stop"
	StopReasonToolCalls     StopReason = "tool_calls"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonError         StopReason = "error"
)

type Usage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	TotalTokens     int
}

type OutputError struct {
	Status  int
	Message string
	Code    string
}

type Turn struct {
	Model             string
	Prompt            string
	RawText           string
	RawThinking       string
	DetectionThinking string
	Text              string
	Thinking          string
	ToolCalls         []toolcall.ParsedToolCall
	ParsedToolCalls   toolcall.ToolCallParseResult
	CitationLinks     map[int]string
	ContentFilter     bool
	ResponseMessageID int
	StopReason        StopReason
	Usage             Usage
	Error             *OutputError
}

type FinalizeOptions struct {
	AlreadyEmittedToolCalls bool
}

type FinalOutcome struct {
	FinishReason     string
	Error            *OutputError
	Usage            Usage
	HasToolCalls     bool
	HasVisibleText   bool
	HasVisibleOutput bool
	ShouldFail       bool
}

type BuildOptions struct {
	Model                 string
	Prompt                string
	RefFileTokens         int
	SearchEnabled         bool
	StripReferenceMarkers bool
	ToolNames             []string
	ToolsRaw              any
	ToolChoice            promptcompat.ToolChoicePolicy
}

type StreamSnapshot struct {
	RawText               string
	VisibleText           string
	RawThinking           string
	VisibleThinking       string
	DetectionThinking     string
	ContentFilter         bool
	CitationLinks         map[int]string
	ResponseMessageID     int
	AlreadyEmittedCalls   bool
	AdditionalToolCalls   []toolcall.ParsedToolCall
	AlreadyEmittedToolRaw bool
}

func BuildTurnFromCollected(result sse.CollectResult, opts BuildOptions) Turn {
	thinking := shared.CleanVisibleOutput(result.Thinking, opts.StripReferenceMarkers)
	text := shared.CleanVisibleOutput(result.Text, opts.StripReferenceMarkers)
	if opts.SearchEnabled {
		text = shared.ReplaceCitationMarkersWithLinks(text, result.CitationLinks)
	}

	parsed := shared.DetectAssistantToolCalls(result.Text, text, result.Thinking, result.ToolDetectionThinking, opts.ToolNames)
	calls := toolcall.NormalizeParsedToolCallsForSchemas(parsed.Calls, opts.ToolsRaw)
	parsed.Calls = calls

	stopReason := StopReasonStop
	if result.ContentFilter {
		stopReason = StopReasonContentFilter
	}
	if len(calls) > 0 {
		stopReason = StopReasonToolCalls
	}

	turn := Turn{
		Model:             opts.Model,
		Prompt:            opts.Prompt,
		RawText:           result.Text,
		RawThinking:       result.Thinking,
		DetectionThinking: result.ToolDetectionThinking,
		Text:              text,
		Thinking:          thinking,
		ToolCalls:         calls,
		ParsedToolCalls:   parsed,
		CitationLinks:     result.CitationLinks,
		ContentFilter:     result.ContentFilter,
		ResponseMessageID: result.ResponseMessageID,
		StopReason:        stopReason,
	}
	turn.Usage = BuildUsage(opts.Model, opts.Prompt, thinking, text, opts.RefFileTokens)
	turn.Error = ValidateTurn(turn, opts.ToolChoice)
	if turn.Error != nil {
		turn.StopReason = StopReasonError
	}
	return turn
}

func BuildTurnFromStreamSnapshot(snapshot StreamSnapshot, opts BuildOptions) Turn {
	thinking := shared.CleanVisibleOutput(snapshot.VisibleThinking, opts.StripReferenceMarkers)
	text := shared.CleanVisibleOutput(snapshot.VisibleText, opts.StripReferenceMarkers)
	if opts.SearchEnabled {
		text = shared.ReplaceCitationMarkersWithLinks(text, snapshot.CitationLinks)
	}

	parsed := shared.DetectAssistantToolCalls(snapshot.RawText, text, snapshot.RawThinking, snapshot.DetectionThinking, opts.ToolNames)
	calls := parsed.Calls
	if len(calls) == 0 && len(snapshot.AdditionalToolCalls) > 0 {
		calls = snapshot.AdditionalToolCalls
	}
	calls = toolcall.NormalizeParsedToolCallsForSchemas(calls, opts.ToolsRaw)
	parsed.Calls = calls

	stopReason := StopReasonStop
	if snapshot.ContentFilter {
		stopReason = StopReasonContentFilter
	}
	if len(calls) > 0 || snapshot.AlreadyEmittedCalls || snapshot.AlreadyEmittedToolRaw {
		stopReason = StopReasonToolCalls
	}

	turn := Turn{
		Model:             opts.Model,
		Prompt:            opts.Prompt,
		RawText:           snapshot.RawText,
		RawThinking:       snapshot.RawThinking,
		DetectionThinking: snapshot.DetectionThinking,
		Text:              text,
		Thinking:          thinking,
		ToolCalls:         calls,
		ParsedToolCalls:   parsed,
		CitationLinks:     snapshot.CitationLinks,
		ContentFilter:     snapshot.ContentFilter,
		ResponseMessageID: snapshot.ResponseMessageID,
		StopReason:        stopReason,
	}
	turn.Usage = BuildUsage(opts.Model, opts.Prompt, thinking, text, opts.RefFileTokens)
	if !snapshot.AlreadyEmittedCalls && !snapshot.AlreadyEmittedToolRaw {
		turn.Error = ValidateTurn(turn, opts.ToolChoice)
	}
	if turn.Error != nil && len(calls) == 0 {
		turn.StopReason = StopReasonError
	}
	return turn
}

func BuildUsage(model, prompt, thinking, text string, refFileTokens int) Usage {
	inputTokens := util.CountPromptTokens(prompt, model) + refFileTokens
	reasoningTokens := util.CountOutputTokens(thinking, model)
	outputTokens := reasoningTokens + util.CountOutputTokens(text, model)
	return Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		TotalTokens:     inputTokens + outputTokens,
	}
}

func ValidateTurn(turn Turn, policy promptcompat.ToolChoicePolicy) *OutputError {
	if policy.IsRequired() && len(turn.ToolCalls) == 0 {
		return &OutputError{
			Status:  http.StatusUnprocessableEntity,
			Message: "tool_choice requires at least one valid tool call.",
			Code:    "tool_choice_violation",
		}
	}
	if len(turn.ToolCalls) > 0 {
		return nil
	}
	if strings.TrimSpace(turn.Text) != "" {
		return nil
	}
	status, message, code := UpstreamEmptyOutputDetail(turn.ContentFilter, turn.Text, turn.Thinking)
	return &OutputError{Status: status, Message: message, Code: code}
}

func UpstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string) {
	_ = text
	if contentFilter {
		return http.StatusBadRequest, "Upstream content filtered the response and returned no output.", "content_filter"
	}
	if strings.TrimSpace(thinking) != "" {
		return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned reasoning without visible output.", "upstream_empty_output"
	}
	return http.StatusServiceUnavailable, "Upstream service is unavailable and returned no output.", "upstream_unavailable"
}

// ShouldRetryEmptyOutput returns true when the turn produced no visible text
// and has no tool calls or content filter. This includes thinking-only responses,
// where the model returned reasoning but no answer — a retry may yield text.
func ShouldRetryEmptyOutput(turn Turn, attempts, maxAttempts int) bool {
	return attempts < maxAttempts &&
		!turn.ContentFilter &&
		len(turn.ToolCalls) == 0 &&
		strings.TrimSpace(turn.Text) == ""
}

func FinalizeTurn(turn Turn, opts FinalizeOptions) FinalOutcome {
	hasToolCalls := len(turn.ToolCalls) > 0 || opts.AlreadyEmittedToolCalls
	hasVisibleText := strings.TrimSpace(turn.Text) != ""
	hasVisibleThinking := strings.TrimSpace(turn.Thinking) != ""
	err := turn.Error
	if hasToolCalls {
		err = nil
	}
	finishReason := FinishReason(turn)
	if hasToolCalls {
		finishReason = "tool_calls"
	}
	return FinalOutcome{
		FinishReason:     finishReason,
		Error:            err,
		Usage:            turn.Usage,
		HasToolCalls:     hasToolCalls,
		HasVisibleText:   hasVisibleText,
		HasVisibleOutput: hasVisibleText || hasVisibleThinking || hasToolCalls,
		ShouldFail:       err != nil,
	}
}

func OpenAIChatUsage(turn Turn) map[string]any {
	return map[string]any{
		"prompt_tokens":     turn.Usage.InputTokens,
		"completion_tokens": turn.Usage.OutputTokens,
		"total_tokens":      turn.Usage.TotalTokens,
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": turn.Usage.ReasoningTokens,
		},
	}
}

func OpenAIResponsesUsage(turn Turn) map[string]any {
	return map[string]any{
		"input_tokens":  turn.Usage.InputTokens,
		"output_tokens": turn.Usage.OutputTokens,
		"total_tokens":  turn.Usage.TotalTokens,
	}
}

func FinishReason(turn Turn) string {
	switch turn.StopReason {
	case StopReasonToolCalls:
		return "tool_calls"
	case StopReasonContentFilter:
		return "content_filter"
	default:
		return "stop"
	}
}
