package responses

import (
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) handleResponsesStreamWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow, owner, responseID string, stdReq promptcompat.StandardRequest, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, toolChoice promptcompat.ToolChoicePolicy, traceID string, historySession *responsehistory.Session) {
	streamRuntime, initialType, ok := h.prepareResponsesStreamRuntime(w, resp, owner, responseID, model, finalPrompt, refFileTokens, thinkingEnabled, searchEnabled, toolNames, toolsRaw, toolChoice, traceID, historySession)
	if !ok {
		return
	}
	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:          "responses",
		Stream:           true,
		RetryEnabled:     emptyOutputRetryEnabled(),
		RetryMaxAttempts: emptyOutputRetryMaxAttempts(),
		MaxAttempts:      3,
		UsagePrompt:      finalPrompt,
		Request:          stdReq,
		CurrentInputFile: h.Store,
	}, completionruntime.StreamRetryHooks{
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) (bool, bool) {
			return h.consumeResponsesStreamAttempt(r, currentResp, streamRuntime, initialType, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(attempts int) {
			streamRuntime.finalize("stop", false)
			config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "responses", "stream", true, "retry_attempts", attempts, "success_source", "none", "error_code", streamRuntime.finalErrorCode)
		},
		ParentMessageID: func() int {
			return streamRuntime.responseMessageID
		},
		OnRetryPrompt: func(prompt string) {
			streamRuntime.finalPrompt = prompt
		},
		OnRetryFailure: func(status int, message, code string) {
			streamRuntime.failResponse(status, strings.TrimSpace(message), code)
		},
		OnTerminal: func(attempts int) {
			logResponsesStreamTerminal(streamRuntime, attempts)
		},
	})
}

func (h *Handler) prepareResponsesStreamRuntime(w http.ResponseWriter, resp *http.Response, owner, responseID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, toolChoice promptcompat.ToolChoicePolicy, traceID string, historySession *responsehistory.Session) (*responsesStreamRuntime, string, bool) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return nil, "", false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamRuntime := newResponsesStreamRuntime(
		w, rc, canFlush, responseID, model, finalPrompt, thinkingEnabled, searchEnabled,
		stripReferenceMarkersEnabled(), toolNames, toolsRaw, len(toolNames) > 0,
		h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence(),
		toolChoice, traceID, func(obj map[string]any) {
			h.getResponseStore().put(owner, responseID, obj)
		}, historySession,
	)
	streamRuntime.refFileTokens = refFileTokens
	streamRuntime.sendCreated()
	return streamRuntime, initialType, true
}

func (h *Handler) consumeResponsesStreamAttempt(r *http.Request, resp *http.Response, streamRuntime *responsesStreamRuntime, initialType string, thinkingEnabled bool, allowDeferEmpty bool) (bool, bool) {
	defer func() { _ = resp.Body.Close() }()
	finalReason := "stop"
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OnParsed: streamRuntime.onParsed,
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				finalReason = "content_filter"
			}
		},
		OnContextDone: func() {
			streamRuntime.markContextCancelled()
		},
	})
	if streamRuntime.finalErrorCode == string(streamengine.StopReasonContextCancelled) {
		return true, false
	}
	terminalWritten := streamRuntime.finalize(finalReason, allowDeferEmpty && finalReason != "content_filter")
	if terminalWritten {
		return true, false
	}
	return false, true
}

func logResponsesStreamTerminal(streamRuntime *responsesStreamRuntime, attempts int) {
	source := "first_attempt"
	if attempts > 0 {
		source = "synthetic_retry"
	}
	if streamRuntime.finalErrorCode == string(streamengine.StopReasonContextCancelled) {
		config.Logger.Info("[openai_empty_retry] terminal cancelled", "surface", "responses", "stream", true, "retry_attempts", attempts, "error_code", streamRuntime.finalErrorCode)
		return
	}
	if streamRuntime.failed {
		config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "responses", "stream", true, "retry_attempts", attempts, "success_source", "none", "error_code", streamRuntime.finalErrorCode)
		return
	}
	config.Logger.Info("[openai_empty_retry] completed", "surface", "responses", "stream", true, "retry_attempts", attempts, "success_source", source)
}
