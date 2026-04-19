package openai

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/util"
)

const (
	// openAIUploadMaxSize limits total multipart request body size (100 MiB).
	openAIUploadMaxSize = 100 << 20
	// openAIGeneralMaxSize limits total JSON request body size (100 MiB).
	openAIGeneralMaxSize = 100 << 20
)

// writeJSON is a package-internal alias kept to avoid mass-renaming across
// every call-site in this package.
var writeJSON = util.WriteJSON

type Handler struct {
	Store ConfigReader
	Auth  AuthResolver
	DS    DeepSeekCaller

	leaseMu      sync.Mutex
	streamLeases map[string]streamLease
	responsesMu  sync.Mutex
	responses    *responseStore
}

func (h *Handler) compatStripReferenceMarkers() bool {
	if h == nil || h.Store == nil {
		return true
	}
	return h.Store.CompatStripReferenceMarkers()
}

type streamLease struct {
	Auth      *auth.RequestAuth
	SessionID string
	ExpiresAt time.Time
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/v1/models", h.ListModels)
	r.Get("/v1/models/{model_id}", h.GetModel)
	r.Post("/v1/chat/completions", h.ChatCompletions)
	r.Post("/v1/responses", h.Responses)
	r.Get("/v1/responses/{response_id}", h.GetResponseByID)
	r.Post("/v1/files", h.UploadFile)
	r.Post("/v1/embeddings", h.Embeddings)
}

func (h *Handler) ListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, config.OpenAIModelsResponse())
}

func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request) {
	modelID := strings.TrimSpace(chi.URLParam(r, "model_id"))
	model, ok := config.OpenAIModelByID(h.Store, modelID)
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "Model not found.")
		return
	}
	writeJSON(w, http.StatusOK, model)
}
