package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/config"
)

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	old := h.Store.Snapshot()
	err := h.Store.Update(func(c *config.Config) error {
		if keys, ok := toStringSlice(req["keys"]); ok {
			c.Keys = keys
		}
		if accountsRaw, ok := req["accounts"].([]any); ok {
			existing := map[string]config.Account{}
			for _, a := range old.Accounts {
				a = normalizeAccountForStorage(a)
				key := accountDedupeKey(a)
				if key != "" {
					existing[key] = a
				}
			}
			seen := map[string]struct{}{}
			accounts := make([]config.Account, 0, len(accountsRaw))
			for _, item := range accountsRaw {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				acc := normalizeAccountForStorage(toAccount(m))
				key := accountDedupeKey(acc)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				if prev, ok := existing[key]; ok {
					if strings.TrimSpace(acc.Password) == "" {
						acc.Password = prev.Password
					}
				}
				seen[key] = struct{}{}
				accounts = append(accounts, acc)
			}
			c.Accounts = accounts
		}
		if proxiesRaw, ok := req["proxies"].([]any); ok {
			proxies := make([]config.Proxy, 0, len(proxiesRaw))
			seen := map[string]struct{}{}
			for _, item := range proxiesRaw {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				proxy := toProxy(m)
				if proxy.ID == "" {
					continue
				}
				if _, ok := seen[proxy.ID]; ok {
					continue
				}
				seen[proxy.ID] = struct{}{}
				proxies = append(proxies, proxy)
			}
			c.Proxies = proxies
		}
		if m, ok := req["claude_mapping"].(map[string]any); ok {
			newMap := map[string]string{}
			for k, v := range m {
				newMap[k] = fmt.Sprintf("%v", v)
			}
			c.ClaudeMapping = newMap
		}
		return config.ValidateConfig(*c)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "配置已更新"})
}

func (h *Handler) addKey(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	key, _ := req["key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "Key 不能为空"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for _, k := range c.Keys {
			if k == key {
				return fmt.Errorf("key 已存在")
			}
		}
		c.Keys = append(c.Keys, key)
		return config.ValidateConfig(*c)
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, k := range c.Keys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("key 不存在")
		}
		c.Keys = append(c.Keys[:idx], c.Keys[idx+1:]...)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) batchImport(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "无效的 JSON 格式"})
		return
	}
	importedKeys, importedAccounts := 0, 0
	err := h.Store.Update(func(c *config.Config) error {
		if keys, ok := req["keys"].([]any); ok {
			existing := map[string]bool{}
			for _, k := range c.Keys {
				existing[k] = true
			}
			for _, k := range keys {
				key := strings.TrimSpace(fmt.Sprintf("%v", k))
				if key == "" || existing[key] {
					continue
				}
				c.Keys = append(c.Keys, key)
				existing[key] = true
				importedKeys++
			}
		}
		if accounts, ok := req["accounts"].([]any); ok {
			existing := map[string]bool{}
			for _, a := range c.Accounts {
				a = normalizeAccountForStorage(a)
				key := accountDedupeKey(a)
				if key != "" {
					existing[key] = true
				}
			}
			for _, item := range accounts {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				acc := normalizeAccountForStorage(toAccount(m))
				key := accountDedupeKey(acc)
				if key == "" || existing[key] {
					continue
				}
				c.Accounts = append(c.Accounts, acc)
				existing[key] = true
				importedAccounts++
			}
		}
		if proxies, ok := req["proxies"].([]any); ok {
			existing := map[string]bool{}
			for _, p := range c.Proxies {
				p = config.NormalizeProxy(p)
				if p.ID != "" {
					existing[p.ID] = true
				}
			}
			for _, item := range proxies {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				proxy := toProxy(m)
				if proxy.ID == "" || existing[proxy.ID] {
					continue
				}
				c.Proxies = append(c.Proxies, proxy)
				existing[proxy.ID] = true
			}
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "imported_keys": importedKeys, "imported_accounts": importedAccounts})
}
