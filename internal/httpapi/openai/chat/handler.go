package chat

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

const openAIGeneralMaxSize = shared.GeneralMaxSize

var writeJSON = shared.WriteJSON

type Handler struct {
	Store       shared.ConfigReader
	Auth        shared.AuthResolver
	DS          shared.DeepSeekCaller
	ChatHistory *chathistory.Store

	leaseMu      sync.Mutex
	streamLeases map[string]streamLease
}

type streamLease struct {
	Auth      *auth.RequestAuth
	ExpiresAt time.Time
}

func (h *Handler) compatStripReferenceMarkers() bool {
	if h == nil {
		return true
	}
	return shared.CompatStripReferenceMarkers(h.Store)
}

func (h *Handler) applyHistorySplit(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if h == nil {
		return stdReq, nil
	}
	return history.Service{Store: h.Store, DS: h.DS}.Apply(ctx, a, stdReq)
}

func (h *Handler) preprocessInlineFileInputs(ctx context.Context, a *auth.RequestAuth, req map[string]any) error {
	if h == nil {
		return nil
	}
	return (&files.Handler{Store: h.Store, Auth: h.Auth, DS: h.DS, ChatHistory: h.ChatHistory}).PreprocessInlineFileInputs(ctx, a, req)
}

func (h *Handler) toolcallFeatureMatchEnabled() bool {
	if h == nil {
		return shared.ToolcallFeatureMatchEnabled(nil)
	}
	return shared.ToolcallFeatureMatchEnabled(h.Store)
}

func (h *Handler) toolcallEarlyEmitHighConfidence() bool {
	if h == nil {
		return shared.ToolcallEarlyEmitHighConfidence(nil)
	}
	return shared.ToolcallEarlyEmitHighConfidence(h.Store)
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	shared.WriteOpenAIError(w, status, message)
}

func openAIErrorType(status int) string {
	return shared.OpenAIErrorType(status)
}

func writeOpenAIInlineFileError(w http.ResponseWriter, err error) {
	files.WriteInlineFileError(w, err)
}

func mapHistorySplitError(err error) (int, string) {
	return history.MapError(err)
}

func requestTraceID(r *http.Request) string {
	return shared.RequestTraceID(r)
}

func asString(v any) string {
	return shared.AsString(v)
}

func cleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	return shared.CleanVisibleOutput(text, stripReferenceMarkers)
}

func replaceCitationMarkersWithLinks(text string, links map[int]string) string {
	return shared.ReplaceCitationMarkersWithLinks(text, links)
}

func shouldWriteUpstreamEmptyOutputError(text string) bool {
	return shared.ShouldWriteUpstreamEmptyOutputError(text)
}

func shouldRetryUpstreamEmptyOutput(text string, contentFilter bool) bool {
	return shared.ShouldRetryUpstreamEmptyOutput(text, contentFilter)
}

func retryUpstreamEmptyOutputWithTimestamp(opts shared.EmptyOutputRetryOptions) (*http.Response, bool, error) {
	return shared.RetryUpstreamEmptyOutputWithTimestamp(opts)
}

func upstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string) {
	return shared.UpstreamEmptyOutputDetail(contentFilter, text, thinking)
}

func writeUpstreamEmptyOutputError(w http.ResponseWriter, text, thinking string, contentFilter bool) bool {
	return shared.WriteUpstreamEmptyOutputError(w, text, thinking, contentFilter)
}

func formatIncrementalStreamToolCallDeltas(deltas []toolstream.ToolCallDelta, ids map[int]string) []map[string]any {
	return shared.FormatIncrementalStreamToolCallDeltas(deltas, ids)
}

func filterIncrementalToolCallDeltasByAllowed(deltas []toolstream.ToolCallDelta, seenNames map[int]string) []toolstream.ToolCallDelta {
	return shared.FilterIncrementalToolCallDeltasByAllowed(deltas, seenNames)
}

func formatFinalStreamToolCallsWithStableIDs(calls []toolcall.ParsedToolCall, ids map[int]string) []map[string]any {
	return shared.FormatFinalStreamToolCallsWithStableIDs(calls, ids)
}
