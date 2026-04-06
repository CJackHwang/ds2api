package admin

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/config"
)

func (h *Handler) configImport(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}

	mode := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = strings.TrimSpace(strings.ToLower(fieldString(req, "mode")))
	}
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "mode must be merge or replace"})
		return
	}

	payload := req
	if raw, ok := req["config"].(map[string]any); ok && len(raw) > 0 {
		payload = raw
	}
	rawJSON, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid config payload"})
		return
	}
	var incoming config.Config
	if err := json.Unmarshal(rawJSON, &incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	incoming.ClearAccountTokens()

	importedKeys, importedAccounts := 0, 0
	err = h.Store.Update(func(c *config.Config) error {
		next := c.Clone()
		if mode == "replace" {
			next = incoming.Clone()
			next.Accounts = normalizeAndDedupeAccounts(next.Accounts)
			next.VercelSyncHash = c.VercelSyncHash
			next.VercelSyncTime = c.VercelSyncTime
			importedKeys = len(next.Keys)
			importedAccounts = len(next.Accounts)
		} else {
			existingKeys := map[string]struct{}{}
			for _, k := range next.Keys {
				existingKeys[k] = struct{}{}
			}
			for _, k := range incoming.Keys {
				key := strings.TrimSpace(k)
				if key == "" {
					continue
				}
				if _, ok := existingKeys[key]; ok {
					continue
				}
				existingKeys[key] = struct{}{}
				next.Keys = append(next.Keys, key)
				importedKeys++
			}

			existingAccounts := map[string]struct{}{}
			existingProxies := map[string]struct{}{}
			for _, proxy := range next.Proxies {
				proxy = config.NormalizeProxy(proxy)
				if proxy.ID != "" {
					existingProxies[proxy.ID] = struct{}{}
				}
			}
			for _, proxy := range incoming.Proxies {
				proxy = config.NormalizeProxy(proxy)
				if proxy.ID == "" {
					continue
				}
				if _, ok := existingProxies[proxy.ID]; ok {
					continue
				}
				existingProxies[proxy.ID] = struct{}{}
				next.Proxies = append(next.Proxies, proxy)
			}

			for _, acc := range next.Accounts {
				acc = normalizeAccountForStorage(acc)
				key := accountDedupeKey(acc)
				if key != "" {
					existingAccounts[key] = struct{}{}
				}
			}
			for _, acc := range incoming.Accounts {
				acc = normalizeAccountForStorage(acc)
				key := accountDedupeKey(acc)
				if key == "" {
					continue
				}
				if _, ok := existingAccounts[key]; ok {
					continue
				}
				existingAccounts[key] = struct{}{}
				next.Accounts = append(next.Accounts, acc)
				importedAccounts++
			}

			if len(incoming.ClaudeMapping) > 0 {
				if next.ClaudeMapping == nil {
					next.ClaudeMapping = map[string]string{}
				}
				for k, v := range incoming.ClaudeMapping {
					next.ClaudeMapping[k] = v
				}
			}
			if len(incoming.ClaudeModelMap) > 0 {
				if next.ClaudeModelMap == nil {
					next.ClaudeModelMap = map[string]string{}
				}
				for k, v := range incoming.ClaudeModelMap {
					next.ClaudeModelMap[k] = v
				}
			}

			if len(incoming.ModelAliases) > 0 {
				if next.ModelAliases == nil {
					next.ModelAliases = map[string]string{}
				}
				for k, v := range incoming.ModelAliases {
					next.ModelAliases[k] = v
				}
			}
			if incoming.Responses.StoreTTLSeconds > 0 {
				next.Responses.StoreTTLSeconds = incoming.Responses.StoreTTLSeconds
			}
			if strings.TrimSpace(incoming.Embeddings.Provider) != "" {
				next.Embeddings.Provider = incoming.Embeddings.Provider
			}
			if strings.TrimSpace(incoming.Admin.PasswordHash) != "" {
				next.Admin.PasswordHash = incoming.Admin.PasswordHash
			}
			if incoming.Admin.JWTExpireHours > 0 {
				next.Admin.JWTExpireHours = incoming.Admin.JWTExpireHours
			}
			if incoming.Admin.JWTValidAfterUnix > 0 {
				next.Admin.JWTValidAfterUnix = incoming.Admin.JWTValidAfterUnix
			}
			if incoming.Runtime.AccountMaxInflight > 0 {
				next.Runtime.AccountMaxInflight = incoming.Runtime.AccountMaxInflight
			}
			if incoming.Runtime.AccountMaxQueue > 0 {
				next.Runtime.AccountMaxQueue = incoming.Runtime.AccountMaxQueue
			}
			if incoming.Runtime.GlobalMaxInflight > 0 {
				next.Runtime.GlobalMaxInflight = incoming.Runtime.GlobalMaxInflight
			}
			if incoming.Runtime.TokenRefreshIntervalHours > 0 {
				next.Runtime.TokenRefreshIntervalHours = incoming.Runtime.TokenRefreshIntervalHours
			}
		}

		normalizeSettingsConfig(&next)
		if err := validateSettingsConfig(next); err != nil {
			return newRequestError(err.Error())
		}

		*c = next
		return nil
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}

	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"mode":              mode,
		"imported_keys":     importedKeys,
		"imported_accounts": importedAccounts,
		"message":           "config imported",
	})
}

func (h *Handler) computeSyncHash() string {
	snap := h.Store.Snapshot().Clone()
	snap.ClearAccountTokens()
	snap.VercelSyncHash = ""
	snap.VercelSyncTime = 0
	b, _ := json.Marshal(snap)
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}
