package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
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
	ok, upgradedHash := authn.VerifyAdminCredentialWithUpgrade(adminKey, h.Store)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "Invalid admin key"})
		return
	}
	if upgradedHash != "" {
		// Transparent migration from a legacy (sha256) hash to bcrypt.
		//
		// Skip on non-persistent stores (env-backed without writeback, or
		// Vercel serverless) where Update would mutate only in-memory state.
		// Such an upgrade would break JWT verification on the next request that
		// lands on a fresh process/lambda that reloads the original sha256 hash
		// from the environment variable.
		nonPersistent := h.Store.IsEnvBacked() && (config.IsVercel() || !h.Store.IsEnvWritebackEnabled())
		if nonPersistent {
			slog.Warn("admin password hash bcrypt upgrade skipped: store is non-persistent (env-only); set DS2API_JWT_SECRET to suppress this warning")
		} else {
			// Persist the new hash before minting the JWT so the token is
			// signed under the post-migration secret.
			if err := h.Store.Update(func(c *config.Config) error {
				c.Admin.PasswordHash = upgradedHash
				return nil
			}); err != nil {
				slog.Error("admin password hash bcrypt upgrade persist failed", "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": "credential upgrade failed; please try again"})
				return
			}
		}
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
	saved := h.Store.Snapshot().Vercel
	token, tokenSource := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_TOKEN")},
		[2]string{"config", saved.Token},
	)
	projectID, _ := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_PROJECT_ID")},
		[2]string{"config", saved.ProjectID},
	)
	teamID, _ := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_TEAM_ID")},
		[2]string{"config", saved.TeamID},
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"has_token":     token != "",
		"token_preview": maskSecretPreview(token),
		"token_source":  nilIfEmpty(tokenSource),
		"project_id":    projectID,
		"team_id":       nilIfEmpty(teamID),
	})
}

func firstConfiguredValue(values ...[2]string) (string, string) {
	for _, pair := range values {
		value := strings.TrimSpace(pair[1])
		if value != "" {
			return value, strings.TrimSpace(pair[0])
		}
	}
	return "", ""
}
