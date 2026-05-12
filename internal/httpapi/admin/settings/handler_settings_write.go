package settings

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
)

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}

	adminCfg, runtimeCfg, responsesCfg, embeddingsCfg, autoDeleteCfg, currentInputCfg, thinkingInjCfg, aliasMap, logCfg, err := parseSettingsUpdateRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	if runtimeCfg != nil {
		if err := validateMergedRuntimeSettings(h.Store.Snapshot().Runtime, runtimeCfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
			return
		}
	}
	currentInputEnabledSet := hasNestedSettingsKey(req, "current_input_file", "enabled")
	currentInputMinCharsSet := hasNestedSettingsKey(req, "current_input_file", "min_chars")
	thinkingInjectionEnabledSet := hasNestedSettingsKey(req, "thinking_injection", "enabled")
	thinkingInjectionPromptSet := hasNestedSettingsKey(req, "thinking_injection", "prompt")
	logLevelSet := hasNestedSettingsKey(req, "log", "level")
	logFileSet := hasNestedSettingsKey(req, "log", "file")
	logFileEnabledSet := hasNestedSettingsKey(req, "log", "file_enabled")
	logMaxSizeMBSet := hasNestedSettingsKey(req, "log", "max_size_mb")
	logMaxBackupsSet := hasNestedSettingsKey(req, "log", "max_backups")
	var refreshedLogCfg config.LogConfig

	if err := h.Store.Update(func(c *config.Config) error {
		if adminCfg != nil {
			if adminCfg.JWTExpireHours > 0 {
				c.Admin.JWTExpireHours = adminCfg.JWTExpireHours
			}
		}
		if runtimeCfg != nil {
			if runtimeCfg.AccountMaxInflight > 0 {
				c.Runtime.AccountMaxInflight = runtimeCfg.AccountMaxInflight
			}
			if runtimeCfg.AccountMaxQueue > 0 {
				c.Runtime.AccountMaxQueue = runtimeCfg.AccountMaxQueue
			}
			if runtimeCfg.GlobalMaxInflight > 0 {
				c.Runtime.GlobalMaxInflight = runtimeCfg.GlobalMaxInflight
			}
			if runtimeCfg.TokenRefreshIntervalHours > 0 {
				c.Runtime.TokenRefreshIntervalHours = runtimeCfg.TokenRefreshIntervalHours
			}
		}
		if responsesCfg != nil && responsesCfg.StoreTTLSeconds > 0 {
			c.Responses.StoreTTLSeconds = responsesCfg.StoreTTLSeconds
		}
		if embeddingsCfg != nil && strings.TrimSpace(embeddingsCfg.Provider) != "" {
			c.Embeddings.Provider = strings.TrimSpace(embeddingsCfg.Provider)
		}
		if autoDeleteCfg != nil {
			c.AutoDelete.Mode = autoDeleteCfg.Mode
			c.AutoDelete.Sessions = autoDeleteCfg.Sessions
		}
		if currentInputCfg != nil {
			if currentInputEnabledSet {
				c.CurrentInputFile.Enabled = currentInputCfg.Enabled
			}
			if currentInputMinCharsSet {
				c.CurrentInputFile.MinChars = currentInputCfg.MinChars
			}
		}
		if thinkingInjCfg != nil {
			if thinkingInjectionEnabledSet {
				c.ThinkingInjection.Enabled = thinkingInjCfg.Enabled
			}
			if thinkingInjectionPromptSet {
				c.ThinkingInjection.Prompt = thinkingInjCfg.Prompt
			}
		}
		if aliasMap != nil {
			c.ModelAliases = aliasMap
		}
		if logCfg != nil {
			if logLevelSet {
				c.Log.Level = logCfg.Level
			}
			if logFileSet {
				c.Log.File = logCfg.File
			}
			if logFileEnabledSet {
				c.Log.FileEnabled = logCfg.FileEnabled
			}
			if logMaxSizeMBSet {
				c.Log.MaxSizeMB = logCfg.MaxSizeMB
			}
			if logMaxBackupsSet {
				c.Log.MaxBackups = logCfg.MaxBackups
			}
			refreshedLogCfg = c.Log
		}
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}

	h.applyRuntimeSettings()
	if logCfg != nil {
		config.RefreshLogger(refreshedLogCfg)
	}
	needsSync := config.IsVercel() || h.Store.IsEnvBacked()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":             true,
		"message":             "settings updated and hot reloaded",
		"env_backed":          h.Store.IsEnvBacked(),
		"needs_vercel_sync":   needsSync,
		"manual_sync_message": "配置已保存。Vercel 部署请在 Vercel Sync 页面手动同步。",
	})
}

func (h *Handler) updateSettingsPassword(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	newPassword := strings.TrimSpace(fieldString(req, "new_password"))
	if newPassword == "" {
		newPassword = strings.TrimSpace(fieldString(req, "password"))
	}
	if len(newPassword) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "new password must be at least 4 characters"})
		return
	}

	now := time.Now().Unix()
	hash := authn.HashAdminPassword(newPassword)
	if err := h.Store.Update(func(c *config.Config) error {
		c.Admin.PasswordHash = hash
		c.Admin.JWTValidAfterUnix = now
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":              true,
		"message":              "password updated",
		"force_relogin":        true,
		"jwt_valid_after_unix": now,
	})
}

func hasNestedSettingsKey(req map[string]any, section, key string) bool {
	raw, ok := req[section].(map[string]any)
	if !ok {
		return false
	}
	_, exists := raw[key]
	return exists
}
