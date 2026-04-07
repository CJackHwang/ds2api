package admin

import "net/http"

func (h *Handler) getUsageStats(w http.ResponseWriter, _ *http.Request) {
	if h == nil || h.Stats == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary": map[string]any{
				"total_calls":    0,
				"account_count":  0,
				"model_count":    0,
				"surface_count":  0,
				"row_count":      0,
				"last_called_at": 0,
			},
			"rows": []any{},
		})
		return
	}
	writeJSON(w, http.StatusOK, h.Stats.Snapshot())
}
