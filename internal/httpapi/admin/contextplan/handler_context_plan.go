package contextplan

import (
	"net/http"

	"ds2api/internal/contextengine"
)

func (h *Handler) listContextPlans(w http.ResponseWriter, _ *http.Request) {
	buf := contextengine.GlobalPlanBuffer()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":    buf.Len(),
		"capacity": buf.Cap(),
		"items":    buf.Snapshot(),
	})
}

func (h *Handler) clearContextPlans(w http.ResponseWriter, _ *http.Request) {
	contextengine.GlobalPlanBuffer().Clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"detail":  "context plan buffer cleared",
	})
}
