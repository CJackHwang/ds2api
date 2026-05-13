package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"ds2api/internal/translatorcliproxy"
	"ds2api/internal/util"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func (h *Handler) handleGenerateContent(w http.ResponseWriter, r *http.Request, stream bool) {
	if isGeminiVercelProxyRequest(r) && h.proxyViaOpenAI(w, r, stream) {
		return
	}
	if h.Auth == nil || h.DS == nil {
		if h.OpenAI != nil && h.proxyViaOpenAI(w, r, stream) {
			return
		}
		writeGeminiError(w, http.StatusInternalServerError, "Gemini runtime backend unavailable.")
		return
	}
	if h.handleGeminiDirect(w, r, stream) {
		return
	}
	writeGeminiError(w, http.StatusBadGateway, "Failed to handle Gemini request.")
}

func isGeminiVercelProxyRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1" ||
		strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"
}

func (h *Handler) handleGeminiDirect(w http.ResponseWriter, r *http.Request, stream bool) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.Is(err, requestbody.ErrInvalidUTF8Body) {
			writeGeminiError(w, http.StatusBadRequest, "invalid json")
		} else {
			writeGeminiError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	routeModel := strings.TrimSpace(chi.URLParam(r, "model"))
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		writeGeminiError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	stdReq, err := normalizeGeminiRequest(h.Store, routeModel, req, stream)
	if err != nil {
		writeGeminiError(w, http.StatusBadRequest, err.Error())
		return true
	}
	a, err := h.Auth.Determine(r)
	if err != nil {
		writeGeminiError(w, http.StatusUnauthorized, err.Error())
		return true
	}
	defer h.Auth.Release(a)
	stdReq, err = h.applyCurrentInputFile(r.Context(), a, stdReq)
	if err != nil {
		status, message := mapCurrentInputFileError(err)
		writeGeminiError(w, status, message)
		return true
	}
	historySession := responsehistory.Start(responsehistory.StartParams{
		Store:    h.ChatHistory,
		Request:  r,
		Auth:     a,
		Surface:  "gemini.generate_content",
		Standard: stdReq,
	})
	if stream {
		h.handleGeminiDirectStream(w, r, a, stdReq, historySession)
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
		writeGeminiError(w, outErr.Status, outErr.Message)
		return true
	}
	if historySession != nil {
		historySession.SuccessTurn(http.StatusOK, result.Turn, responsehistory.GenericUsage(result.Turn))
	}
	writeJSON(w, http.StatusOK, buildGeminiGenerateContentResponseFromTurn(result.Turn))
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

func (h *Handler) handleGeminiDirectStream(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, historySession *responsehistory.Session) {
	start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		CurrentInputFile: h.Store,
	})
	if outErr != nil {
		if historySession != nil {
			historySession.Error(outErr.Status, outErr.Message, outErr.Code, "", "")
		}
		writeGeminiError(w, outErr.Status, outErr.Message)
		return
	}
	streamReq := start.Request
	h.handleStreamGenerateContentWithRetry(w, r, a, start.Response, start.Payload, start.Pow, streamReq, streamReq.ResponseModel, streamReq.PromptTokenText, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, historySession)
}

func (h *Handler) proxyViaOpenAI(w http.ResponseWriter, r *http.Request, stream bool) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.Is(err, requestbody.ErrInvalidUTF8Body) {
			writeGeminiError(w, http.StatusBadRequest, "invalid json")
		} else {
			writeGeminiError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	routeModel := strings.TrimSpace(chi.URLParam(r, "model"))
	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		writeGeminiError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	translatedReq := translatorcliproxy.ToOpenAI(sdktranslator.FormatGemini, routeModel, raw, stream)
	if !strings.Contains(string(translatedReq), `"stream"`) {
		var reqMap map[string]any
		if json.Unmarshal(translatedReq, &reqMap) == nil {
			reqMap["stream"] = stream
			if b, e := json.Marshal(reqMap); e == nil {
				translatedReq = b
			}
		}
	}
	translatedReq = applyGeminiThinkingPolicyToOpenAIRequest(translatedReq, req)

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
		streamWriter := translatorcliproxy.NewOpenAIStreamTranslatorWriter(w, sdktranslator.FormatGemini, routeModel, raw, translatedReq)
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
		writeGeminiErrorFromOpenAI(w, res.StatusCode, body)
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
	converted := translatorcliproxy.FromOpenAINonStream(sdktranslator.FormatGemini, routeModel, raw, translatedReq, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(converted)
	return true
}

func applyGeminiThinkingPolicyToOpenAIRequest(translated []byte, original map[string]any) []byte {
	req := map[string]any{}
	if err := json.Unmarshal(translated, &req); err != nil {
		return translated
	}
	enabled, ok := resolveGeminiThinkingOverride(original)
	if !ok {
		return translated
	}
	typ := "disabled"
	if enabled {
		typ = "enabled"
	}
	req["thinking"] = map[string]any{"type": typ}
	out, err := json.Marshal(req)
	if err != nil {
		return translated
	}
	return out
}

func resolveGeminiThinkingOverride(req map[string]any) (bool, bool) {
	generationConfig, ok := req["generationConfig"].(map[string]any)
	if !ok {
		generationConfig, ok = req["generation_config"].(map[string]any)
	}
	if !ok {
		return false, false
	}
	thinkingConfig, ok := generationConfig["thinkingConfig"].(map[string]any)
	if !ok {
		thinkingConfig, ok = generationConfig["thinking_config"].(map[string]any)
	}
	if !ok {
		return false, false
	}
	budget, ok := numericAny(thinkingConfig["thinkingBudget"])
	if !ok {
		budget, ok = numericAny(thinkingConfig["thinking_budget"])
	}
	if !ok {
		return false, false
	}
	return budget > 0, true
}

func numericAny(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func writeGeminiErrorFromOpenAI(w http.ResponseWriter, status int, raw []byte) {
	message := strings.TrimSpace(string(raw))
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err == nil {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
				message = strings.TrimSpace(msg)
			}
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	writeGeminiError(w, status, message)
}

//nolint:unused // retained for native Gemini non-stream handling path.
func (h *Handler) handleNonStreamGenerateContent(w http.ResponseWriter, resp *http.Response, model, finalPrompt string, thinkingEnabled bool, toolNames []string) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	result := sse.CollectStream(resp, thinkingEnabled, true)
	writeJSON(w, http.StatusOK, buildGeminiGenerateContentResponse(
		model,
		finalPrompt,
		cleanVisibleOutput(result.Thinking, false),
		cleanVisibleOutput(result.Text, false),
		toolNames,
	))
}

//nolint:unused // retained for native Gemini non-stream handling path.
func buildGeminiGenerateContentResponse(model, finalPrompt, finalThinking, finalText string, toolNames []string) map[string]any {
	parts := buildGeminiPartsFromFinal(finalText, finalThinking, toolNames)
	usage := buildGeminiUsage(model, finalPrompt, finalThinking, finalText)
	return map[string]any{
		"candidates": []map[string]any{
			{
				"index": 0,
				"content": map[string]any{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": "STOP",
			},
		},
		"modelVersion":  model,
		"usageMetadata": usage,
	}
}

func buildGeminiGenerateContentResponseFromTurn(turn assistantturn.Turn) map[string]any {
	parts := buildGeminiPartsFromTurn(turn)
	return map[string]any{
		"candidates": []map[string]any{
			{
				"index": 0,
				"content": map[string]any{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": "STOP",
			},
		},
		"modelVersion": turn.Model,
		"usageMetadata": map[string]any{
			"promptTokenCount":     turn.Usage.InputTokens,
			"candidatesTokenCount": turn.Usage.OutputTokens,
			"totalTokenCount":      turn.Usage.TotalTokens,
		},
	}
}

func buildGeminiPartsFromTurn(turn assistantturn.Turn) []map[string]any {
	thinkingPart := func() []map[string]any {
		if turn.Thinking == "" {
			return nil
		}
		return []map[string]any{{"text": turn.Thinking, "thought": true}}
	}
	if len(turn.ToolCalls) > 0 {
		parts := thinkingPart()
		if parts == nil {
			parts = make([]map[string]any, 0, len(turn.ToolCalls))
		}
		for _, tc := range turn.ToolCalls {
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": tc.Name,
					"args": tc.Input,
				},
			})
		}
		return parts
	}
	parts := thinkingPart()
	if turn.Text != "" {
		parts = append(parts, map[string]any{"text": turn.Text})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": ""})
	}
	return parts
}

//nolint:unused // retained for native Gemini non-stream handling path.
func buildGeminiUsage(model, finalPrompt, finalThinking, finalText string) map[string]any {
	promptTokens := util.CountPromptTokens(finalPrompt, model)
	reasoningTokens := util.CountOutputTokens(finalThinking, model)
	completionTokens := util.CountOutputTokens(finalText, model)
	return map[string]any{
		"promptTokenCount":     promptTokens,
		"candidatesTokenCount": reasoningTokens + completionTokens,
		"totalTokenCount":      promptTokens + reasoningTokens + completionTokens,
	}
}

//nolint:unused // retained for native Gemini non-stream handling path.
func buildGeminiPartsFromFinal(finalText, finalThinking string, toolNames []string) []map[string]any {
	detected := toolcall.ParseToolCalls(finalText, toolNames)
	if len(detected) == 0 && finalThinking != "" {
		detected = toolcall.ParseToolCalls(finalThinking, toolNames)
	}
	thinkingPart := func() []map[string]any {
		if finalThinking == "" {
			return nil
		}
		return []map[string]any{{"text": finalThinking, "thought": true}}
	}
	if len(detected) > 0 {
		parts := thinkingPart()
		if parts == nil {
			parts = make([]map[string]any, 0, len(detected))
		}
		for _, tc := range detected {
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": tc.Name,
					"args": tc.Input,
				},
			})
		}
		return parts
	}

	parts := thinkingPart()
	if finalText != "" {
		parts = append(parts, map[string]any{"text": finalText})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": ""})
	}
	return parts
}
