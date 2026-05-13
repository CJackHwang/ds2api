package ollama

import (
	"ds2api/internal/config"
	"ds2api/internal/util"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
)

var WriteJSON = util.WriteJSON

type ConfigReader interface {
	ModelAliases() map[string]string
}

type Handler struct {
	Store ConfigReader
}

type OllamaModelRequest struct {
	Model string `json:"model"`
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/version", h.GetVersion)
	r.Get("/api/tags", h.ListOllamaModels)
	r.Post("/api/show", h.GetOllamaModel)
}

func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"version":"0.23.1"}`))
}
func (h *Handler) ListOllamaModels(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, config.OllamaModelsResponse())
}
func (h *Handler) GetOllamaModel(w http.ResponseWriter, r *http.Request) {
	var payload OllamaModelRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			slog.Warn("[ollama] failed to close request body", "error", err)
		}
	}()
	modelID := payload.Model
	model, ok := config.OllamaModelByID(h.Store, modelID)
	if !ok {
		http.Error(w, "Model not found.", http.StatusNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, model)
}
