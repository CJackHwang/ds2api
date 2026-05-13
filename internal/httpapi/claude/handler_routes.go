package claude

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/textclean"
	"ds2api/internal/util"
)

// writeJSON is a package-internal alias to avoid mass-renaming all call-sites.
var writeJSON = util.WriteJSON

type Handler struct {
	Store       ConfigReader
	Auth        AuthResolver
	DS          DeepSeekCaller
	OpenAI      OpenAIChatRunner
	ChatHistory *chathistory.Store
	Session     *history.SessionStore
}

func stripReferenceMarkersEnabled() bool {
	return textclean.StripReferenceMarkersEnabled()
}

var (
	claudeStreamPingInterval    = time.Duration(dsprotocol.KeepAliveTimeout) * time.Second
	claudeStreamIdleTimeout     = time.Duration(dsprotocol.StreamIdleTimeout) * time.Second
	claudeStreamMaxKeepaliveCnt = dsprotocol.MaxKeepaliveCount
)

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/anthropic/v1/models", h.ListModels)
	r.Post("/anthropic/v1/messages", h.Messages)
	r.Post("/anthropic/v1/messages/count_tokens", h.CountTokens)
	r.Post("/v1/messages", h.Messages)
	r.Post("/messages", h.Messages)
	r.Post("/v1/messages/count_tokens", h.CountTokens)
	r.Post("/messages/count_tokens", h.CountTokens)
}

func (h *Handler) ListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, config.ClaudeModelsResponse())
}
