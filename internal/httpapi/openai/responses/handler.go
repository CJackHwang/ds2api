package responses

import (
	"context"
	"net/http"
	"sync"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/textclean"
	"ds2api/internal/toolstream"
)

const openAIGeneralMaxSize = shared.GeneralMaxSize

var writeJSON = shared.WriteJSON

type Handler struct {
	Store       shared.ConfigReader
	Auth        shared.AuthResolver
	DS          shared.DeepSeekCaller
	ChatHistory *chathistory.Store

	responsesMu sync.Mutex
	responses   *responseStore
}

func stripReferenceMarkersEnabled() bool {
	return textclean.StripReferenceMarkersEnabled()
}

func (h *Handler) applyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if h == nil {
		return stdReq, nil
	}
	stdReq = shared.ApplyThinkingInjection(h.Store, stdReq)
	svc := history.Service{Store: h.Store, DS: h.DS}
	out, err := svc.ApplyCurrentInputFile(ctx, a, stdReq)
	if err != nil || out.CurrentInputFileApplied {
		return out, err
	}
	return out, nil
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

func writeOpenAIErrorWithCode(w http.ResponseWriter, status int, message, code string) {
	shared.WriteOpenAIErrorWithCode(w, status, message, code)
}

func openAIErrorType(status int) string {
	return shared.OpenAIErrorType(status)
}

func writeOpenAIInlineFileError(w http.ResponseWriter, err error) {
	files.WriteInlineFileError(w, err)
}

func mapCurrentInputFileError(err error) (int, string) {
	return history.MapError(err)
}

func requestTraceID(r *http.Request) string {
	return shared.RequestTraceID(r)
}

func cleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	return shared.CleanVisibleOutput(text, stripReferenceMarkers)
}

func emptyOutputRetryEnabled() bool {
	return shared.EmptyOutputRetryEnabled()
}

func emptyOutputRetryMaxAttempts() int {
	return shared.EmptyOutputRetryMaxAttempts()
}

func filterIncrementalToolCallDeltasByAllowed(deltas []toolstream.ToolCallDelta, seenNames map[int]string) []toolstream.ToolCallDelta {
	return shared.FilterIncrementalToolCallDeltasByAllowed(deltas, seenNames)
}
