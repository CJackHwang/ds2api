package admin

import "net/http"

func (h *Handler) queueStatus(w http.ResponseWriter, _ *http.Request) {
	status := h.Pool.Status()
	metrics := h.Store.CallMetrics()
	status["calls"] = map[string]any{
		"total":        metrics.TotalCalls,
		"success":      metrics.SuccessCalls,
		"failed":       metrics.FailedCalls,
		"last_updated": nilIfZero(metrics.LastUpdatedAt),
	}
	writeJSON(w, http.StatusOK, status)
}
