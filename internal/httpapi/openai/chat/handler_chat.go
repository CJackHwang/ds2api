package chat

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
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if isVercelStreamReleaseRequest(r) {
		h.handleVercelStreamRelease(w, r)
		return
	}
	if isVercelStreamPowRequest(r) {
		h.handleVercelStreamPow(w, r)
		return
	}
	if isVercelStreamSwitchRequest(r) {
		h.handleVercelStreamSwitch(w, r)
		return
	}
	if isVercelStreamPrepareRequest(r) {
		h.handleVercelStreamPrepare(w, r)
		return
	}

	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeOpenAIError(w, status, detail)
		return
	}
	var sessionID string
	defer func() {
		h.autoDeleteRemoteSession(r.Context(), a, sessionID)
		h.Auth.Release(a)
	}()

	r = r.WithContext(auth.WithAuth(r.Context(), a))

	r.Body = http.MaxBytesReader(w, r.Body, openAIGeneralMaxSize)
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.preprocessInlineFileInputs(r.Context(), a, req); err != nil {
		writeOpenAIInlineFileError(w, err)
		return
	}
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, requestTraceID(r))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	stdReq, err = h.applyCurrentInputFile(r.Context(), a, stdReq)
	if err != nil {
		status, message := mapCurrentInputFileError(err)
		writeOpenAIError(w, status, message)
		return
	}
	historySession := startChatHistory(h.ChatHistory, r, a, stdReq)

	if !stdReq.Stream {
		result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
			RetryEnabled:     true,
			CurrentInputFile: h.Store,
		})
		sessionID = result.SessionID
		if outErr != nil {
			if historySession != nil {
				historySession.error(outErr.Status, outErr.Message, outErr.Code, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text))
			}
			writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
			return
		}
		respBody := openaifmt.BuildChatCompletionWithToolCalls(result.SessionID, stdReq.ResponseModel, result.Turn.Prompt, result.Turn.Thinking, result.Turn.Text, result.Turn.ToolCalls, stdReq.ToolsRaw)
		respBody["usage"] = assistantturn.OpenAIChatUsage(result.Turn)
		finishReason := assistantturn.FinalizeTurn(result.Turn, assistantturn.FinalizeOptions{}).FinishReason
		if historySession != nil {
			historySession.success(http.StatusOK, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), finishReason, assistantturn.OpenAIChatUsage(result.Turn))
		}
		writeJSON(w, http.StatusOK, respBody)
		return
	}

	start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		CurrentInputFile: h.Store,
	})
	sessionID = start.SessionID
	if outErr != nil {
		if historySession != nil {
			historySession.error(outErr.Status, outErr.Message, outErr.Code, "", "")
		}
		writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
		return
	}
	streamReq := start.Request
	refFileTokens := streamReq.RefFileTokens
	h.handleStreamWithRetry(w, r, a, start.Response, start.Payload, start.Pow, sessionID, &sessionID, streamReq, streamReq.ResponseModel, streamReq.PromptTokenText, refFileTokens, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.ToolChoice, historySession)
}

func (h *Handler) autoDeleteRemoteSession(ctx context.Context, a *auth.RequestAuth, sessionID string) {
	mode := h.Store.AutoDeleteMode()
	if mode == "none" || a.DeepSeekToken == "" {
		return
	}

	deleteBaseCtx := context.WithoutCancel(ctx)
	deleteCtx, cancel := context.WithTimeout(deleteBaseCtx, 10*time.Second)
	defer cancel()

	switch mode {
	case "single":
		if sessionID == "" {
			config.Logger.Warn("[auto_delete_sessions] skipped single-session delete because session_id is empty", "account", a.AccountID)
			return
		}
		_, err := h.DS.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID)
		if err != nil {
			config.Logger.Warn("[auto_delete_sessions] failed", "account", a.AccountID, "mode", mode, "session_id", sessionID, "error", err)
			return
		}
		config.Logger.Debug("[auto_delete_sessions] success", "account", a.AccountID, "mode", mode, "session_id", sessionID)
	case "all":
		if err := h.DS.DeleteAllSessionsForToken(deleteCtx, a.DeepSeekToken); err != nil {
			config.Logger.Warn("[auto_delete_sessions] failed", "account", a.AccountID, "mode", mode, "error", err)
			return
		}
		config.Logger.Debug("[auto_delete_sessions] success", "account", a.AccountID, "mode", mode)
	default:
		config.Logger.Warn("[auto_delete_sessions] unknown mode", "account", a.AccountID, "mode", mode)
	}
}

func (h *Handler) handleNonStream(w http.ResponseWriter, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	result := sse.CollectStream(resp, thinkingEnabled, true)

	turn := assistantturn.BuildTurnFromCollected(result, assistantturn.BuildOptions{
		Model:         model,
		Prompt:        finalPrompt,
		RefFileTokens: refFileTokens,
		SearchEnabled: searchEnabled,
		ToolNames:     toolNames,
		ToolsRaw:      toolsRaw,
		ToolChoice:    promptcompat.DefaultToolChoicePolicy(),
	})
	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
	if outcome.ShouldFail {
		status, message, code := outcome.Error.Status, outcome.Error.Message, outcome.Error.Code
		if historySession != nil {
			historySession.error(status, message, code, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text))
		}
		writeOpenAIErrorWithCode(w, status, message, code)
		return
	}
	respBody := openaifmt.BuildChatCompletionWithToolCalls(completionID, model, finalPrompt, turn.Thinking, turn.Text, turn.ToolCalls, toolsRaw)
	respBody["usage"] = assistantturn.OpenAIChatUsage(turn)
	if historySession != nil {
		historySession.success(http.StatusOK, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text), outcome.FinishReason, assistantturn.OpenAIChatUsage(turn))
	}
	writeJSON(w, http.StatusOK, respBody)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
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

	created := time.Now().Unix()
	bufferToolContent := len(toolNames) > 0
	emitEarlyToolDeltas := h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence()
	stripReferenceMarkers := stripReferenceMarkersEnabled()
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}

	streamRuntime := newChatStreamRuntime(
		w,
		rc,
		canFlush,
		completionID,
		created,
		model,
		finalPrompt,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkers,
		toolNames,
		toolsRaw,
		promptcompat.DefaultToolChoicePolicy(),
		bufferToolContent,
		emitEarlyToolDeltas,
	)
	streamRuntime.refFileTokens = refFileTokens

	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: func() {
			streamRuntime.sendKeepAlive()
		},
		OnParsed: func(parsed sse.LineResult) streamengine.ParsedDecision {
			decision := streamRuntime.onParsed(parsed)
			if historySession != nil {
				historySession.progress(streamRuntime.historyThinking(), streamRuntime.historyText())
			}
			return decision
		},
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				streamRuntime.finalize("content_filter", false)
			} else {
				streamRuntime.finalize("stop", false)
			}
			if historySession == nil {
				return
			}
			if streamRuntime.finalErrorMessage != "" {
				historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.historyThinking(), streamRuntime.historyText())
				return
			}
			historySession.success(http.StatusOK, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.finalFinishReason, streamRuntime.finalUsage)
		},
		OnContextDone: func() {
			streamRuntime.markContextCancelled()
			if historySession != nil {
				historySession.stopped(streamRuntime.historyThinking(), streamRuntime.historyText(), string(streamengine.StopReasonContextCancelled))
			}
		},
	})
}
