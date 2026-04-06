package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
	"ds2api/internal/sse"
)

func (h *Handler) testSingleAccount(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	identifier, _ := req["identifier"].(string)
	if strings.TrimSpace(identifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要账号标识（identifier / email / mobile）"})
		return
	}
	acc, ok := findAccountByIdentifier(h.Store, identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	message, _ := req["message"].(string)
	result := h.testAccount(r.Context(), acc, model, message)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) testAllAccounts(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	accounts := h.Store.Snapshot().Accounts
	if len(accounts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "success": 0, "failed": 0, "results": []any{}})
		return
	}

	// Concurrent testing with a semaphore to limit parallelism.
	const maxConcurrency = 5
	results := runAccountTestsConcurrently(accounts, maxConcurrency, func(_ int, account config.Account) map[string]any {
		return h.testAccount(r.Context(), account, model, "")
	})

	success := 0
	for _, res := range results {
		if ok, _ := res["success"].(bool); ok {
			success++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": len(accounts), "success": success, "failed": len(accounts) - success, "results": results})
}

func runAccountTestsConcurrently(accounts []config.Account, maxConcurrency int, testFn func(int, config.Account) map[string]any) []map[string]any {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	sem := make(chan struct{}, maxConcurrency)
	results := make([]map[string]any, len(accounts))
	var wg sync.WaitGroup
	for i, acc := range accounts {
		wg.Add(1)
		go func(idx int, account config.Account) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release
			results[idx] = testFn(idx, account)
		}(i, acc)
	}
	wg.Wait()
	return results
}

func (h *Handler) testAccount(ctx context.Context, acc config.Account, model, message string) map[string]any {
	start := time.Now()
	identifier := acc.Identifier()
	result := map[string]any{
		"account":         identifier,
		"success":         false,
		"response_time":   0,
		"message":         "",
		"model":           model,
		"session_count":   0,
		"config_writable": !h.Store.IsEnvBacked(),
	}
	defer func() {
		status := "failed"
		if ok, _ := result["success"].(bool); ok {
			status = "ok"
		}
		_ = h.Store.UpdateAccountTestStatus(identifier, status)
	}()
	token, err := h.DS.Login(ctx, acc)
	if err != nil {
		result["message"] = "登录失败: " + err.Error()
		return result
	}
	if err := h.Store.UpdateAccountToken(acc.Identifier(), token); err != nil {
		result["message"] = "登录成功但写入运行时 token 失败: " + err.Error()
		return result
	}
	authCtx := &authn.RequestAuth{UseConfigToken: false, DeepSeekToken: token, AccountID: identifier, Account: acc}
	proxyCtx := authn.WithAuth(ctx, authCtx)
	sessionID, err := h.DS.CreateSession(proxyCtx, authCtx, 1)
	if err != nil {
		newToken, loginErr := h.DS.Login(proxyCtx, acc)
		if loginErr != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
		token = newToken
		authCtx.DeepSeekToken = token
		if err := h.Store.UpdateAccountToken(acc.Identifier(), token); err != nil {
			result["message"] = "刷新 token 成功但写入运行时 token 失败: " + err.Error()
			return result
		}
		sessionID, err = h.DS.CreateSession(proxyCtx, authCtx, 1)
		if err != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
	}

	// 获取会话数量
	sessionStats, sessionErr := h.DS.GetSessionCountForToken(proxyCtx, token)
	if sessionErr == nil && sessionStats != nil {
		result["session_count"] = sessionStats.FirstPageCount
	}

	if strings.TrimSpace(message) == "" {
		result["success"] = true
		result["message"] = "Token 刷新成功（登录与会话创建成功）"
		result["response_time"] = int(time.Since(start).Milliseconds())
		return result
	}
	thinking, search, ok := config.GetModelConfig(model)
	if !ok {
		thinking, search = false, false
	}
	_ = search
	pow, err := h.DS.GetPow(proxyCtx, authCtx, 1)
	if err != nil {
		result["message"] = "获取 PoW 失败: " + err.Error()
		return result
	}
	payload := map[string]any{"chat_session_id": sessionID, "prompt": deepseek.MessagesPrepare([]map[string]any{{"role": "user", "content": message}}), "ref_file_ids": []any{}, "thinking_enabled": thinking, "search_enabled": search}
	resp, err := h.DS.CallCompletion(proxyCtx, authCtx, payload, pow, 1)
	if err != nil {
		result["message"] = "请求失败: " + err.Error()
		return result
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		result["message"] = fmt.Sprintf("请求失败: HTTP %d", resp.StatusCode)
		return result
	}
	collected := sse.CollectStream(resp, thinking, true)
	result["success"] = true
	result["response_time"] = int(time.Since(start).Milliseconds())
	if collected.Text != "" {
		result["message"] = collected.Text
	} else {
		result["message"] = "（无回复内容）"
	}
	if collected.Thinking != "" {
		result["thinking"] = collected.Thinking
	}
	return result
}

func (h *Handler) testAPI(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	message, _ := req["message"].(string)
	apiKey, _ := req["api_key"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	if message == "" {
		message = "你好"
	}
	if apiKey == "" {
		keys := h.Store.Snapshot().Keys
		if len(keys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "没有可用的 API Key"})
			return
		}
		apiKey = keys[0]
	}
	host := r.Host
	scheme := "http"
	if strings.Contains(strings.ToLower(host), "vercel") || strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	payload := map[string]any{"model": model, "messages": []map[string]any{{"role": "user", "content": message}}, "stream": false}
	b, _ := json.Marshal(payload)
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, fmt.Sprintf("%s://%s/v1/chat/completions", scheme, host), bytes.NewReader(b))
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		var parsed any
		_ = json.Unmarshal(body, &parsed)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "status_code": resp.StatusCode, "response": parsed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": false, "status_code": resp.StatusCode, "response": string(body)})
}

func (h *Handler) deleteAllSessions(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	identifier, _ := req["identifier"].(string)
	if strings.TrimSpace(identifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要账号标识（identifier / email / mobile）"})
		return
	}
	acc, ok := findAccountByIdentifier(h.Store, identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}

	// 每次先登录刷新一次 token，避免使用过期 token。
	authCtx := &authn.RequestAuth{UseConfigToken: false, AccountID: acc.Identifier(), Account: acc}
	proxyCtx := authn.WithAuth(r.Context(), authCtx)
	token, err := h.DS.Login(proxyCtx, acc)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "登录失败: " + err.Error()})
		return
	}
	_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
	authCtx.DeepSeekToken = token

	// 删除所有会话
	err = h.DS.DeleteAllSessionsForToken(proxyCtx, token)
	if err != nil {
		// token 可能过期，尝试重新登录并重试一次
		newToken, loginErr := h.DS.Login(proxyCtx, acc)
		if loginErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "删除失败: " + err.Error()})
			return
		}
		token = newToken
		_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
		authCtx.DeepSeekToken = token
		if retryErr := h.DS.DeleteAllSessionsForToken(proxyCtx, token); retryErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "删除失败: " + retryErr.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "删除成功"})
}
