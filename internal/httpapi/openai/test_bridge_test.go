package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/httpapi/openai/chat"
	"ds2api/internal/httpapi/openai/embeddings"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/openai/responses"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

type openAITestSurface struct {
	Store       shared.ConfigReader
	Auth        shared.AuthResolver
	DS          shared.DeepSeekCaller
	ChatHistory *chathistory.Store

	chat       *chat.Handler
	responses  *responses.Handler
	files      *files.Handler
	embeddings *embeddings.Handler
	models     *shared.ModelsHandler
}

func (h *openAITestSurface) deps() shared.Deps {
	if h == nil {
		return shared.Deps{}
	}
	return shared.Deps{Store: h.Store, Auth: h.Auth, DS: h.DS, ChatHistory: h.ChatHistory}
}

func (h *openAITestSurface) chatHandler() *chat.Handler {
	if h.chat == nil {
		deps := h.deps()
		h.chat = &chat.Handler{Store: deps.Store, Auth: deps.Auth, DS: deps.DS, ChatHistory: deps.ChatHistory}
	}
	return h.chat
}

func (h *openAITestSurface) responsesHandler() *responses.Handler {
	if h.responses == nil {
		deps := h.deps()
		h.responses = &responses.Handler{Store: deps.Store, Auth: deps.Auth, DS: deps.DS, ChatHistory: deps.ChatHistory}
	}
	return h.responses
}

func (h *openAITestSurface) filesHandler() *files.Handler {
	if h.files == nil {
		deps := h.deps()
		h.files = &files.Handler{Store: deps.Store, Auth: deps.Auth, DS: deps.DS, ChatHistory: deps.ChatHistory}
	}
	return h.files
}

func (h *openAITestSurface) embeddingsHandler() *embeddings.Handler {
	if h.embeddings == nil {
		deps := h.deps()
		h.embeddings = &embeddings.Handler{Store: deps.Store, Auth: deps.Auth, DS: deps.DS, ChatHistory: deps.ChatHistory}
	}
	return h.embeddings
}

func (h *openAITestSurface) modelsHandler() *shared.ModelsHandler {
	if h.models == nil {
		h.models = &shared.ModelsHandler{Store: h.Store}
	}
	return h.models
}

func (h *openAITestSurface) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	h.chatHandler().ChatCompletions(w, r)
}

func (h *openAITestSurface) applyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	stdReq = shared.ApplyThinkingInjection(h.Store, stdReq)
	svc := history.Service{Store: h.Store, DS: h.DS}
	out, err := svc.ApplyCurrentInputFile(ctx, a, stdReq)
	if err != nil || out.CurrentInputFileApplied {
		return out, err
	}
	return out, nil
}

func (h *openAITestSurface) preprocessInlineFileInputs(ctx context.Context, a *auth.RequestAuth, req map[string]any) error {
	return h.filesHandler().PreprocessInlineFileInputs(ctx, a, req)
}

func registerOpenAITestRoutes(r chi.Router, h *openAITestSurface) {
	r.Get("/v1/models", h.modelsHandler().ListModels)
	r.Get("/v1/models/{model_id}", h.modelsHandler().GetModel)
	r.Post("/v1/chat/completions", h.chatHandler().ChatCompletions)
	r.Post("/v1/responses", h.responsesHandler().Responses)
	r.Get("/v1/responses/{response_id}", h.responsesHandler().GetResponseByID)
	r.Post("/v1/files", h.filesHandler().UploadFile)
	r.Get("/v1/files/{file_id}", h.filesHandler().RetrieveFile)
	r.Post("/v1/embeddings", h.embeddingsHandler().Embeddings)
}

func buildOpenAICurrentInputContextTranscript(messages []any, historyFilename string) string {
	return promptcompat.BuildOpenAICurrentInputContextTranscript(messages, historyFilename)
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	shared.WriteOpenAIError(w, status, message)
}

func replaceCitationMarkersWithLinks(text string, links map[int]string) string {
	return shared.ReplaceCitationMarkersWithLinks(text, links)
}

func sanitizeLeakedOutput(text string) string {
	return shared.CleanVisibleOutput(text, false)
}

func requestTraceID(r *http.Request) string {
	return shared.RequestTraceID(r)
}

func asString(v any) string {
	return shared.AsString(v)
}

func parseSSEDataFrames(t *testing.T, body string) ([]map[string]any, bool) {
	t.Helper()
	lines := strings.Split(body, "\n")
	frames := make([]map[string]any, 0, len(lines))
	done := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			done = true
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			t.Fatalf("decode sse frame failed: %v, payload=%s", err, payload)
		}
		frames = append(frames, frame)
	}
	return frames, done
}
