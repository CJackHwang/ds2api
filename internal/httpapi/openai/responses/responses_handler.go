package responses

import (
	"ds2api/internal/toolcall"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) GetResponseByID(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.DetermineCaller(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "response_id"))
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "response_id is required.")
		return
	}
	owner := responseStoreOwner(a)
	if owner == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	st := h.getResponseStore()
	item, ok := st.get(owner, id)
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "Response not found.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
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
	defer h.Auth.Release(a)
	r = r.WithContext(auth.WithAuth(r.Context(), a))
	owner := responseStoreOwner(a)
	if owner == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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
	traceID := requestTraceID(r)
	stdReq, err := promptcompat.NormalizeOpenAIResponsesRequest(h.Store, req, traceID)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	stdReq, err = h.applyHistorySplit(r.Context(), a, stdReq)
	if err != nil {
		status, message := mapHistorySplitError(err)
		writeOpenAIError(w, status, message)
		return
	}

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		if a.UseConfigToken {
			writeOpenAIError(w, http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.")
		} else {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
		}
		return
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	payload := stdReq.CompletionPayload(sessionID)
	resp, err := h.DS.CallCompletion(r.Context(), a, payload, pow, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to get completion.")
		return
	}

	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if stdReq.Stream {
		h.handleResponsesStream(w, r, resp, owner, responseID, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.Search, stdReq.ToolNames, stdReq.ToolChoice, traceID)
		return
	}
	h.handleResponsesNonStream(w, resp, owner, responseID, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.Search, stdReq.ToolNames, stdReq.ToolChoice, traceID)
}

func (h *Handler) handleResponsesNonStream(w http.ResponseWriter, resp *http.Response, owner, responseID, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolChoice promptcompat.ToolChoicePolicy, traceID string) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}
	result := sse.CollectStream(resp, thinkingEnabled, true)
	stripReferenceMarkers := h.compatStripReferenceMarkers()
	sanitizedThinking := cleanVisibleOutput(result.Thinking, stripReferenceMarkers)
	sanitizedText := cleanVisibleOutput(result.Text, stripReferenceMarkers)
	if searchEnabled {
		sanitizedText = replaceCitationMarkersWithLinks(sanitizedText, result.CitationLinks)
	}
	textParsed := toolcall.ParseStandaloneToolCallsDetailed(sanitizedText, toolNames)
	logResponsesToolPolicyRejection(traceID, toolChoice, textParsed, "text")
	detected, sanitizedThinking := shared.DetectToolCallsWithThinkingFallback(sanitizedText, sanitizedThinking, sanitizedThinking, toolNames)
	if writeUpstreamEmptyOutputError(w, sanitizedText, sanitizedThinking, result.ContentFilter) && len(detected.Calls) == 0 {
		return
	}
	callCount := len(detected.Calls)
	if toolChoice.IsRequired() && callCount == 0 {
		writeOpenAIErrorWithCode(w, http.StatusUnprocessableEntity, "tool_choice requires at least one valid tool call.", "tool_choice_violation")
		return
	}

	responseObj := openaifmt.BuildResponseObject(responseID, model, finalPrompt, sanitizedThinking, sanitizedText, toolNames)
	h.getResponseStore().put(owner, responseID, responseObj)
	writeJSON(w, http.StatusOK, responseObj)
}

func (h *Handler) handleResponsesStream(w http.ResponseWriter, r *http.Request, resp *http.Response, owner, responseID, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolChoice promptcompat.ToolChoicePolicy, traceID string) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
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
	bufferToolContent := len(toolNames) > 0
	emitEarlyToolDeltas := h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence()
	stripReferenceMarkers := h.compatStripReferenceMarkers()

	streamRuntime := newResponsesStreamRuntime(
		w,
		rc,
		canFlush,
		responseID,
		model,
		finalPrompt,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkers,
		toolNames,
		bufferToolContent,
		emitEarlyToolDeltas,
		toolChoice,
		traceID,
		func(obj map[string]any) {
			h.getResponseStore().put(owner, responseID, obj)
		},
	)
	streamRuntime.sendCreated()

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
		OnFinalize: func(_ streamengine.StopReason, _ error) {
			streamRuntime.finalize()
		},
	})
}

func logResponsesToolPolicyRejection(traceID string, policy promptcompat.ToolChoicePolicy, parsed toolcall.ToolCallParseResult, channel string) {
	rejected := filteredRejectedToolNamesForLog(parsed.RejectedToolNames)
	if !parsed.RejectedByPolicy || len(rejected) == 0 {
		return
	}
	config.Logger.Warn(
		"[responses] rejected tool calls by policy",
		"trace_id", strings.TrimSpace(traceID),
		"channel", channel,
		"tool_choice_mode", policy.Mode,
		"rejected_tool_names", strings.Join(rejected, ","),
	)
}

func filteredRejectedToolNamesForLog(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		switch strings.ToLower(trimmed) {
		case "", "tool_name":
			continue
		default:
			out = append(out, trimmed)
		}
	}
	return out
}
