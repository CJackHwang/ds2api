package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	claudefmt "ds2api/internal/format/claude"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/translatorcliproxy"
	"ds2api/internal/util"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
		r.Header.Set("anthropic-version", "2023-06-01")
	}
	if isClaudeVercelProxyRequest(r) && h.proxyViaOpenAI(w, r, h.Store) {
		return
	}
	if h.Auth == nil || h.DS == nil {
		if h.OpenAI != nil && h.proxyViaOpenAI(w, r, h.Store) {
			return
		}
		writeClaudeError(w, http.StatusInternalServerError, "Claude runtime backend unavailable.")
		return
	}
	if h.handleClaudeDirect(w, r) {
		return
	}
	writeClaudeError(w, http.StatusBadGateway, "Failed to handle Claude request.")
}

func isClaudeVercelProxyRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1" ||
		strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"
}

func (h *Handler) handleClaudeDirect(w http.ResponseWriter, r *http.Request) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.Is(err, requestbody.ErrInvalidUTF8Body) {
			writeClaudeError(w, http.StatusBadRequest, "invalid json")
		} else {
			writeClaudeError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	norm, err := normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return true
	}
	exposeThinking := norm.Standard.Thinking
	a, err := h.Auth.Determine(r)
	if err != nil {
		writeClaudeError(w, http.StatusUnauthorized, err.Error())
		return true
	}
	defer h.Auth.Release(a)
	stdReq, err := h.applyCurrentInputFile(r.Context(), a, norm.Standard)
	if err != nil {
		status, message := mapCurrentInputFileError(err)
		writeClaudeError(w, status, message)
		return true
	}
	historySession := responsehistory.Start(responsehistory.StartParams{
		Store:    h.ChatHistory,
		Request:  r,
		Auth:     a,
		Surface:  "claude.messages",
		Standard: stdReq,
	})
	if stdReq.Stream {
		h.handleClaudeDirectStream(w, r, a, stdReq, historySession)
		return true
	}
	result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		RetryEnabled:     true,
		CurrentInputFile: h.Store,
	})
	if outErr != nil {
		if historySession != nil {
			historySession.ErrorTurn(outErr.Status, outErr.Message, outErr.Code, result.Turn)
		}
		writeClaudeError(w, outErr.Status, outErr.Message)
		return true
	}
	if historySession != nil {
		historySession.SuccessTurn(http.StatusOK, result.Turn, responsehistory.GenericUsage(result.Turn))
	}
	writeJSON(w, http.StatusOK, claudefmt.BuildMessageResponseFromTurn(
		fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		stdReq.ResponseModel,
		result.Turn,
		exposeThinking,
	))
	return true
}

func (h *Handler) applyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if h == nil {
		return stdReq, nil
	}
	return (history.Service{Store: h.Store, DS: h.DS}).ApplyCurrentInputFile(ctx, a, stdReq)
}

func mapCurrentInputFileError(err error) (int, string) {
	return history.MapError(err)
}

func (h *Handler) handleClaudeDirectStream(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, historySession *responsehistory.Session) {
	start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		CurrentInputFile: h.Store,
	})
	if outErr != nil {
		if historySession != nil {
			historySession.Error(outErr.Status, outErr.Message, outErr.Code, "", "")
		}
		writeClaudeError(w, outErr.Status, outErr.Message)
		return
	}
	streamReq := start.Request
	h.handleClaudeStreamRealtimeWithRetry(w, r, a, start.Response, start.Payload, start.Pow, streamReq, streamReq.ResponseModel, streamReq.Messages, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.PromptTokenText, historySession)
}

func (h *Handler) proxyViaOpenAI(w http.ResponseWriter, r *http.Request, store ConfigReader) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.Is(err, requestbody.ErrInvalidUTF8Body) {
			writeClaudeError(w, http.StatusBadRequest, "invalid json")
		} else {
			writeClaudeError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	model, _ := req["model"].(string)
	stream := util.ToBool(req["stream"])

	// Use the shared global model resolver so Claude/OpenAI/Gemini stay consistent.
	translateModel := model
	if store != nil {
		if norm, normErr := normalizeClaudeRequest(store, cloneMap(req)); normErr == nil && strings.TrimSpace(norm.Standard.ResolvedModel) != "" {
			translateModel = strings.TrimSpace(norm.Standard.ResolvedModel)
		}
	}
	translatedReq := translatorcliproxy.ToOpenAI(sdktranslator.FormatClaude, translateModel, raw, stream)
	translatedReq, exposeThinking := applyClaudeThinkingPolicyToOpenAIRequest(translatedReq, req)

	isVercelPrepare := strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1"
	isVercelRelease := strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"

	if isVercelRelease {
		proxyReq := r.Clone(r.Context())
		proxyReq.URL.Path = "/v1/chat/completions"
		proxyReq.Body = io.NopCloser(bytes.NewReader(raw))
		proxyReq.ContentLength = int64(len(raw))
		rec := httptest.NewRecorder()
		h.OpenAI.ChatCompletions(rec, proxyReq)
		res := rec.Result()
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.URL.Path = "/v1/chat/completions"
	proxyReq.Body = io.NopCloser(bytes.NewReader(translatedReq))
	proxyReq.ContentLength = int64(len(translatedReq))

	if stream && !isVercelPrepare {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		streamWriter := translatorcliproxy.NewOpenAIStreamTranslatorWriter(w, sdktranslator.FormatClaude, model, raw, translatedReq)
		h.OpenAI.ChatCompletions(streamWriter, proxyReq)
		return true
	}

	rec := httptest.NewRecorder()
	h.OpenAI.ChatCompletions(rec, proxyReq)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}
	if isVercelPrepare {
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}
	converted := translatorcliproxy.FromOpenAINonStream(sdktranslator.FormatClaude, model, raw, translatedReq, body)
	if !exposeThinking {
		converted = stripClaudeThinkingBlocks(converted)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(converted)
	return true
}

func applyClaudeThinkingPolicyToOpenAIRequest(translated []byte, original map[string]any) ([]byte, bool) {
	req := map[string]any{}
	if err := json.Unmarshal(translated, &req); err != nil {
		return translated, false
	}
	enabled, ok := util.ResolveThinkingOverride(original)
	if !ok {
		if _, translatedHasOverride := util.ResolveThinkingOverride(req); translatedHasOverride {
			return translated, false
		}
		enabled = true
	}
	typ := "disabled"
	if enabled {
		typ = "enabled"
	}
	req["thinking"] = map[string]any{"type": typ}
	out, err := json.Marshal(req)
	if err != nil {
		return translated, enabled
	}
	return out, enabled
}

func stripClaudeThinkingBlocks(raw []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	content, _ := payload["content"].([]any)
	if len(content) == 0 {
		return raw
	}
	filtered := make([]any, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		blockType, _ := block["type"].(string)
		if strings.TrimSpace(blockType) == "thinking" {
			continue
		}
		filtered = append(filtered, item)
	}
	payload["content"] = filtered
	out, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return out
}

func (h *Handler) handleClaudeStreamRealtime(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, messages []any, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySessions ...*responsehistory.Session) {
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
		writeClaudeError(w, http.StatusInternalServerError, string(body))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[claude_stream] response writer does not support flush; streaming may be buffered")
	}

	streamRuntime := newClaudeStreamRuntime(
		w,
		rc,
		canFlush,
		model,
		messages,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkersEnabled(),
		toolNames,
		toolsRaw,
		buildClaudePromptTokenText(messages, thinkingEnabled),
		historySession,
	)
	streamRuntime.sendMessageStart()

	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   claudeStreamPingInterval,
		IdleTimeout:         claudeStreamIdleTimeout,
		MaxKeepAliveNoInput: claudeStreamMaxKeepaliveCnt,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: func() {
			streamRuntime.sendPing()
		},
		OnParsed:   streamRuntime.onParsed,
		OnFinalize: streamRuntime.onFinalize,
	})
}

func (h *Handler) handleClaudeStreamRealtimeWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow string, stdReq promptcompat.StandardRequest, model string, messages []any, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, promptTokenText string, historySession *responsehistory.Session) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeClaudeError(w, http.StatusInternalServerError, string(body))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[claude_stream] response writer does not support flush; streaming may be buffered")
	}

	streamRuntime := newClaudeStreamRuntime(
		w,
		rc,
		canFlush,
		model,
		messages,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkersEnabled(),
		toolNames,
		toolsRaw,
		promptTokenText,
		historySession,
	)
	streamRuntime.sendMessageStart()

	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:          "claude.messages",
		Stream:           true,
		RetryEnabled:     true,
		MaxAttempts:      3,
		UsagePrompt:      promptTokenText,
		Request:          stdReq,
		CurrentInputFile: h.Store,
	}, completionruntime.StreamRetryHooks{
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) (bool, bool) {
			return h.consumeClaudeStreamAttempt(r, currentResp, streamRuntime, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(_ int) {
			streamRuntime.finalize("end_turn", false)
		},
		ParentMessageID: func() int {
			return streamRuntime.responseMessageID
		},
		OnRetryPrompt: func(prompt string) {
			streamRuntime.promptTokenText = prompt
		},
		OnRetryFailure: func(status int, message, code string) {
			streamRuntime.sendErrorWithCode(status, strings.TrimSpace(message), code)
		},
	})
}

func (h *Handler) consumeClaudeStreamAttempt(r *http.Request, resp *http.Response, streamRuntime *claudeStreamRuntime, thinkingEnabled bool, allowDeferEmpty bool) (bool, bool) {
	defer func() { _ = resp.Body.Close() }()
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	finalReason := streamengine.StopReason("")
	var scannerErr error
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   claudeStreamPingInterval,
		IdleTimeout:         claudeStreamIdleTimeout,
		MaxKeepAliveNoInput: claudeStreamMaxKeepaliveCnt,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: func() {
			streamRuntime.sendPing()
		},
		OnParsed: streamRuntime.onParsed,
		OnFinalize: func(reason streamengine.StopReason, err error) {
			finalReason = reason
			scannerErr = err
		},
	})
	if string(finalReason) == "upstream_error" {
		if streamRuntime.history != nil {
			streamRuntime.history.Error(500, streamRuntime.upstreamErr, "upstream_error", responsehistory.ThinkingForArchive(streamRuntime.rawThinking.String(), streamRuntime.toolDetectionThinking.String(), streamRuntime.thinking.String()), responsehistory.TextForArchive(streamRuntime.rawText.String(), streamRuntime.text.String()))
		}
		streamRuntime.sendError(streamRuntime.upstreamErr)
		return true, false
	}
	if scannerErr != nil {
		if streamRuntime.history != nil {
			streamRuntime.history.Error(500, scannerErr.Error(), "error", responsehistory.ThinkingForArchive(streamRuntime.rawThinking.String(), streamRuntime.toolDetectionThinking.String(), streamRuntime.thinking.String()), responsehistory.TextForArchive(streamRuntime.rawText.String(), streamRuntime.text.String()))
		}
		streamRuntime.sendError(scannerErr.Error())
		return true, false
	}
	terminalWritten := streamRuntime.finalize("end_turn", allowDeferEmpty)
	if terminalWritten {
		return true, false
	}
	return false, true
}
