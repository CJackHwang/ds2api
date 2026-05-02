package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
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
	var parentMessageID interface{}
	keepSession := h.Store.KeepSessionEnabled()
	defer func() {
		if !keepSession {
			h.autoDeleteRemoteSession(r.Context(), a, sessionID)
		}
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

	cache := GetSessionCache()

	if !keepSession || cache.IsFirstMessage(a.DeepSeekToken) {
		stdReq, err = h.applyCurrentInputFile(r.Context(), a, stdReq)
		if err != nil {
			status, message := mapCurrentInputFileError(err)
			writeOpenAIError(w, status, message)
			return
		}
	}

	if keepSession && !cache.IsFirstMessage(a.DeepSeekToken) {
		stdReq = h.applyKeepSessionHistory(r.Context(), a, stdReq)
	}
	historySession := startChatHistory(h.ChatHistory, r, a, stdReq)

	if keepSession {
		cache := GetSessionCache()
		cachedSID, cachedParentID, found := cache.GetSession(a.DeepSeekToken)
		if found && cachedSID != "" {
			sessionID = cachedSID
			parentMessageID = cachedParentID
			config.Logger.Info("[keep_session] reusing cached session", "account", a.AccountID, "session_id", sessionID[:min(len(sessionID), 12)]+"...")
		} else {
			sessionID, err = h.DS.CreateSession(r.Context(), a, 3)
			if err != nil {
				handleCreateSessionError(w, a, historySession, err)
				return
			}
			cache.SetSession(a.DeepSeekToken, sessionID, nil)
			config.Logger.Info("[keep_session] created new session", "account", a.AccountID, "session_id", sessionID[:min(len(sessionID), 12)]+"...")
		}
	} else {
		sessionID, err = h.DS.CreateSession(r.Context(), a, 3)
		if err != nil {
			handleCreateSessionError(w, a, historySession, err)
			return
		}
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		if historySession != nil {
			historySession.error(http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).", "error", "", "")
		}
		writeOpenAIError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	payload := stdReq.CompletionPayload(sessionID)
	if keepSession && parentMessageID != nil {
		payload["parent_message_id"] = parentMessageID
	}
	resp, err := h.DS.CallCompletion(r.Context(), a, payload, pow, 3)
	if err != nil {
		if historySession != nil {
			historySession.error(http.StatusInternalServerError, "Failed to get completion.", "error", "", "")
		}
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to get completion.")
		return
	}
	refFileTokens := stdReq.RefFileTokens
	if stdReq.Stream {
		h.handleStreamWithRetry(w, r, a, resp, payload, pow, sessionID, stdReq.ResponseModel, stdReq.PromptTokenText, refFileTokens, stdReq.Thinking, stdReq.Search, stdReq.ToolNames, stdReq.ToolsRaw, historySession, keepSession)
		return
	}
	h.handleNonStreamWithRetry(w, r.Context(), a, resp, payload, pow, sessionID, stdReq.ResponseModel, stdReq.PromptTokenText, refFileTokens, stdReq.Thinking, stdReq.Search, stdReq.ToolNames, stdReq.ToolsRaw, historySession, keepSession)
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

	stripReferenceMarkers := h.compatStripReferenceMarkers()
	finalThinking := cleanVisibleOutput(result.Thinking, stripReferenceMarkers)
	finalText := cleanVisibleOutput(result.Text, stripReferenceMarkers)
	if searchEnabled {
		finalText = replaceCitationMarkersWithLinks(finalText, result.CitationLinks)
	}
	detected := detectAssistantToolCalls(result.Text, finalText, result.Thinking, result.ToolDetectionThinking, toolNames)
	if shouldWriteUpstreamEmptyOutputError(finalText) && len(detected.Calls) == 0 {
		status, message, code := upstreamEmptyOutputDetail(result.ContentFilter, finalText, finalThinking)
		if historySession != nil {
			historySession.error(status, message, code, finalThinking, finalText)
		}
		writeUpstreamEmptyOutputError(w, finalText, finalThinking, result.ContentFilter)
		return
	}
	respBody := openaifmt.BuildChatCompletionWithToolCalls(completionID, model, finalPrompt, finalThinking, finalText, detected.Calls, toolsRaw)
	if refFileTokens > 0 {
		addRefFileTokensToUsage(respBody, refFileTokens)
	}
	finishReason := "stop"
	if choices, ok := respBody["choices"].([]map[string]any); ok && len(choices) > 0 {
		if fr, _ := choices[0]["finish_reason"].(string); strings.TrimSpace(fr) != "" {
			finishReason = fr
		}
	}
	if historySession != nil {
		historySession.success(http.StatusOK, finalThinking, finalText, finishReason, openaifmt.BuildChatUsageForModel(model, finalPrompt, finalThinking, finalText, refFileTokens))
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
	stripReferenceMarkers := h.compatStripReferenceMarkers()
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
		bufferToolContent,
		emitEarlyToolDeltas,
		false,
		"",
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
				historySession.progress(streamRuntime.thinking.String(), streamRuntime.text.String())
			}
			return decision
		},
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				streamRuntime.finalize("content_filter", false)
			} else {
				streamRuntime.finalize("stop", false)
			}
			if streamRuntime.keepSession && streamRuntime.responseMessageID > 0 {
				cache := GetSessionCache()
				cache.UpdateMessageIDs(streamRuntime.token, streamRuntime.responseMessageID, streamRuntime.responseMessageID)
			}
			if historySession == nil {
				return
			}
			if streamRuntime.finalErrorMessage != "" {
				historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.thinking.String(), streamRuntime.text.String())
				return
			}
			historySession.success(http.StatusOK, streamRuntime.finalThinking, streamRuntime.finalText, streamRuntime.finalFinishReason, streamRuntime.finalUsage)
		},
		OnContextDone: func() {
			if historySession != nil {
				historySession.stopped(streamRuntime.thinking.String(), streamRuntime.text.String(), string(streamengine.StopReasonContextCancelled))
			}
		},
	})
}

func handleCreateSessionError(w http.ResponseWriter, a *auth.RequestAuth, historySession *chatHistorySession, err error) {
	if a.UseConfigToken {
		if historySession != nil {
			historySession.error(http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.", "error", "", "")
		}
		writeOpenAIError(w, http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.")
	} else {
		if historySession != nil {
			historySession.error(http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.", "error", "", "")
		}
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
	}
}

func (h *Handler) applyKeepSessionHistory(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) promptcompat.StandardRequest {
	cache := GetSessionCache()
	token := a.DeepSeekToken
	oldHistory := cache.GetHistoryText(token)
	newHistory := stdReq.HistoryText

	if cache.IsFirstMessage(token) {
		cache.SetHistoryText(token, newHistory)
		return stdReq
	}

	newContent := extractNewHistoryContent(oldHistory, newHistory)
	if newContent == "" {
		return stdReq
	}

	cache.SetHistoryText(token, newHistory)

	supplementContent := buildSupplementFromMessages(stdReq.Messages, newContent)
	if supplementContent != "" {
		supplementFilename := h.Store.SupplementFilename()
		if supplementFilename == "" {
			supplementFilename = "supplement.txt"
		}
		modelType := "default"
		if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
			modelType = resolvedType
		}
		config.Logger.Info("[keep_session] uploading supplement file",
			"filename", supplementFilename,
			"content_len", len(supplementContent))
		result, err := h.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
			Filename:    supplementFilename,
			ContentType: "text/plain; charset=utf-8",
			Purpose:     "assistants",
			ModelType:   modelType,
			Data:        []byte(supplementContent),
		}, 3)
		if err != nil {
			config.Logger.Warn("[keep_session] supplement upload failed, falling back to inline", "error", err)
			stdReq.PassThrough["supplement_text"] = supplementContent
		} else {
			fileID := strings.TrimSpace(result.ID)
			if fileID != "" {
				stdReq.RefFileIDs = append(stdReq.RefFileIDs, fileID)
				config.Logger.Info("[keep_session] supplement uploaded successfully", "file_id", fileID)
			}
		}
	}

	stdReq.FinalPrompt = extractLastUserInput(newContent)

	return stdReq
}

func buildSupplementFromMessages(messages []any, newUserContent string) string {
	if len(messages) == 0 {
		return ""
	}

	systemContent := ""
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		if role == "system" {
			systemContent = asString(msg["content"])
			break
		}
	}

	config.Logger.Info("[keep_session] buildSupplementFromMessages",
		"messages_count", len(messages),
		"system_content_len", len(systemContent),
		"has_begin_tag", strings.Contains(systemContent, "<｜begin▁of▁sentence｜>"),
		"has_end_tag", strings.Contains(systemContent, "<｜end▁of▁instructions｜>"))

	result := extractSystemPrompt(systemContent)
	if result != "" {
		config.Logger.Info("[keep_session] supplement extracted",
			"result_len", len(result),
			"preview", result[:min(100, len(result))]+"...")
	}
	return result
}

func extractSystemPrompt(content string) string {
	startIdx := strings.Index(content, "<｜begin▁of▁sentence｜>")
	endIdx := strings.Index(content, "<｜end▁of▁instructions｜>")

	if startIdx < 0 && endIdx < 0 {
		return content
	}

	if startIdx < 0 {
		startIdx = 0
	} else if endIdx < 0 {
		return content[startIdx:]
	}

	endIdx += len("<｜end▁of▁instructions｜>")
	if endIdx > len(content) {
		endIdx = len(content)
	}
	return content[startIdx:endIdx]
}

func extractLastUserInput(text string) string {
	idx := strings.LastIndex(text, "<｜User｜>")
	if idx < 0 {
		return text
	}
	start := idx + len("<｜User｜>")
	end := strings.Index(text[start:], "</attachment>")
	if end < 0 {
		return text[start:]
	}
	return text[start : start+end]
}
