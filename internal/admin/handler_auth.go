package admin

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	authn "ds2api/internal/auth"
)

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := authn.VerifyAdminRequestWithStore(r, h.Store); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	adminKey, _ := req["admin_key"].(string)
	expireHours := intFrom(req["expire_hours"])
	if !authn.VerifyAdminCredential(adminKey, h.Store) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "Invalid admin key"})
		return
	}
	token, err := authn.CreateJWTWithStore(expireHours, h.Store)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	if expireHours <= 0 {
		expireHours = h.Store.AdminJWTExpireHours()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": token, "expires_in": expireHours * 3600})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "No credentials provided"})
		return
	}
	token := strings.TrimSpace(header[7:])
	payload, err := authn.VerifyJWTWithStore(token, h.Store)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
		return
	}
	exp, _ := payload["exp"].(float64)
	remaining := int64(exp) - time.Now().Unix()
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "expires_at": int64(exp), "remaining_seconds": remaining})
}

func (h *Handler) getVercelConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"has_token":          strings.TrimSpace(os.Getenv("VERCEL_TOKEN")) != "",
		"project_id":         strings.TrimSpace(os.Getenv("VERCEL_PROJECT_ID")),
		"team_id":            nilIfEmpty(strings.TrimSpace(os.Getenv("VERCEL_TEAM_ID"))),
		"has_redis_url":      strings.TrimSpace(os.Getenv("DS2API_REDIS_URL")) != "" || strings.TrimSpace(os.Getenv("KV_URL")) != "" || strings.TrimSpace(os.Getenv("REDIS_URL")) != "" || strings.TrimSpace(os.Getenv("UPSTASH_REDIS_URL")) != "",
		"redis_key":          nilIfEmpty(strings.TrimSpace(os.Getenv("DS2API_REDIS_KEY"))),
		"storage_backend":    h.Store.StorageBackend(),
		"persistent_storage": h.Store.IsPersistentStorage(),
		"redis_configured":   h.Store.IsRedisConfigured(),
		"redis_runtime_key":  nilIfEmpty(h.Store.RedisConfigKey()),
	})
}
