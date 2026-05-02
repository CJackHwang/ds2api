package chat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

type chatNonStreamResult struct {
	rawThinking           string
	rawText               string
	thinking              string
	toolDetectionThinking string
	text                  string
	contentFilter         bool
	detectedCalls         int
	body                  map[string]any
	finishReason          string
	responseMessageID     int
}

func (h *Handler) handleNonStreamWithRetry(w http.ResponseWriter, ctx context.Context, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession, keepSession bool) {
	attempts := 0
	currentResp := resp
	usagePrompt := finalPrompt
	accumulatedThinking := ""
	accumulatedRawThinking := ""
	accumulatedToolDetectionThinking := ""
	for {
		result, ok := h.collectChatNonStreamAttempt(w, currentResp, completionID, model, usagePrompt, thinkingEnabled, searchEnabled, toolNames, toolsRaw)
		if !ok {
			return
		}
		accumulatedThinking += sse.TrimContinuationOverlap(accumulatedThinking, result.thinking)
		accumulatedRawThinking += sse.TrimContinuationOverlap(accumulatedRawThinking, result.rawThinking)
		accumulatedToolDetectionThinking += sse.TrimContinuationOverlap(accumulatedToolDetectionThinking, result.toolDetectionThinking)
		result.thinking = accumulatedThinking
		result.rawThinking = accumulatedRawThinking
		result.toolDetectionThinking = accumulatedToolDetectionThinking
		detected := detectAssistantToolCalls(result.rawText, result.text, result.rawThinking, result.toolDetectionThinking, toolNames)
		result.detectedCalls = len(detected.Calls)
		result.body = openaifmt.BuildChatCompletionWithToolCalls(completionID, model, usagePrompt, result.thinking, result.text, detected.Calls, toolsRaw)
		addRefFileTokensToUsage(result.body, refFileTokens)
		result.finishReason = chatFinishReason(result.body)
		if !shouldRetryChatNonStream(result, attempts) {
			h.finishChatNonStreamResult(w, result, attempts, usagePrompt, refFileTokens, historySession, a.DeepSeekToken, keepSession)
			return
		}

		attempts++
		config.Logger.Info("[openai_empty_retry] attempting synthetic retry", "surface", "chat.completions", "stream", false, "retry_attempt", attempts, "parent_message_id", result.responseMessageID)
		retryPow, powErr := h.DS.GetPow(ctx, a, 3)
		if powErr != nil {
			config.Logger.Warn("[openai_empty_retry] retry PoW fetch failed, falling back to original PoW", "surface", "chat.completions", "stream", false, "retry_attempt", attempts, "error", powErr)
			retryPow = pow
		}
		retryPayload := clonePayloadForEmptyOutputRetry(payload, result.responseMessageID)
		nextResp, err := h.DS.CallCompletion(ctx, a, retryPayload, retryPow, 3)
		if err != nil {
			if historySession != nil {
				historySession.error(http.StatusInternalServerError, "Failed to get completion.", "error", result.thinking, result.text)
			}
			writeOpenAIError(w, http.StatusInternalServerError, "Failed to get completion.")
			config.Logger.Warn("[openai_empty_retry] retry request failed", "surface", "chat.completions", "stream", false, "retry_attempt", attempts, "error", err)
			return
		}
		usagePrompt = usagePromptWithEmptyOutputRetry(usagePrompt, attempts)
		currentResp = nextResp
	}
}

func (h *Handler) collectChatNonStreamAttempt(w http.ResponseWriter, resp *http.Response, completionID, model, usagePrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any) (chatNonStreamResult, bool) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, string(body))
		return chatNonStreamResult{}, false
	}
	result := sse.CollectStream(resp, thinkingEnabled, true)
	stripReferenceMarkers := h.compatStripReferenceMarkers()
	finalThinking := cleanVisibleOutput(result.Thinking, stripReferenceMarkers)
	finalText := cleanVisibleOutput(result.Text, stripReferenceMarkers)
	if searchEnabled {
		finalText = replaceCitationMarkersWithLinks(finalText, result.CitationLinks)
	}
	detected := detectAssistantToolCalls(result.Text, finalText, result.Thinking, result.ToolDetectionThinking, toolNames)
	respBody := openaifmt.BuildChatCompletionWithToolCalls(completionID, model, usagePrompt, finalThinking, finalText, detected.Calls, toolsRaw)
	return chatNonStreamResult{
		rawThinking:           result.Thinking,
		rawText:               result.Text,
		thinking:              finalThinking,
		toolDetectionThinking: result.ToolDetectionThinking,
		text:                  finalText,
		contentFilter:         result.ContentFilter,
		detectedCalls:         len(detected.Calls),
		body:                  respBody,
		finishReason:          chatFinishReason(respBody),
		responseMessageID:     result.ResponseMessageID,
	}, true
}

func (h *Handler) finishChatNonStreamResult(w http.ResponseWriter, result chatNonStreamResult, attempts int, usagePrompt string, refFileTokens int, historySession *chatHistorySession, token string, keepSession bool) {
	if result.detectedCalls == 0 && shouldWriteUpstreamEmptyOutputError(result.text) {
		status, message, code := upstreamEmptyOutputDetail(result.contentFilter, result.text, result.thinking)
		if historySession != nil {
			historySession.error(status, message, code, result.thinking, result.text)
		}
		writeUpstreamEmptyOutputError(w, result.text, result.thinking, result.contentFilter)
		config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "chat.completions", "stream", false, "retry_attempts", attempts, "success_source", "none", "content_filter", result.contentFilter)
		return
	}
	if keepSession && result.responseMessageID > 0 && token != "" {
		cache := GetSessionCache()
		cache.UpdateMessageIDs(token, result.responseMessageID, result.responseMessageID)
	}
	if historySession != nil {
		historySession.success(http.StatusOK, result.thinking, result.text, result.finishReason, openaifmt.BuildChatUsageForModel("", usagePrompt, result.thinking, result.text, refFileTokens))
	}
	writeJSON(w, http.StatusOK, result.body)
	source := "first_attempt"
	if attempts > 0 {
		source = "synthetic_retry"
	}
	config.Logger.Info("[openai_empty_retry] completed", "surface", "chat.completions", "stream", false, "retry_attempts", attempts, "success_source", source)
}

func chatFinishReason(respBody map[string]any) string {
	if choices, ok := respBody["choices"].([]map[string]any); ok && len(choices) > 0 {
		if fr, _ := choices[0]["finish_reason"].(string); strings.TrimSpace(fr) != "" {
			return fr
		}
	}
	return "stop"
}

func shouldRetryChatNonStream(result chatNonStreamResult, attempts int) bool {
	return emptyOutputRetryEnabled() &&
		attempts < emptyOutputRetryMaxAttempts() &&
		!result.contentFilter &&
		result.detectedCalls == 0 &&
		strings.TrimSpace(result.text) == ""
}

func (h *Handler) handleStreamWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession, keepSession bool) {
	streamRuntime, initialType, ok := h.prepareChatStreamRuntime(w, resp, completionID, model, finalPrompt, refFileTokens, thinkingEnabled, searchEnabled, toolNames, toolsRaw, historySession, keepSession, a.DeepSeekToken)
	if !ok {
		return
	}
	attempts := 0
	currentResp := resp
	for {
		terminalWritten, retryable := h.consumeChatStreamAttempt(r, currentResp, streamRuntime, initialType, thinkingEnabled, historySession, attempts < emptyOutputRetryMaxAttempts())
		if terminalWritten {
			logChatStreamTerminal(streamRuntime, attempts)
			return
		}
		if !retryable || !emptyOutputRetryEnabled() || attempts >= emptyOutputRetryMaxAttempts() {
			streamRuntime.finalize("stop", false)
			recordChatStreamHistory(streamRuntime, historySession)
			config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "chat.completions", "stream", true, "retry_attempts", attempts, "success_source", "none")
			return
		}
		attempts++
		config.Logger.Info("[openai_empty_retry] attempting synthetic retry", "surface", "chat.completions", "stream", true, "retry_attempt", attempts, "parent_message_id", streamRuntime.responseMessageID)
		retryPow, powErr := h.DS.GetPow(r.Context(), a, 3)
		if powErr != nil {
			config.Logger.Warn("[openai_empty_retry] retry PoW fetch failed, falling back to original PoW", "surface", "chat.completions", "stream", true, "retry_attempt", attempts, "error", powErr)
			retryPow = pow
		}
		nextResp, err := h.DS.CallCompletion(r.Context(), a, clonePayloadForEmptyOutputRetry(payload, streamRuntime.responseMessageID), retryPow, 3)
		if err != nil {
			failChatStreamRetry(streamRuntime, historySession, http.StatusInternalServerError, "Failed to get completion.", "error")
			config.Logger.Warn("[openai_empty_retry] retry request failed", "surface", "chat.completions", "stream", true, "retry_attempt", attempts, "error", err)
			return
		}
		if nextResp.StatusCode != http.StatusOK {
			defer func() { _ = nextResp.Body.Close() }()
			body, _ := io.ReadAll(nextResp.Body)
			failChatStreamRetry(streamRuntime, historySession, nextResp.StatusCode, string(body), "error")
			return
		}
		streamRuntime.finalPrompt = usagePromptWithEmptyOutputRetry(finalPrompt, attempts)
		currentResp = nextResp
	}
}

func (h *Handler) prepareChatStreamRuntime(w http.ResponseWriter, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession, keepSession bool, token string) (*chatStreamRuntime, string, bool) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return nil, "", false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[stream] response writer does not support flush; streaming may be buffered")
	}
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamRuntime := newChatStreamRuntime(
		w, rc, canFlush, completionID, time.Now().Unix(), model, finalPrompt,
		thinkingEnabled, searchEnabled, h.compatStripReferenceMarkers(), toolNames, toolsRaw,
		len(toolNames) > 0, h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence(),
		keepSession, token,
	)
	streamRuntime.refFileTokens = refFileTokens
	return streamRuntime, initialType, true
}

func (h *Handler) consumeChatStreamAttempt(r *http.Request, resp *http.Response, streamRuntime *chatStreamRuntime, initialType string, thinkingEnabled bool, historySession *chatHistorySession, allowDeferEmpty bool) (bool, bool) {
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
		OnKeepAlive: streamRuntime.sendKeepAlive,
		OnParsed: func(parsed sse.LineResult) streamengine.ParsedDecision {
			decision := streamRuntime.onParsed(parsed)
			if historySession != nil {
				historySession.progress(streamRuntime.thinking.String(), streamRuntime.text.String())
			}
			return decision
		},
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				finalReason = "content_filter"
			}
			if streamRuntime.keepSession && streamRuntime.responseMessageID > 0 {
				cache := GetSessionCache()
				cache.UpdateMessageIDs(streamRuntime.token, streamRuntime.responseMessageID, streamRuntime.responseMessageID)
			}
		},
		OnContextDone: func() {
			streamRuntime.markContextCancelled()
			if historySession != nil {
				historySession.stopped(streamRuntime.thinking.String(), streamRuntime.text.String(), string(streamengine.StopReasonContextCancelled))
			}
		},
	})
	if streamRuntime.finalErrorCode == string(streamengine.StopReasonContextCancelled) {
		return true, false
	}
	terminalWritten := streamRuntime.finalize(finalReason, allowDeferEmpty && finalReason != "content_filter")
	if terminalWritten {
		recordChatStreamHistory(streamRuntime, historySession)
		return true, false
	}
	return false, true
}

func recordChatStreamHistory(streamRuntime *chatStreamRuntime, historySession *chatHistorySession) {
	if historySession == nil {
		return
	}
	if streamRuntime.finalErrorMessage != "" {
		historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.thinking.String(), streamRuntime.text.String())
		return
	}
	historySession.success(http.StatusOK, streamRuntime.finalThinking, streamRuntime.finalText, streamRuntime.finalFinishReason, streamRuntime.finalUsage)
}

func failChatStreamRetry(streamRuntime *chatStreamRuntime, historySession *chatHistorySession, status int, message, code string) {
	streamRuntime.sendFailedChunk(status, message, code)
	if historySession != nil {
		historySession.error(status, message, code, streamRuntime.thinking.String(), streamRuntime.text.String())
	}
}

func logChatStreamTerminal(streamRuntime *chatStreamRuntime, attempts int) {
	source := "first_attempt"
	if attempts > 0 {
		source = "synthetic_retry"
	}
	if streamRuntime.finalErrorCode == string(streamengine.StopReasonContextCancelled) {
		config.Logger.Info("[openai_empty_retry] terminal cancelled", "surface", "chat.completions", "stream", true, "retry_attempts", attempts, "error_code", streamRuntime.finalErrorCode)
		return
	}
	if streamRuntime.finalErrorMessage != "" {
		config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "chat.completions", "stream", true, "retry_attempts", attempts, "success_source", "none", "error_code", streamRuntime.finalErrorCode)
		return
	}
	config.Logger.Info("[openai_empty_retry] completed", "surface", "chat.completions", "stream", true, "retry_attempts", attempts, "success_source", source)
}
