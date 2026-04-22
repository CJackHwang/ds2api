package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
)

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	page := intFromQuery(r, "page", 1)
	pageSize := intFromQuery(r, "page_size", 10)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 5000 {
		pageSize = 5000
	}
	accounts := h.Store.Snapshot().Accounts
	reverseAccounts(accounts)
	q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	if q != "" {
		filtered := make([]config.Account, 0, len(accounts))
		for _, acc := range accounts {
			id := strings.ToLower(acc.Identifier())
			if strings.Contains(id, q) ||
				strings.Contains(strings.ToLower(acc.Email), q) ||
				strings.Contains(strings.ToLower(acc.Mobile), q) {
				filtered = append(filtered, acc)
			}
		}
		accounts = filtered
	}
	total := len(accounts)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-start)
	for _, acc := range accounts[start:end] {
		testStatus, _ := h.Store.AccountTestStatus(acc.Identifier())
		items = append(items, accountListItem(acc, testStatus))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize, "total_pages": totalPages})
}

func validateNewAccountCandidate(cfg config.Config, acc config.Account) error {
	if acc.ProxyID != "" {
		if _, ok := findProxyByID(cfg, acc.ProxyID); !ok {
			return fmt.Errorf("proxy does not exist")
		}
	}
	mobileKey := config.CanonicalMobileKey(acc.Mobile)
	for _, existing := range cfg.Accounts {
		if acc.Email != "" && existing.Email == acc.Email {
			return fmt.Errorf("account email already exists")
		}
		if mobileKey != "" && config.CanonicalMobileKey(existing.Mobile) == mobileKey {
			return fmt.Errorf("account mobile already exists")
		}
	}
	return nil
}

func (h *Handler) validateAccountAvailability(ctx context.Context, acc config.Account) (string, int, error) {
	if h == nil || h.DS == nil {
		return "", 0, fmt.Errorf("account validation is unavailable")
	}
	start := time.Now()
	token, err := h.DS.Login(ctx, acc)
	if err != nil {
		return "", int(time.Since(start).Milliseconds()), fmt.Errorf("account login failed: %w", err)
	}
	authCtx := &authn.RequestAuth{
		UseConfigToken: false,
		DeepSeekToken:  token,
		AccountID:      acc.Identifier(),
		Account:        acc,
	}
	proxyCtx := authn.WithAuth(ctx, authCtx)
	if _, err := h.DS.CreateSession(proxyCtx, authCtx, 1); err != nil {
		retryToken, retryErr := h.DS.Login(proxyCtx, acc)
		if retryErr == nil {
			token = retryToken
			authCtx.DeepSeekToken = token
			if _, retryCreateErr := h.DS.CreateSession(proxyCtx, authCtx, 1); retryCreateErr == nil {
				return token, int(time.Since(start).Milliseconds()), nil
			}
		}
		return "", int(time.Since(start).Milliseconds()), fmt.Errorf("account session validation failed: %w", err)
	}
	return token, int(time.Since(start).Milliseconds()), nil
}

func (h *Handler) addAccount(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	acc := normalizeAccountForStorage(toAccount(req))
	if acc.Identifier() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "email or mobile is required"})
		return
	}

	if err := validateNewAccountCandidate(h.Store.Snapshot(), acc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}

	token, responseTime, err := h.validateAccountAvailability(r.Context(), acc)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}

	err = h.Store.Update(func(c *config.Config) error {
		if err := validateNewAccountCandidate(*c, acc); err != nil {
			return err
		}
		c.Accounts = append(c.Accounts, acc)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}

	if token != "" {
		if err := h.Store.UpdateAccountToken(acc.Identifier(), token); err != nil {
			config.Logger.Warn("[admin_add_account] failed to cache validated token", "account", acc.Identifier(), "error", err)
		}
	}
	_ = h.Store.UpdateAccountTestStatus(acc.Identifier(), "ok")
	h.Pool.Reset()

	saved, ok := h.Store.FindAccount(acc.Identifier())
	if !ok {
		saved = acc
		saved.Token = token
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"validated":      true,
		"response_time":  responseTime,
		"total_accounts": len(h.Store.Accounts()),
		"account":        accountListItem(saved, "ok"),
	})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")
	if decoded, err := url.PathUnescape(identifier); err == nil {
		identifier = decoded
	}
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, a := range c.Accounts {
			if accountMatchesIdentifier(a, identifier) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("account does not exist")
		}
		c.Accounts = append(c.Accounts[:idx], c.Accounts[idx+1:]...)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":            true,
		"deleted_identifier": identifier,
		"total_accounts":     len(h.Store.Accounts()),
	})
}
