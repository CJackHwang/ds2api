package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

//nolint:unused // retained for native Gemini stream handling path.
func (h *Handler) handleStreamGenerateContent(w http.ResponseWriter, r *http.Request, resp *http.Response, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySessions ...*responsehistory.Session) {
	var historySession *responsehistory.Session
	if len(historySessions) > 0 {
		historySession = historySessions[0]
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	runtime := newGeminiStreamRuntime(w, rc, canFlush, model, finalPrompt, thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw, historySession)

	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OnParsed: runtime.onParsed,
		OnFinalize: func(_ streamengine.StopReason, _ error) {
			runtime.finalize(false)
		},
	})
}

//nolint:unused // retained for native Gemini stream handling path.
type geminiStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	model       string
	finalPrompt string

	thinkingEnabled       bool
	searchEnabled         bool
	bufferContent         bool
	stripReferenceMarkers bool
	toolNames             []string
	toolsRaw              any

	accumulator       *assistantturn.Accumulator
	contentFilter     bool
	responseMessageID int
	finalErrorStatus  int
	finalErrorMessage string
	finalErrorCode    string
	history           *responsehistory.Session
}

func (h *Handler) handleStreamGenerateContentWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow string, stdReq promptcompat.StandardRequest, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *responsehistory.Session) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	runtime := newGeminiStreamRuntime(w, rc, canFlush, model, finalPrompt, thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw, historySession)

	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:          "gemini.generate_content",
		Stream:           true,
		RetryEnabled:     true,
		MaxAttempts:      3,
		UsagePrompt:      finalPrompt,
		Request:          stdReq,
		CurrentInputFile: h.Store,
	}, completionruntime.StreamRetryHooks{
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) (bool, bool) {
			return h.consumeGeminiStreamAttempt(r.Context(), currentResp, runtime, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(_ int) {
			runtime.finalize(false)
		},
		ParentMessageID: func() int {
			return runtime.responseMessageID
		},
		OnRetryPrompt: func(prompt string) {
			runtime.finalPrompt = prompt
		},
		OnRetryFailure: func(status int, message, _ string) {
			runtime.sendErrorChunk(status, strings.TrimSpace(message))
		},
	})
}

func (h *Handler) consumeGeminiStreamAttempt(ctx context.Context, resp *http.Response, runtime *geminiStreamRuntime, thinkingEnabled bool, allowDeferEmpty bool) (bool, bool) {
	defer func() { _ = resp.Body.Close() }()
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             ctx,
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OnParsed: runtime.onParsed,
		OnFinalize: func(_ streamengine.StopReason, _ error) {
		},
	})
	terminalWritten := runtime.finalize(allowDeferEmpty)
	if terminalWritten {
		return true, false
	}
	return false, true
}

//nolint:unused // retained for native Gemini stream handling path.
func newGeminiStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	toolsRaw any,
	history *responsehistory.Session,
) *geminiStreamRuntime {
	return &geminiStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		model:                 model,
		finalPrompt:           finalPrompt,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		bufferContent:         len(toolNames) > 0,
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		history:               history,
		accumulator: assistantturn.NewAccumulator(assistantturn.AccumulatorOptions{
			ThinkingEnabled:       thinkingEnabled,
			SearchEnabled:         searchEnabled,
			StripReferenceMarkers: stripReferenceMarkers,
		}),
	}
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) sendChunk(payload map[string]any) {
	b, _ := json.Marshal(payload)
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *geminiStreamRuntime) sendErrorChunk(status int, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = http.StatusText(status)
	}
	errorStatus := "INVALID_ARGUMENT"
	switch status {
	case http.StatusUnauthorized:
		errorStatus = "UNAUTHENTICATED"
	case http.StatusForbidden:
		errorStatus = "PERMISSION_DENIED"
	case http.StatusTooManyRequests:
		errorStatus = "RESOURCE_EXHAUSTED"
	case http.StatusNotFound:
		errorStatus = "NOT_FOUND"
	default:
		if status >= 500 {
			errorStatus = "INTERNAL"
		}
	}
	s.sendChunk(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": msg,
			"status":  errorStatus,
		},
	})
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" || parsed.Stop {
		if parsed.ContentFilter {
			s.contentFilter = true
		}
		return streamengine.ParsedDecision{Stop: true}
	}

	accumulated := s.accumulator.Apply(parsed)
	for _, p := range accumulated.Parts {
		if p.Type == "thinking" {
			if p.VisibleText == "" || s.bufferContent {
				continue
			}
			s.sendChunk(map[string]any{
				"candidates": []map[string]any{
					{
						"index": 0,
						"content": map[string]any{
							"role":  "model",
							"parts": []map[string]any{{"text": p.VisibleText, "thought": true}},
						},
					},
				},
				"modelVersion": s.model,
			})
			continue
		}
		if p.RawText == "" || p.CitationOnly || p.VisibleText == "" {
			continue
		}
		if s.bufferContent {
			continue
		}
		s.sendChunk(map[string]any{
			"candidates": []map[string]any{
				{
					"index": 0,
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{{"text": p.VisibleText}},
					},
				},
			},
			"modelVersion": s.model,
		})
	}
	if s.history != nil {
		rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
		s.history.Progress(
			responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
			responsehistory.TextForArchive(rawText, text),
		)
	}
	return streamengine.ParsedDecision{ContentSeen: accumulated.ContentSeen}
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) finalize(deferEmptyOutput bool) bool {
	rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
	turn := assistantturn.BuildTurnFromStreamSnapshot(assistantturn.StreamSnapshot{
		RawText:           rawText,
		VisibleText:       text,
		RawThinking:       rawThinking,
		VisibleThinking:   thinking,
		DetectionThinking: detectionThinking,
		ContentFilter:     s.contentFilter,
		ResponseMessageID: s.responseMessageID,
	}, assistantturn.BuildOptions{
		Model:                 s.model,
		Prompt:                s.finalPrompt,
		SearchEnabled:         s.searchEnabled,
		StripReferenceMarkers: s.stripReferenceMarkers,
		ToolNames:             s.toolNames,
		ToolsRaw:              s.toolsRaw,
	})
	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
	if outcome.ShouldFail {
		if deferEmptyOutput {
			s.finalErrorStatus = outcome.Error.Status
			s.finalErrorMessage = outcome.Error.Message
			s.finalErrorCode = outcome.Error.Code
			return false
		}
		if s.history != nil {
			s.history.Error(outcome.Error.Status, outcome.Error.Message, outcome.Error.Code, responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), responsehistory.TextForArchive(turn.RawText, turn.Text))
		}
		s.sendErrorChunk(outcome.Error.Status, outcome.Error.Message)
		return true
	}
	if s.history != nil {
		s.history.Success(
			http.StatusOK,
			responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking),
			responsehistory.TextForArchive(turn.RawText, turn.Text),
			assistantturn.FinishReason(turn),
			responsehistory.GenericUsage(turn),
		)
	}

	if s.bufferContent {
		parts := buildGeminiPartsFromTurn(turn)
		s.sendChunk(map[string]any{
			"candidates": []map[string]any{
				{
					"index": 0,
					"content": map[string]any{
						"role":  "model",
						"parts": parts,
					},
				},
			},
			"modelVersion": s.model,
		})
	}

	s.sendChunk(map[string]any{
		"candidates": []map[string]any{
			{
				"index": 0,
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{
						{"text": ""},
					},
				},
				"finishReason": "STOP",
			},
		},
		"modelVersion": s.model,
		"usageMetadata": map[string]any{
			"promptTokenCount":     outcome.Usage.InputTokens,
			"candidatesTokenCount": outcome.Usage.OutputTokens,
			"totalTokenCount":      outcome.Usage.TotalTokens,
		},
	})
	return true
}
