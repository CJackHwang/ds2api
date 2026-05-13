package chat

import (
	"context"
	"io"
	"net/http"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) handleNonStreamWithRetry(w http.ResponseWriter, ctx context.Context, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	stdReq := promptcompat.StandardRequest{
		Surface:         "chat.completions",
		ResponseModel:   model,
		PromptTokenText: finalPrompt,
		FinalPrompt:     finalPrompt,
		RefFileTokens:   refFileTokens,
		Thinking:        thinkingEnabled,
		Search:          searchEnabled,
		ToolNames:       toolNames,
		ToolsRaw:        toolsRaw,
		ToolChoice:      promptcompat.DefaultToolChoicePolicy(),
	}
	retryEnabled := h != nil && h.DS != nil && emptyOutputRetryEnabled()
	result, outErr := completionruntime.ExecuteNonStreamStartedWithRetry(ctx, h.DS, a, completionruntime.StartResult{
		SessionID: completionID,
		Payload:   payload,
		Pow:       pow,
		Response:  resp,
		Request:   stdReq,
	}, completionruntime.Options{
		RetryEnabled:     retryEnabled,
		RetryMaxAttempts: emptyOutputRetryMaxAttempts(),
	})
	if outErr != nil {
		if historySession != nil {
			historySession.error(outErr.Status, outErr.Message, outErr.Code, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text))
		}
		writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
		return
	}
	respBody := openaifmt.BuildChatCompletionWithToolCalls(result.SessionID, model, result.Turn.Prompt, result.Turn.Thinking, result.Turn.Text, result.Turn.ToolCalls, toolsRaw)
	respBody["usage"] = assistantturn.OpenAIChatUsage(result.Turn)
	outcome := assistantturn.FinalizeTurn(result.Turn, assistantturn.FinalizeOptions{})
	if historySession != nil {
		historySession.success(http.StatusOK, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), outcome.FinishReason, assistantturn.OpenAIChatUsage(result.Turn))
	}
	writeJSON(w, http.StatusOK, respBody)
}

func (h *Handler) handleStreamWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow, completionID string, sessionIDRef *string, stdReq promptcompat.StandardRequest, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, toolChoice promptcompat.ToolChoicePolicy, historySession *chatHistorySession) {
	streamRuntime, initialType, ok := h.prepareChatStreamRuntime(w, resp, completionID, model, finalPrompt, refFileTokens, thinkingEnabled, searchEnabled, toolNames, toolsRaw, toolChoice, historySession)
	if !ok {
		return
	}
	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:          "chat.completions",
		Stream:           true,
		RetryEnabled:     emptyOutputRetryEnabled(),
		RetryMaxAttempts: emptyOutputRetryMaxAttempts(),
		MaxAttempts:      3,
		UsagePrompt:      finalPrompt,
		Request:          stdReq,
		CurrentInputFile: h.Store,
	}, completionruntime.StreamRetryHooks{
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) (bool, bool) {
			return h.consumeChatStreamAttempt(r, currentResp, streamRuntime, initialType, thinkingEnabled, historySession, allowDeferEmpty)
		},
		Finalize: func(attempts int) {
			streamRuntime.finalize("stop", false)
			recordChatStreamHistory(streamRuntime, historySession)
			config.Logger.Info("[openai_empty_retry] terminal empty output", "surface", "chat.completions", "stream", true, "retry_attempts", attempts, "success_source", "none")
		},
		ParentMessageID: func() int {
			return streamRuntime.responseMessageID
		},
		OnRetryPrompt: func(prompt string) {
			streamRuntime.finalPrompt = prompt
		},
		OnRetryFailure: func(status int, message, code string) {
			failChatStreamRetry(streamRuntime, historySession, status, message, code)
		},
		OnAccountSwitch: func(sessionID string) {
			if sessionIDRef != nil {
				*sessionIDRef = sessionID
			}
		},
		OnTerminal: func(attempts int) {
			logChatStreamTerminal(streamRuntime, attempts)
		},
	})
}

func (h *Handler) prepareChatStreamRuntime(w http.ResponseWriter, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, toolChoice promptcompat.ToolChoicePolicy, historySession *chatHistorySession) (*chatStreamRuntime, string, bool) {
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
		thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw,
		toolChoice,
		len(toolNames) > 0, h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence(),
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
				historySession.progress(streamRuntime.historyThinking(), streamRuntime.historyText())
			}
			return decision
		},
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				finalReason = "content_filter"
			}
		},
		OnContextDone: func() {
			streamRuntime.markContextCancelled()
			if historySession != nil {
				historySession.stopped(streamRuntime.historyThinking(), streamRuntime.historyText(), string(streamengine.StopReasonContextCancelled))
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
		historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.historyThinking(), streamRuntime.historyText())
		return
	}
	historySession.success(http.StatusOK, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.finalFinishReason, streamRuntime.finalUsage)
}

func failChatStreamRetry(streamRuntime *chatStreamRuntime, historySession *chatHistorySession, status int, message, code string) {
	streamRuntime.sendFailedChunk(status, message, code)
	if historySession != nil {
		historySession.error(status, message, code, streamRuntime.historyThinking(), streamRuntime.historyText())
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
