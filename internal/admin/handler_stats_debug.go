package admin

import "net/http"

func (h *Handler) getStatsDebug(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{"success": true}
	if h.Stats == nil {
		resp["stats"] = map[string]any{
			"redis_enabled": false,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["stats"] = h.Stats.DebugSnapshot()
	writeJSON(w, http.StatusOK, resp)
}

