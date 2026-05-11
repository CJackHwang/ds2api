package settings

import (
	"net/http"
	"os"
	"strings"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func (h *Handler) getSettings(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	recommended := defaultRuntimeRecommended(len(snap.Accounts), h.Store.RuntimeAccountMaxInflight())
	needsSync := config.IsVercel() && snap.VercelSyncHash != "" && snap.VercelSyncHash != h.computeSyncHash()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"admin": map[string]any{
			"has_password_hash":        strings.TrimSpace(snap.Admin.PasswordHash) != "",
			"jwt_expire_hours":         h.Store.AdminJWTExpireHours(),
			"jwt_valid_after_unix":     snap.Admin.JWTValidAfterUnix,
			"default_password_warning": authn.UsingDefaultAdminKey(h.Store),
		},
		"runtime": map[string]any{
			"account_max_inflight":         h.Store.RuntimeAccountMaxInflight(),
			"account_max_queue":            h.Store.RuntimeAccountMaxQueue(recommended),
			"global_max_inflight":          h.Store.RuntimeGlobalMaxInflight(recommended),
			"token_refresh_interval_hours": h.Store.RuntimeTokenRefreshIntervalHours(),
		},
		"responses":   snap.Responses,
		"embeddings":  snap.Embeddings,
		"auto_delete": snap.AutoDelete,
		"current_input_file": map[string]any{
			"enabled":   h.Store.CurrentInputFileEnabled(),
			"min_chars": h.Store.CurrentInputFileMinChars(),
		},
		"thinking_injection": map[string]any{
			"enabled":        h.Store.ThinkingInjectionEnabled(),
			"prompt":         h.Store.ThinkingInjectionPrompt(),
			"default_prompt": promptcompat.DefaultThinkingInjectionPrompt,
		},
		"model_aliases":     snap.ModelAliases,
		"env_backed":        h.Store.IsEnvBacked(),
		"needs_vercel_sync": needsSync,
		"parser_v2": map[string]any{
			"mode":         h.Store.ParserV2Mode(),
			"env_override": os.Getenv("DS2API_PARSER_V2") != "",
		},
	})
}
