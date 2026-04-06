package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/util"
)

// writeJSON and intFrom are package-internal aliases for the shared util versions.
var writeJSON = util.WriteJSON
var intFrom = util.IntFrom

func reverseAccounts(a []config.Account) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func intFromQuery(r *http.Request, key string, d int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func toStringSlice(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%v", item)))
	}
	return out, true
}

func toAccount(m map[string]any) config.Account {
	email := fieldString(m, "email")
	mobile := config.NormalizeMobileForStorage(fieldString(m, "mobile"))
	return config.Account{
		Email:    email,
		Mobile:   mobile,
		Password: fieldString(m, "password"),
		ProxyID:  fieldString(m, "proxy_id"),
	}
}

func fieldString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func statusOr(v int, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func accountMatchesIdentifier(acc config.Account, identifier string) bool {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return false
	}
	if strings.TrimSpace(acc.Email) == id {
		return true
	}
	if mobileKey := config.CanonicalMobileKey(id); mobileKey != "" && mobileKey == config.CanonicalMobileKey(acc.Mobile) {
		return true
	}
	return acc.Identifier() == id
}

func normalizeAccountForStorage(acc config.Account) config.Account {
	acc.Email = strings.TrimSpace(acc.Email)
	acc.Mobile = config.NormalizeMobileForStorage(acc.Mobile)
	acc.ProxyID = strings.TrimSpace(acc.ProxyID)
	return acc
}

func toProxy(m map[string]any) config.Proxy {
	return config.NormalizeProxy(config.Proxy{
		ID:       fieldString(m, "id"),
		Name:     fieldString(m, "name"),
		Type:     fieldString(m, "type"),
		Host:     fieldString(m, "host"),
		Port:     intFrom(m["port"]),
		Username: fieldString(m, "username"),
		Password: fieldString(m, "password"),
	})
}

func findProxyByID(c config.Config, proxyID string) (config.Proxy, bool) {
	id := strings.TrimSpace(proxyID)
	if id == "" {
		return config.Proxy{}, false
	}
	for _, proxy := range c.Proxies {
		proxy = config.NormalizeProxy(proxy)
		if proxy.ID == id {
			return proxy, true
		}
	}
	return config.Proxy{}, false
}

func accountDedupeKey(acc config.Account) string {
	if email := strings.TrimSpace(acc.Email); email != "" {
		return "email:" + email
	}
	if mobile := config.CanonicalMobileKey(acc.Mobile); mobile != "" {
		return "mobile:" + mobile
	}
	if id := strings.TrimSpace(acc.Identifier()); id != "" {
		return "id:" + id
	}
	return ""
}

func normalizeAndDedupeAccounts(accounts []config.Account) []config.Account {
	if len(accounts) == 0 {
		return nil
	}
	out := make([]config.Account, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, acc := range accounts {
		acc = normalizeAccountForStorage(acc)
		key := accountDedupeKey(acc)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, acc)
	}
	return out
}

func findAccountByIdentifier(store ConfigStore, identifier string) (config.Account, bool) {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return config.Account{}, false
	}
	if acc, ok := store.FindAccount(id); ok {
		return acc, true
	}
	accounts := store.Snapshot().Accounts
	for _, acc := range accounts {
		if accountMatchesIdentifier(acc, id) {
			return acc, true
		}
	}
	return config.Account{}, false
}
