package admin

import "net/http"

func (h *Handler) getStats(w http.ResponseWriter, _ *http.Request) {
	success, failed := int64(0), int64(0)
	if h.Stats != nil {
		success, failed = h.Stats.Snapshot()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"success_calls": success,
		"failed_calls":  failed,
	})
}

