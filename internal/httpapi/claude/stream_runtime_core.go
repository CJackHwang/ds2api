package claude

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

type claudeStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	model           string
	toolNames       []string
	messages        []any
	toolsRaw        any
	promptTokenText string

	thinkingEnabled       bool
	searchEnabled         bool
	bufferToolContent     bool
	stripReferenceMarkers bool

	messageID         string
	thinking          strings.Builder
	text              strings.Builder
	responseMessageID int

	sieve                 toolstream.State
	rawText               strings.Builder
	rawThinking           strings.Builder
	toolDetectionThinking strings.Builder
	toolCallsDetected     bool

	nextBlockIndex     int
	thinkingBlockOpen  bool
	thinkingBlockIndex int
	textBlockOpen      bool
	textBlockIndex     int
	textEmitted        bool
	ended              bool
	upstreamErr        string
	history            *responsehistory.Session
}

func newClaudeStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	model string,
	messages []any,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	toolsRaw any,
	promptTokenText string,
	history *responsehistory.Session,
) *claudeStreamRuntime {
	return &claudeStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		model:                 model,
		messages:              messages,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		bufferToolContent:     len(toolNames) > 0,
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		promptTokenText:       promptTokenText,
		history:               history,
		messageID:             fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		thinkingBlockIndex:    -1,
		textBlockIndex:        -1,
	}
}

func (s *claudeStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ErrorMessage != "" {
		s.upstreamErr = parsed.ErrorMessage
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("upstream_error")}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.Stop {
		return streamengine.ParsedDecision{Stop: true}
	}

	contentSeen := false
	for _, p := range parsed.ToolDetectionThinkingParts {
		trimmed := sse.TrimContinuationOverlapFromBuilder(&s.toolDetectionThinking, p.Text)
		if trimmed != "" {
			s.toolDetectionThinking.WriteString(trimmed)
		}
	}
	for _, p := range parsed.Parts {
		var rawTrimmed string
		if p.Type == "thinking" {
			rawTrimmed = sse.TrimContinuationOverlapFromBuilder(&s.rawThinking, p.Text)
		} else {
			rawTrimmed = sse.TrimContinuationOverlapFromBuilder(&s.rawText, p.Text)
		}
		if rawTrimmed == "" {
			continue
		}
		if p.Type == "thinking" {
			s.rawThinking.WriteString(rawTrimmed)
		} else {
			s.rawText.WriteString(rawTrimmed)
		}
		cleanedText := cleanVisibleOutput(rawTrimmed, s.stripReferenceMarkers)
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
			trimmed := sse.TrimContinuationOverlapFromBuilder(&s.thinking, cleanedText)
			if trimmed == "" {
				continue
			}
			s.thinking.WriteString(trimmed)
			s.closeTextBlock()
			if !s.thinkingBlockOpen {
				s.thinkingBlockIndex = s.nextBlockIndex
				s.nextBlockIndex++
				s.send("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": s.thinkingBlockIndex,
					"content_block": map[string]any{
						"type":     "thinking",
						"thinking": "",
					},
				})
				s.thinkingBlockOpen = true
			}
			s.send("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": s.thinkingBlockIndex,
				"delta": map[string]any{
					"type":     "thinking_delta",
					"thinking": trimmed,
				},
			})
			continue
		}

		s.text.WriteString(cleanedText)

		if !s.bufferToolContent {
			s.closeThinkingBlock()
			if !s.textBlockOpen {
				s.textBlockIndex = s.nextBlockIndex
				s.nextBlockIndex++
				s.send("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": s.textBlockIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				s.textBlockOpen = true
			}
			s.send("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": s.textBlockIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": cleanedText,
				},
			})
			s.textEmitted = true
			continue
		}

		events := toolstream.ProcessChunk(&s.sieve, rawTrimmed, s.toolNames)
		for _, evt := range events {
			if len(evt.ToolCalls) > 0 {
				s.closeTextBlock()
				s.toolCallsDetected = true
				normalized := toolcall.NormalizeParsedToolCallsForSchemas(evt.ToolCalls, s.toolsRaw)
				for _, tc := range normalized {
					idx := s.nextBlockIndex
					s.nextBlockIndex++
					s.sendToolUseBlock(idx, tc)
				}
				continue
			}
			if evt.Content == "" {
				continue
			}
			cleaned := cleanVisibleOutput(evt.Content, s.stripReferenceMarkers)
			if cleaned == "" || (s.searchEnabled && sse.IsCitation(cleaned)) {
				continue
			}
			s.closeThinkingBlock()
			if !s.textBlockOpen {
				s.textBlockIndex = s.nextBlockIndex
				s.nextBlockIndex++
				s.send("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": s.textBlockIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				s.textBlockOpen = true
			}
			s.send("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": s.textBlockIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": cleaned,
				},
			})
			s.textEmitted = true
		}
	}

	if s.history != nil {
		s.history.Progress(
			responsehistory.ThinkingForArchive(s.rawThinking.String(), s.toolDetectionThinking.String(), s.thinking.String()),
			responsehistory.TextForArchive(s.rawText.String(), s.text.String()),
		)
	}
	return streamengine.ParsedDecision{ContentSeen: contentSeen}
}
