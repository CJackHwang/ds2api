package chat

import (
	"encoding/json"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolstream"
)

type chatStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	completionID  string
	created       int64
	model         string
	finalPrompt   string
	refFileTokens int
	toolNames     []string
	toolsRaw      any
	toolChoice    promptcompat.ToolChoicePolicy

	thinkingEnabled       bool
	searchEnabled         bool
	stripReferenceMarkers bool

	firstChunkSent       bool
	bufferToolContent    bool
	emitEarlyToolDeltas  bool
	toolCallsEmitted     bool
	toolCallsDoneEmitted bool

	toolSieve         toolstream.State
	streamToolCallIDs map[int]string
	streamToolNames   map[int]string
	accumulator       shared.StreamAccumulator
	responseMessageID int

	finalThinking     string
	finalText         string
	finalFinishReason string
	finalUsage        map[string]any
	finalErrorStatus  int
	finalErrorMessage string
	finalErrorCode    string
}

type chatDeltaBatch struct {
	runtime *chatStreamRuntime
	field   string
	text    strings.Builder
}

func (b *chatDeltaBatch) append(field, text string) {
	if text == "" {
		return
	}
	if b.field != "" && b.field != field {
		b.flush()
	}
	b.field = field
	b.text.WriteString(text)
}

func (b *chatDeltaBatch) flush() {
	if b.field == "" || b.text.Len() == 0 {
		return
	}
	b.runtime.sendDelta(map[string]any{b.field: b.text.String()})
	b.field = ""
	b.text.Reset()
}

func newChatStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	completionID string,
	created int64,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	toolsRaw any,
	toolChoice promptcompat.ToolChoicePolicy,
	bufferToolContent bool,
	emitEarlyToolDeltas bool,
) *chatStreamRuntime {
	return &chatStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		completionID:          completionID,
		created:               created,
		model:                 model,
		finalPrompt:           finalPrompt,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		toolChoice:            toolChoice,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		stripReferenceMarkers: stripReferenceMarkers,
		bufferToolContent:     bufferToolContent,
		emitEarlyToolDeltas:   emitEarlyToolDeltas,
		streamToolCallIDs:     map[int]string{},
		streamToolNames:       map[int]string{},
		accumulator: shared.StreamAccumulator{
			ThinkingEnabled:       thinkingEnabled,
			SearchEnabled:         searchEnabled,
			StripReferenceMarkers: stripReferenceMarkers,
		},
	}
}

func (s *chatStreamRuntime) sendKeepAlive() {
	if !s.canFlush {
		return
	}
	_, _ = s.w.Write([]byte(": keep-alive\n\n"))
	_ = s.rc.Flush()
}

func (s *chatStreamRuntime) sendChunk(v any) {
	b, _ := json.Marshal(v)
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *chatStreamRuntime) sendDelta(delta map[string]any) {
	if len(delta) == 0 {
		return
	}
	if !s.firstChunkSent {
		delta["role"] = "assistant"
		s.firstChunkSent = true
	}
	s.sendChunk(openaifmt.BuildChatStreamChunk(
		s.completionID,
		s.created,
		s.model,
		[]map[string]any{openaifmt.BuildChatStreamDeltaChoice(0, delta)},
		nil,
	))
}

func (s *chatStreamRuntime) sendDone() {
	_, _ = s.w.Write([]byte("data: [DONE]\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *chatStreamRuntime) sendFailedChunk(status int, message, code string) {
	s.finalErrorStatus = status
	s.finalErrorMessage = message
	s.finalErrorCode = code
	s.sendChunk(map[string]any{
		"status_code": status,
		"error": map[string]any{
			"message": message,
			"type":    openAIErrorType(status),
			"code":    code,
			"param":   nil,
		},
	})
	s.sendDone()
}

func (s *chatStreamRuntime) markContextCancelled() {
	s.finalErrorStatus = 499
	s.finalErrorMessage = "request context cancelled"
	s.finalErrorCode = string(streamengine.StopReasonContextCancelled)
	s.finalThinking = s.accumulator.Thinking.String()
	s.finalText = cleanVisibleOutput(s.accumulator.Text.String(), s.stripReferenceMarkers)
	s.finalFinishReason = string(streamengine.StopReasonContextCancelled)
}

func (s *chatStreamRuntime) historyText() string {
	if s == nil {
		return ""
	}
	return historyTextForArchive(s.accumulator.RawText.String(), s.finalText)
}

func (s *chatStreamRuntime) historyThinking() string {
	if s == nil {
		return ""
	}
	return historyThinkingForArchive(
		s.accumulator.RawThinking.String(),
		s.accumulator.ToolDetectionThinking.String(),
		s.finalThinking,
	)
}

func (s *chatStreamRuntime) resetStreamToolCallState() {
	s.streamToolCallIDs = map[int]string{}
	s.streamToolNames = map[int]string{}
}

func (s *chatStreamRuntime) finalize(finishReason string, deferEmptyOutput bool) bool {
	s.finalErrorStatus = 0
	s.finalErrorMessage = ""
	s.finalErrorCode = ""
	finalThinking := s.accumulator.Thinking.String()
	finalToolDetectionThinking := s.accumulator.ToolDetectionThinking.String()
	finalText := s.accumulator.Text.String()
	turn := assistantturn.BuildTurnFromStreamSnapshot(assistantturn.StreamSnapshot{
		RawText:               s.accumulator.RawText.String(),
		VisibleText:           finalText,
		RawThinking:           s.accumulator.RawThinking.String(),
		VisibleThinking:       finalThinking,
		DetectionThinking:     finalToolDetectionThinking,
		ContentFilter:         finishReason == "content_filter",
		ResponseMessageID:     s.responseMessageID,
		AlreadyEmittedCalls:   s.toolCallsEmitted,
		AlreadyEmittedToolRaw: s.toolCallsDoneEmitted,
	}, assistantturn.BuildOptions{
		Model:                 s.model,
		Prompt:                s.finalPrompt,
		RefFileTokens:         s.refFileTokens,
		SearchEnabled:         s.searchEnabled,
		StripReferenceMarkers: s.stripReferenceMarkers,
		ToolNames:             s.toolNames,
		ToolsRaw:              s.toolsRaw,
		ToolChoice:            s.toolChoice,
	})
	s.finalThinking = turn.Thinking
	s.finalText = turn.Text
	if len(turn.ToolCalls) > 0 && !s.toolCallsDoneEmitted {
		s.sendDelta(map[string]any{
			"tool_calls": formatFinalStreamToolCallsWithStableIDs(turn.ToolCalls, s.streamToolCallIDs, s.toolsRaw),
		})
		s.toolCallsEmitted = true
		s.toolCallsDoneEmitted = true
	} else if s.bufferToolContent {
		batch := chatDeltaBatch{runtime: s}
		for _, evt := range toolstream.Flush(&s.toolSieve, s.toolNames) {
			if len(evt.ToolCalls) > 0 {
				batch.flush()
				s.toolCallsEmitted = true
				s.toolCallsDoneEmitted = true
				s.sendDelta(map[string]any{
					"tool_calls": formatFinalStreamToolCallsWithStableIDs(evt.ToolCalls, s.streamToolCallIDs, s.toolsRaw),
				})
				s.resetStreamToolCallState()
			}
			if evt.Content == "" {
				continue
			}
			cleaned := cleanVisibleOutput(evt.Content, s.stripReferenceMarkers)
			if cleaned == "" || (s.searchEnabled && sse.IsCitation(cleaned)) {
				continue
			}
			batch.append("content", cleaned)
		}
		batch.flush()
	}

	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{
		AlreadyEmittedToolCalls: s.toolCallsEmitted || s.toolCallsDoneEmitted,
	})
	if outcome.ShouldFail {
		status, message, code := outcome.Error.Status, outcome.Error.Message, outcome.Error.Code
		if deferEmptyOutput {
			s.finalErrorStatus = status
			s.finalErrorMessage = message
			s.finalErrorCode = code
			return false
		}
		s.sendFailedChunk(status, message, code)
		return true
	}
	usage := assistantturn.OpenAIChatUsage(turn)
	s.finalFinishReason = outcome.FinishReason
	s.finalUsage = usage
	s.sendChunk(openaifmt.BuildChatStreamChunk(
		s.completionID,
		s.created,
		s.model,
		[]map[string]any{openaifmt.BuildChatStreamFinishChoice(0, outcome.FinishReason)},
		usage,
	))
	s.sendDone()
	return true
}

func (s *chatStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.ContentFilter {
		if strings.TrimSpace(s.accumulator.Text.String()) == "" {
			return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("content_filter")}
		}
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}
	if parsed.ErrorMessage != "" {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("content_filter")}
	}
	if parsed.Stop {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}

	batch := chatDeltaBatch{runtime: s}
	accumulated := s.accumulator.Apply(parsed)
	for _, p := range accumulated.Parts {
		if p.Type == "thinking" {
			batch.append("reasoning_content", p.VisibleText)
			continue
		}
		if p.RawText == "" {
			continue
		}
		if p.CitationOnly {
			continue
		}
		if !s.bufferToolContent {
			batch.append("content", p.VisibleText)
		} else {
			events := toolstream.ProcessChunk(&s.toolSieve, p.RawText, s.toolNames)
			for _, evt := range events {
				if len(evt.ToolCallDeltas) > 0 {
					if !s.emitEarlyToolDeltas {
						continue
					}
					filtered := filterIncrementalToolCallDeltasByAllowed(evt.ToolCallDeltas, s.streamToolNames)
					if len(filtered) == 0 {
						continue
					}
					formatted := formatIncrementalStreamToolCallDeltas(filtered, s.streamToolCallIDs)
					if len(formatted) == 0 {
						continue
					}
					batch.flush()
					tcDelta := map[string]any{
						"tool_calls": formatted,
					}
					s.toolCallsEmitted = true
					s.sendDelta(tcDelta)
					continue
				}
				if len(evt.ToolCalls) > 0 {
					batch.flush()
					s.toolCallsEmitted = true
					s.toolCallsDoneEmitted = true
					tcDelta := map[string]any{
						"tool_calls": formatFinalStreamToolCallsWithStableIDs(evt.ToolCalls, s.streamToolCallIDs, s.toolsRaw),
					}
					s.sendDelta(tcDelta)
					s.resetStreamToolCallState()
					continue
				}
				if evt.Content != "" {
					cleaned := cleanVisibleOutput(evt.Content, s.stripReferenceMarkers)
					if cleaned == "" || (s.searchEnabled && sse.IsCitation(cleaned)) {
						continue
					}
					batch.append("content", cleaned)
				}
			}
		}
	}
	batch.flush()
	return streamengine.ParsedDecision{ContentSeen: accumulated.ContentSeen}
}
