package admin

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ds2api/internal/config"
)

func (h *Handler) syncVercel(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	opts, err := parseVercelSyncOptions(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}

	validated, failed := h.validateAccountsForVercelSync(r.Context(), opts.AutoValidate)
	cfgJSON, _, err := h.exportSyncConfig(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	if err := h.Store.SaveConfigToRedis(r.Context(), cfgJSON, opts.RedisURL, opts.RedisKey); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "save config to redis failed: " + err.Error()})
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	params := buildVercelParams(opts.TeamID)
	headers := map[string]string{"Authorization": "Bearer " + opts.VercelToken}

	envResp, status, err := vercelRequest(r.Context(), client, http.MethodGet, "https://api.vercel.com/v9/projects/"+opts.ProjectID+"/env", params, headers, nil)
	if err != nil || status != http.StatusOK {
		writeJSON(w, statusOr(status, http.StatusInternalServerError), map[string]any{"detail": "failed to fetch vercel envs"})
		return
	}
	envs, _ := envResp["envs"].([]any)

	status, err = upsertVercelEnv(r.Context(), client, opts.ProjectID, params, headers, envs, "DS2API_REDIS_URL", opts.RedisURL)
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		writeJSON(w, statusOr(status, http.StatusInternalServerError), map[string]any{"detail": "failed to sync redis url"})
		return
	}
	status, err = upsertVercelEnv(r.Context(), client, opts.ProjectID, params, headers, envs, "DS2API_REDIS_KEY", opts.RedisKey)
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		writeJSON(w, statusOr(status, http.StatusInternalServerError), map[string]any{"detail": "failed to sync redis key"})
		return
	}
	if err := deleteVercelEnv(r.Context(), client, opts.ProjectID, params, headers, envs, "DS2API_CONFIG_JSON"); err != nil {
		config.Logger.Warn("[vercel_sync] failed to delete legacy env", "error", err)
	}

	savedCreds := h.saveVercelProjectCredentials(r.Context(), client, opts, params, headers, envs)
	manual, deployURL := triggerVercelDeployment(r.Context(), client, opts.ProjectID, params, headers)
	_ = h.Store.SetVercelSync(syncHashForJSON(cfgJSON), time.Now().Unix())

	result := map[string]any{
		"success":            true,
		"validated_accounts": validated,
		"storage_backend":    "redis",
		"redis_key":          opts.RedisKey,
	}
	if manual {
		result["message"] = "Config synced to Redis and Vercel. Trigger a redeploy manually."
		result["manual_deploy_required"] = true
	} else {
		result["message"] = "Config synced to Redis. Vercel redeploy has been triggered."
		result["deployment_url"] = deployURL
	}
	if len(failed) > 0 {
		result["failed_accounts"] = failed
	}
	if len(savedCreds) > 0 {
		result["saved_credentials"] = savedCreds
	}
	writeJSON(w, http.StatusOK, result)
}

type vercelSyncOptions struct {
	VercelToken  string
	ProjectID    string
	TeamID       string
	RedisURL     string
	RedisKey     string
	AutoValidate bool
	SaveCreds    bool
	UsePreconfig bool
}

func parseVercelSyncOptions(req map[string]any) (vercelSyncOptions, error) {
	vercelToken, _ := req["vercel_token"].(string)
	projectID, _ := req["project_id"].(string)
	teamID, _ := req["team_id"].(string)
	redisURL, _ := req["redis_url"].(string)
	redisKey, _ := req["redis_key"].(string)

	autoValidate := true
	if v, ok := req["auto_validate"].(bool); ok {
		autoValidate = v
	}
	saveCreds := true
	if v, ok := req["save_credentials"].(bool); ok {
		saveCreds = v
	}

	usePreconfig := vercelToken == "__USE_PRECONFIG__" || strings.TrimSpace(vercelToken) == ""
	if usePreconfig {
		vercelToken = strings.TrimSpace(os.Getenv("VERCEL_TOKEN"))
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = strings.TrimSpace(os.Getenv("VERCEL_PROJECT_ID"))
	}
	if strings.TrimSpace(teamID) == "" {
		teamID = strings.TrimSpace(os.Getenv("VERCEL_TEAM_ID"))
	}
	if strings.TrimSpace(redisURL) == "" {
		redisURL = firstNonEmptyRedisURL()
	}
	if strings.TrimSpace(redisKey) == "" {
		redisKey = strings.TrimSpace(os.Getenv("DS2API_REDIS_KEY"))
		if redisKey == "" {
			redisKey = "ds2api:config"
		}
	}

	vercelToken = strings.TrimSpace(vercelToken)
	projectID = strings.TrimSpace(projectID)
	teamID = strings.TrimSpace(teamID)
	redisURL = strings.TrimSpace(redisURL)
	redisKey = strings.TrimSpace(redisKey)

	if vercelToken == "" || projectID == "" {
		return vercelSyncOptions{}, fmt.Errorf("vercel token and project id are required")
	}
	if redisURL == "" {
		return vercelSyncOptions{}, fmt.Errorf("redis url is required")
	}
	if redisKey == "" {
		return vercelSyncOptions{}, fmt.Errorf("redis key is required")
	}

	return vercelSyncOptions{
		VercelToken:  vercelToken,
		ProjectID:    projectID,
		TeamID:       teamID,
		RedisURL:     redisURL,
		RedisKey:     redisKey,
		AutoValidate: autoValidate,
		SaveCreds:    saveCreds,
		UsePreconfig: usePreconfig,
	}, nil
}

func buildVercelParams(teamID string) url.Values {
	params := url.Values{}
	if strings.TrimSpace(teamID) != "" {
		params.Set("teamId", strings.TrimSpace(teamID))
	}
	return params
}

func (h *Handler) validateAccountsForVercelSync(ctx context.Context, enabled bool) (int, []string) {
	if !enabled {
		return 0, nil
	}
	validated, failed := 0, []string{}
	for _, acc := range h.Store.Snapshot().Accounts {
		if strings.TrimSpace(acc.Token) != "" {
			continue
		}
		token, err := h.DS.Login(ctx, acc)
		if err != nil {
			failed = append(failed, acc.Identifier())
		} else {
			validated++
			_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return validated, failed
}

func upsertVercelEnv(ctx context.Context, client *http.Client, projectID string, params url.Values, headers map[string]string, envs []any, key, value string) (int, error) {
	existingID := findEnvID(envs, key)
	if existingID != "" {
		_, status, err := vercelRequest(ctx, client, http.MethodPatch, "https://api.vercel.com/v9/projects/"+projectID+"/env/"+existingID, params, headers, map[string]any{"value": value})
		return status, err
	}
	_, status, err := vercelRequest(ctx, client, http.MethodPost, "https://api.vercel.com/v10/projects/"+projectID+"/env", params, headers, map[string]any{
		"key":    key,
		"value":  value,
		"type":   "encrypted",
		"target": []string{"production", "preview"},
	})
	return status, err
}

func deleteVercelEnv(ctx context.Context, client *http.Client, projectID string, params url.Values, headers map[string]string, envs []any, key string) error {
	existingID := findEnvID(envs, key)
	if existingID == "" {
		return nil
	}
	_, status, err := vercelRequest(ctx, client, http.MethodDelete, "https://api.vercel.com/v9/projects/"+projectID+"/env/"+existingID, params, headers, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("delete env status=%d", status)
	}
	return nil
}

func (h *Handler) saveVercelProjectCredentials(ctx context.Context, client *http.Client, opts vercelSyncOptions, params url.Values, headers map[string]string, envs []any) []string {
	if !opts.SaveCreds || opts.UsePreconfig {
		return nil
	}
	saved := []string{}
	creds := [][2]string{{"VERCEL_TOKEN", opts.VercelToken}, {"VERCEL_PROJECT_ID", opts.ProjectID}}
	if opts.TeamID != "" {
		creds = append(creds, [2]string{"VERCEL_TEAM_ID", opts.TeamID})
	}
	for _, kv := range creds {
		status, _ := upsertVercelEnv(ctx, client, opts.ProjectID, params, headers, envs, kv[0], kv[1])
		if status == http.StatusOK || status == http.StatusCreated {
			saved = append(saved, kv[0])
		}
	}
	return saved
}

func triggerVercelDeployment(ctx context.Context, client *http.Client, projectID string, params url.Values, headers map[string]string) (bool, string) {
	projectResp, status, _ := vercelRequest(ctx, client, http.MethodGet, "https://api.vercel.com/v9/projects/"+projectID, params, headers, nil)
	if status != http.StatusOK {
		return true, ""
	}
	link, ok := projectResp["link"].(map[string]any)
	if !ok {
		return true, ""
	}
	linkType, _ := link["type"].(string)
	if linkType != "github" {
		return true, ""
	}
	repoID := intFrom(link["repoId"])
	ref, _ := link["productionBranch"].(string)
	if ref == "" {
		ref = "main"
	}
	depResp, depStatus, _ := vercelRequest(ctx, client, http.MethodPost, "https://api.vercel.com/v13/deployments", params, headers, map[string]any{
		"name":    projectID,
		"project": projectID,
		"target":  "production",
		"gitSource": map[string]any{
			"type":   "github",
			"repoId": repoID,
			"ref":    ref,
		},
	})
	if depStatus != http.StatusOK && depStatus != http.StatusCreated {
		return true, ""
	}
	deployURL, _ := depResp["url"].(string)
	return false, deployURL
}

func (h *Handler) vercelStatus(w http.ResponseWriter, r *http.Request) {
	snap := h.Store.Snapshot()
	current := h.computeSyncHash()
	synced := snap.VercelSyncHash != "" && snap.VercelSyncHash == current
	draftHash := ""
	draftDiffers := false
	if r != nil && r.Method == http.MethodPost && r.Body != nil {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if cfgJSON, _, err := h.exportSyncConfig(req); err == nil {
				draftHash = syncHashForJSON(cfgJSON)
				draftDiffers = draftHash != "" && draftHash != current
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"synced":             synced,
		"last_sync_time":     nilIfZero(snap.VercelSyncTime),
		"has_synced_before":  snap.VercelSyncHash != "",
		"env_backed":         h.Store.IsEnvBacked(),
		"storage_backend":    h.Store.StorageBackend(),
		"persistent_storage": h.Store.IsPersistentStorage(),
		"redis_configured":   h.Store.IsRedisConfigured(),
		"redis_config_key":   nilIfEmpty(h.Store.RedisConfigKey()),
		"config_hash":        current,
		"last_synced_hash":   snap.VercelSyncHash,
		"draft_hash":         draftHash,
		"draft_differs":      draftDiffers,
	})
}

func (h *Handler) exportSyncConfig(req map[string]any) (string, string, error) {
	override, ok := req["config_override"]
	if !ok || override == nil {
		return h.Store.ExportJSONAndBase64()
	}
	raw, err := json.Marshal(override)
	if err != nil {
		return "", "", err
	}
	var cfg config.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", "", err
	}
	cfg.DropInvalidAccounts()
	cfg.ClearAccountTokens()
	cfg.VercelSyncHash = ""
	cfg.VercelSyncTime = 0
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", "", err
	}
	return string(b), base64.StdEncoding.EncodeToString(b), nil
}

func syncHashForJSON(s string) string {
	var cfg config.Config
	if err := json.Unmarshal([]byte(s), &cfg); err != nil {
		return ""
	}
	cfg.VercelSyncHash = ""
	cfg.VercelSyncTime = 0
	cfg.ClearAccountTokens()
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}

func vercelRequest(ctx context.Context, client *http.Client, method, endpoint string, params url.Values, headers map[string]string, body any) (map[string]any, int, error) {
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	parsed := map[string]any{}
	_ = json.Unmarshal(b, &parsed)
	if len(parsed) == 0 {
		parsed["raw"] = string(b)
	}
	return parsed, resp.StatusCode, nil
}

func findEnvID(envs []any, key string) string {
	for _, item := range envs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if k, _ := m["key"].(string); k == key {
			id, _ := m["id"].(string)
			return id
		}
	}
	return ""
}

func firstNonEmptyRedisURL() string {
	for _, key := range []string{"DS2API_REDIS_URL", "KV_URL", "REDIS_URL", "UPSTASH_REDIS_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
