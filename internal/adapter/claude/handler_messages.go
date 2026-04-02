package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	claudefmt "ds2api/internal/format/claude"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/translatorcliproxy"
	"ds2api/internal/util"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
		r.Header.Set("anthropic-version", "2023-06-01")
	}
	if h.OpenAI != nil {
		if h.proxyViaOpenAI(w, r) {
			return
		}
	}
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeClaudeError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	norm, err := normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stdReq := norm.Standard

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		writeClaudeError(w, http.StatusUnauthorized, "invalid token.")
		return
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeClaudeError(w, http.StatusUnauthorized, "Failed to get PoW")
		return
	}
	requestPayload := stdReq.CompletionPayload(sessionID)
	resp, err := h.DS.CallCompletion(r.Context(), a, requestPayload, pow, 3)
	if err != nil {
		writeClaudeError(w, http.StatusInternalServerError, "Failed to get Claude response.")
		return
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeClaudeError(w, http.StatusInternalServerError, string(body))
		return
	}

	if stdReq.Stream {
		h.handleClaudeStreamRealtime(w, r, resp, stdReq.ResponseModel, norm.NormalizedMessages, stdReq.Thinking, stdReq.Search, stdReq.ToolNames)
		return
	}
	result := sse.CollectStream(resp, stdReq.Thinking, true)
	respBody := claudefmt.BuildMessageResponse(
		fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		stdReq.ResponseModel,
		norm.NormalizedMessages,
		result.Thinking,
		result.Text,
		stdReq.ToolNames,
	)
	if result.OutputTokens > 0 {
		if usage, ok := respBody["usage"].(map[string]any); ok {
			usage["output_tokens"] = result.OutputTokens
		}
	}
	writeJSON(w, http.StatusOK, respBody)
}

func (h *Handler) proxyViaOpenAI(w http.ResponseWriter, r *http.Request) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid body")
		return true
	}
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	model, _ := req["model"].(string)
	stream := util.ToBool(req["stream"])
	if norm, normErr := normalizeClaudeRequest(h.Store, req); normErr == nil {
		model = norm.Standard.ResolvedModel
		req["model"] = model
		if encoded, encodeErr := json.Marshal(req); encodeErr == nil {
			raw = encoded
		}
	}
	translatedReq := translatorcliproxy.ToOpenAI(sdktranslator.FormatClaude, model, raw, stream)

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
		defer res.Body.Close()
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
	defer res.Body.Close()
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(converted)
	return true
}

func (h *Handler) handleClaudeStreamRealtime(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, messages []any, thinkingEnabled, searchEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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
		toolNames,
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
