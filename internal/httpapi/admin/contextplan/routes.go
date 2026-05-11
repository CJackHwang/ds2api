package contextplan

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/context-plans", h.listContextPlans)
	r.Delete("/context-plans", h.clearContextPlans)
}
