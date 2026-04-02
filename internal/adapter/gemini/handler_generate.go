package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/sse"
	"ds2api/internal/translatorcliproxy"
	"ds2api/internal/util"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func (h *Handler) handleGenerateContent(w http.ResponseWriter, r *http.Request, stream bool) {
	if h.OpenAI != nil {
		if h.proxyViaOpenAI(w, r, stream) {
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
		writeGeminiError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGeminiError(w, http.StatusBadRequest, "invalid json")
		return
	}

	routeModel := strings.TrimSpace(chi.URLParam(r, "model"))
	stdReq, err := normalizeGeminiRequest(h.Store, routeModel, req, stream)
	if err != nil {
		writeGeminiError(w, http.StatusBadRequest, err.Error())
		return
	}

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		if a.UseConfigToken {
			writeGeminiError(w, http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.")
		} else {
			writeGeminiError(w, http.StatusUnauthorized, "Invalid token.")
		}
		return
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeGeminiError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	payload := stdReq.CompletionPayload(sessionID)
	resp, err := h.DS.CallCompletion(r.Context(), a, payload, pow, 3)
	if err != nil {
		writeGeminiError(w, http.StatusInternalServerError, "Failed to get completion.")
		return
	}

	if stream {
		h.handleStreamGenerateContent(w, r, resp, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.Search, stdReq.ToolNames)
		return
	}
	h.handleNonStreamGenerateContent(w, resp, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.ToolNames)
}

func (h *Handler) proxyViaOpenAI(w http.ResponseWriter, r *http.Request, stream bool) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeGeminiError(w, http.StatusBadRequest, "invalid body")
		return true
	}
	routeModel := strings.TrimSpace(chi.URLParam(r, "model"))
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
		streamWriter := translatorcliproxy.NewOpenAIStreamTranslatorWriter(w, sdktranslator.FormatGemini, routeModel, raw, translatedReq)
		h.OpenAI.ChatCompletions(streamWriter, proxyReq)
		return true
	}

	rec := httptest.NewRecorder()
	h.OpenAI.ChatCompletions(rec, proxyReq)
	res := rec.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeGeminiError(w, res.StatusCode, extractProxyErrorMessage(body))
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

func extractProxyErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "Request failed."
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return trimmed
	}
	errObj, _ := parsed["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	if strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	return trimmed
}

func (h *Handler) handleNonStreamGenerateContent(w http.ResponseWriter, resp *http.Response, model, finalPrompt string, thinkingEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	result := sse.CollectStream(resp, thinkingEnabled, true)
	writeJSON(w, http.StatusOK, buildGeminiGenerateContentResponse(model, finalPrompt, result.Thinking, result.Text, toolNames, result.OutputTokens))
}

func buildGeminiGenerateContentResponse(model, finalPrompt, finalThinking, finalText string, toolNames []string, outputTokens int) map[string]any {
	parts := buildGeminiPartsFromFinal(finalText, finalThinking, toolNames)
	usage := buildGeminiUsage(finalPrompt, finalThinking, finalText, outputTokens)
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

func buildGeminiUsage(finalPrompt, finalThinking, finalText string, outputTokens int) map[string]any {
	promptTokens := util.EstimateTokens(finalPrompt)
	reasoningTokens := util.EstimateTokens(finalThinking)
	completionTokens := util.EstimateTokens(finalText)
	if outputTokens > 0 {
		completionTokens = outputTokens
		reasoningTokens = 0
	}
	return map[string]any{
		"promptTokenCount":     promptTokens,
		"candidatesTokenCount": reasoningTokens + completionTokens,
		"totalTokenCount":      promptTokens + reasoningTokens + completionTokens,
	}
}

func buildGeminiPartsFromFinal(finalText, finalThinking string, toolNames []string) []map[string]any {
	detected := util.ParseToolCalls(finalText, toolNames)
	if len(detected) == 0 && strings.TrimSpace(finalThinking) != "" {
		detected = util.ParseToolCalls(finalThinking, toolNames)
	}
	if len(detected) > 0 {
		parts := make([]map[string]any, 0, len(detected))
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

	text := finalText
	if strings.TrimSpace(text) == "" {
		text = finalThinking
	}
	return []map[string]any{{"text": text}}
}
