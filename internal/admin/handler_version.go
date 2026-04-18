package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/version"
)

const latestReleaseAPI = "https://api.github.com/repos/CJackHwang/ds2api/releases/latest"

type latestReleasePayload struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func (h *Handler) getVersion(w http.ResponseWriter, _ *http.Request) {
	current, source := version.Current()
	resp := map[string]any{
		"success":         true,
		"current_version": current,
		"current_tag":     version.Tag(current),
		"source":          source,
		"checked_at":      time.Now().UTC().Format(time.RFC3339),
	}
	if h.Stats != nil {
		successCalls, failedCalls := h.Stats.Snapshot()
		resp["success_calls"] = successCalls
		resp["failed_calls"] = failedCalls
	}

	req, err := http.NewRequest(http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		resp["check_error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ds2api-version-check")

	client := &http.Client{Timeout: 4 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		resp["check_error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		resp["check_error"] = "github api status: " + r.Status
		writeJSON(w, http.StatusOK, resp)
		return
	}

	var data latestReleasePayload
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		resp["check_error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	latest := strings.TrimSpace(data.TagName)
	if latest == "" {
		resp["check_error"] = "missing latest tag"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	latestVersion := strings.TrimPrefix(latest, "v")

	resp["latest_tag"] = latest
	resp["latest_version"] = latestVersion
	resp["release_url"] = data.HTMLURL
	resp["published_at"] = data.PublishedAt
	resp["has_update"] = version.Compare(current, latestVersion) < 0

	writeJSON(w, http.StatusOK, resp)
}
