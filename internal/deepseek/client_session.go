package deepseek

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

// SessionInfo 会话信息
type SessionInfo struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	TitleType string  `json:"title_type"`
	Pinned    bool    `json:"pinned"`
	UpdatedAt float64 `json:"updated_at"`
}

// SessionStats 会话统计结果
type SessionStats struct {
	AccountID      string // 账号标识 (email 或 mobile)
	FirstPageCount int    // 第一页会话数量（当 HasMore 为 true 时，真实总数可能更大）
	PinnedCount    int    // 置顶会话数量
	HasMore        bool   // 是否还有更多页
	Success        bool   // 请求是否成功
	ErrorMessage   string // 错误信息
}

// GetSessionCount 获取单个账号的会话数量
func (c *Client) GetSessionCount(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (*SessionStats, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	clients := c.requestClientsForAuth(ctx, a)

	stats := &SessionStats{
		AccountID: a.AccountID,
	}

	attempts := 0
	refreshed := false

	for attempts < maxAttempts {
		headers := c.authHeaders(a.DeepSeekToken)

		// 构建请求 URL
		reqURL := DeepSeekFetchSessionURL + "?lte_cursor.pinned=false"

		resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
		if err != nil {
			config.Logger.Warn("[get_session_count] request error", "error", err, "account", a.AccountID)
			attempts++
			continue
		}

		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			data, _ := resp["data"].(map[string]any)
			bizData, _ := data["biz_data"].(map[string]any)
			chatSessions, _ := bizData["chat_sessions"].([]any)
			hasMore, _ := bizData["has_more"].(bool)

			stats.FirstPageCount = len(chatSessions)
			stats.HasMore = hasMore
			stats.Success = true

			// 统计置顶会话数量
			for _, session := range chatSessions {
				if s, ok := session.(map[string]any); ok {
					if pinned, ok := s["pinned"].(bool); ok && pinned {
						stats.PinnedCount++
					}
				}
			}

			return stats, nil
		}

		stats.ErrorMessage = fmt.Sprintf("status=%d, code=%d, msg=%s", status, code, msg)
		config.Logger.Warn("[get_session_count] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "account", a.AccountID)

		if a.UseConfigToken {
			if isTokenInvalid(status, code, bizCode, msg, bizMsg) && !refreshed {
				if c.Auth.RefreshToken(ctx, a) {
					refreshed = true
					continue
				}
			}
			if c.Auth.SwitchAccount(ctx, a) {
				refreshed = false
				attempts++
				continue
			}
		}
		attempts++
	}

	stats.Success = false
	stats.ErrorMessage = "get session count failed after retries"
	return stats, errors.New(stats.ErrorMessage)
}

// GetSessionCountForToken 直接使用 token 获取会话数量（直通模式）
func (c *Client) GetSessionCountForToken(ctx context.Context, token string) (*SessionStats, error) {
	clients := c.requestClientsFromContext(ctx)
	headers := c.authHeaders(token)
	reqURL := DeepSeekFetchSessionURL + "?lte_cursor.pinned=false"

	resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, err
	}

	code, bizCode, msg, bizMsg := extractResponseStatus(resp)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		if strings.TrimSpace(bizMsg) != "" {
			msg = bizMsg
		}
		return nil, fmt.Errorf("request failed: status=%d, code=%d, msg=%s", status, code, msg)
	}

	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	chatSessions, _ := bizData["chat_sessions"].([]any)
	hasMore, _ := bizData["has_more"].(bool)

	stats := &SessionStats{
		FirstPageCount: len(chatSessions),
		HasMore:        hasMore,
		Success:        true,
	}

	// 统计置顶会话数量
	for _, session := range chatSessions {
		if s, ok := session.(map[string]any); ok {
			if pinned, ok := s["pinned"].(bool); ok && pinned {
				stats.PinnedCount++
			}
		}
	}

	return stats, nil
}

// GetSessionCountAll 获取所有账号的会话数量统计
func (c *Client) GetSessionCountAll(ctx context.Context) []*SessionStats {
	accounts := c.Store.Accounts()
	results := make([]*SessionStats, 0, len(accounts))

	for _, acc := range accounts {
		token := acc.Token
		accountID := acc.Email
		if accountID == "" {
			accountID = acc.Mobile
		}

		// 如果没有 token，尝试登录获取
		if token == "" {
			var err error
			token, err = c.Login(auth.WithAuth(ctx, &auth.RequestAuth{AccountID: acc.Identifier(), Account: acc}), acc)
			if err != nil {
				results = append(results, &SessionStats{
					AccountID:    accountID,
					Success:      false,
					ErrorMessage: fmt.Sprintf("login failed: %v", err),
				})
				continue
			}
		}

		ctxWithAuth := auth.WithAuth(ctx, &auth.RequestAuth{AccountID: acc.Identifier(), Account: acc, DeepSeekToken: token})
		stats, err := c.GetSessionCountForToken(ctxWithAuth, token)
		if err != nil {
			results = append(results, &SessionStats{
				AccountID:    accountID,
				Success:      false,
				ErrorMessage: err.Error(),
			})
			continue
		}

		stats.AccountID = accountID
		results = append(results, stats)
	}

	return results
}

// FetchSessionPage 获取会话列表（支持分页）
func (c *Client) FetchSessionPage(ctx context.Context, a *auth.RequestAuth, cursor string) ([]SessionInfo, bool, error) {
	clients := c.requestClientsForAuth(ctx, a)
	headers := c.authHeaders(a.DeepSeekToken)

	// 构建请求 URL
	params := url.Values{}
	params.Set("lte_cursor.pinned", "false")
	if cursor != "" {
		params.Set("lte_cursor", cursor)
	}
	reqURL := DeepSeekFetchSessionURL + "?" + params.Encode()

	resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, false, err
	}

	code := intFrom(resp["code"])
	if status != http.StatusOK || code != 0 {
		msg, _ := resp["msg"].(string)
		return nil, false, fmt.Errorf("request failed: status=%d, code=%d, msg=%s", status, code, msg)
	}

	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	chatSessions, _ := bizData["chat_sessions"].([]any)
	hasMore, _ := bizData["has_more"].(bool)

	sessions := make([]SessionInfo, 0, len(chatSessions))
	for _, s := range chatSessions {
		if m, ok := s.(map[string]any); ok {
			session := SessionInfo{
				ID:        stringFromMap(m, "id"),
				Title:     stringFromMap(m, "title"),
				TitleType: stringFromMap(m, "title_type"),
				Pinned:    boolFromMap(m, "pinned"),
				UpdatedAt: floatFromMap(m, "updated_at"),
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, hasMore, nil
}

// 辅助函数
func stringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolFromMap(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func floatFromMap(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
