package responses

import (
	"ds2api/internal/toolcall"
	"net/http"
	"strings"

	"ds2api/internal/config"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolstream"
)

type responsesStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	responseID  string
	model       string
	finalPrompt string
	toolNames   []string
	traceID     string
	toolChoice  promptcompat.ToolChoicePolicy

	thinkingEnabled       bool
	searchEnabled         bool
	stripReferenceMarkers bool

	bufferToolContent    bool
	emitEarlyToolDeltas  bool
	toolCallsEmitted     bool
	toolCallsDoneEmitted bool

	sieve             toolstream.State
	thinking          strings.Builder
	text              strings.Builder
	visibleText       strings.Builder
	streamToolCallIDs map[int]string
	functionItemIDs   map[int]string
	functionOutputIDs map[int]int
	functionArgs      map[int]string
	functionDone      map[int]bool
	functionAdded     map[int]bool
	functionNames     map[int]string
	messageItemID     string
	messageOutputID   int
	nextOutputID      int
	messageAdded      bool
	messagePartAdded  bool
	sequence          int
	failed            bool

	persistResponse func(obj map[string]any)
}

func newResponsesStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	responseID string,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	bufferToolContent bool,
	emitEarlyToolDeltas bool,
	toolChoice promptcompat.ToolChoicePolicy,
	traceID string,
	persistResponse func(obj map[string]any),
) *responsesStreamRuntime {
	return &responsesStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		responseID:            responseID,
		model:                 model,
		finalPrompt:           finalPrompt,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		bufferToolContent:     bufferToolContent,
		emitEarlyToolDeltas:   emitEarlyToolDeltas,
		streamToolCallIDs:     map[int]string{},
		functionItemIDs:       map[int]string{},
		functionOutputIDs:     map[int]int{},
		functionArgs:          map[int]string{},
		functionDone:          map[int]bool{},
		functionAdded:         map[int]bool{},
		functionNames:         map[int]string{},
		messageOutputID:       -1,
		toolChoice:            toolChoice,
		traceID:               traceID,
		persistResponse:       persistResponse,
	}
}

func (s *responsesStreamRuntime) failResponse(status int, message, code string) {
	s.failed = true
	failedResp := map[string]any{
		"id":          s.responseID,
		"type":        "response",
		"object":      "response",
		"model":       s.model,
		"status":      "failed",
		"status_code": status,
		"output":      []any{},
		"output_text": "",
		"error": map[string]any{
			"message": message,
			"type":    openAIErrorType(status),
			"code":    code,
			"param":   nil,
		},
	}
	if s.persistResponse != nil {
		s.persistResponse(failedResp)
	}
	s.sendEvent("response.failed", openaifmt.BuildResponsesFailedPayload(s.responseID, s.model, status, message, code))
	s.sendDone()
}

func (s *responsesStreamRuntime) finalize() {
	finalThinking := s.thinking.String()
	finalText := cleanVisibleOutput(s.text.String(), s.stripReferenceMarkers)

	if s.bufferToolContent {
		s.processToolStreamEvents(toolstream.Flush(&s.sieve, s.toolNames), true, true)
	}

	textParsed := toolcall.ParseStandaloneToolCallsDetailed(finalText, s.toolNames)
	detected := textParsed.Calls
	s.logToolPolicyRejections(textParsed)

	// Fallback: if text has no tool calls, check thinking content.
	if len(detected) == 0 && strings.Contains(finalThinking, "<tool_calls") {
		thinkingParsed := toolcall.ParseStandaloneToolCallsDetailed(finalThinking, s.toolNames)
		if len(thinkingParsed.Calls) > 0 {
			detected = thinkingParsed.Calls
			finalThinking = shared.CleanToolCallXML(finalThinking)
		}
	}

	if len(detected) > 0 {
		s.toolCallsEmitted = true
		if !s.toolCallsDoneEmitted {
			s.emitFunctionCallDoneEvents(detected)
		}
	}

	s.closeMessageItem()

	if s.toolChoice.IsRequired() && len(detected) == 0 {
		s.failResponse(http.StatusUnprocessableEntity, "tool_choice requires at least one valid tool call.", "tool_choice_violation")
		return
	}
	if len(detected) == 0 && strings.TrimSpace(finalText) == "" {
		status, message, code := upstreamEmptyOutputDetail(false, finalText, finalThinking)
		s.failResponse(status, message, code)
		return
	}
	s.closeIncompleteFunctionItems()

	obj := s.buildCompletedResponseObject(finalThinking, finalText, detected)
	if s.persistResponse != nil {
		s.persistResponse(obj)
	}
	s.sendEvent("response.completed", openaifmt.BuildResponsesCompletedPayload(obj))
	s.sendDone()
}

func (s *responsesStreamRuntime) logToolPolicyRejections(textParsed toolcall.ToolCallParseResult) {
	logRejected := func(parsed toolcall.ToolCallParseResult, channel string) {
		rejected := filteredRejectedToolNamesForLog(parsed.RejectedToolNames)
		if !parsed.RejectedByPolicy || len(rejected) == 0 {
			return
		}
		config.Logger.Warn(
			"[responses] rejected tool calls by policy",
			"trace_id", strings.TrimSpace(s.traceID),
			"channel", channel,
			"tool_choice_mode", s.toolChoice.Mode,
			"rejected_tool_names", strings.Join(rejected, ","),
		)
	}
	logRejected(textParsed, "text")
}

func (s *responsesStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" || parsed.Stop {
		return streamengine.ParsedDecision{Stop: true}
	}

	contentSeen := false
	for _, p := range parsed.Parts {
		cleanedText := cleanVisibleOutput(p.Text, s.stripReferenceMarkers)
		if cleanedText == "" {
			continue
		}
		if p.Type != "thinking" && s.searchEnabled && sse.IsCitation(cleanedText) {
			continue
		}
		contentSeen = true
		if p.Type == "thinking" {
			if !s.thinkingEnabled {
				continue
			}
			trimmed := sse.TrimContinuationOverlap(s.thinking.String(), cleanedText)
			if trimmed == "" {
				continue
			}
			s.thinking.WriteString(trimmed)
			s.sendEvent("response.reasoning.delta", openaifmt.BuildResponsesReasoningDeltaPayload(s.responseID, trimmed))
			continue
		}

		trimmed := sse.TrimContinuationOverlap(s.text.String(), cleanedText)
		if trimmed == "" {
			continue
		}
		s.text.WriteString(trimmed)
		if !s.bufferToolContent {
			s.emitTextDelta(trimmed)
			continue
		}
		s.processToolStreamEvents(toolstream.ProcessChunk(&s.sieve, trimmed, s.toolNames), true, true)
	}

	return streamengine.ParsedDecision{ContentSeen: contentSeen}
}
