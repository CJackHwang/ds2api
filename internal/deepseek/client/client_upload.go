package client

import (
	"bytes"
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

type UploadFileRequest struct {
	Filename    string
	ContentType string
	Purpose     string
	ModelType   string
	Data        []byte
}

type UploadFileResult struct {
	ID         string
	Filename   string
	Bytes      int64
	Status     string
	Purpose    string
	AccountID  string
	IsImage    bool
	Raw        map[string]any
	RawHeaders http.Header
}

func (c *Client) UploadFile(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	if len(req.Data) == 0 {
		return nil, errors.New("file is required")
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "upload.bin"
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	purpose := strings.TrimSpace(req.Purpose)
	modelType := strings.ToLower(strings.TrimSpace(req.ModelType))
	body, contentTypeHeader, err := buildUploadMultipartBody(filename, contentType, req.Data)
	if err != nil {
		return nil, err
	}
	capturePayload := map[string]any{
		"filename":     filename,
		"content_type": contentType,
		"purpose":      purpose,
		"bytes":        len(req.Data),
	}
	if modelType != "" {
		capturePayload["model_type"] = modelType
	}
	captureSession := c.capture.Start("deepseek_upload_file", dsprotocol.DeepSeekUploadFileURL, a.AccountID, capturePayload)
	attempts := 0
	refreshed := false
	powHeader := ""
	lastFailureKind := FailureUnknown
	lastFailureMessage := ""
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		if strings.TrimSpace(powHeader) == "" {
			powHeader, err = c.GetPowForTarget(ctx, a, dsprotocol.DeepSeekUploadTargetPath, maxAttempts)
			if err != nil {
				return nil, err
			}
			clients = c.requestClientsForAuth(ctx, a)
		}
		headers := c.authHeaders(a.DeepSeekToken)
		headers["Content-Type"] = contentTypeHeader
		if modelType != "" {
			headers["x-model-type"] = modelType
		}
		headers["x-ds-pow-response"] = powHeader
		headers["x-file-size"] = strconv.Itoa(len(req.Data))
		headers["x-thinking-enabled"] = "1"
		resp, err := c.doUpload(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekUploadFileURL, headers, body)
		if err != nil {
			config.Logger.Warn("[upload_file] request error", "error", err, "account", a.AccountID, "filename", filename)
			return nil, err
		}
		if captureSession != nil {
			resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
		}
		payloadBytes, readErr := readResponseBody(resp)
		_ = resp.Body.Close()
		if readErr != nil {
			powHeader = ""
			attempts++
			continue
		}
		parsed := map[string]any{}
		if len(payloadBytes) > 0 {
			if err := json.Unmarshal(payloadBytes, &parsed); err != nil {
				config.Logger.Warn("[upload_file] json parse failed", "status", resp.StatusCode, "preview", preview(payloadBytes))
			}
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(parsed)
		if resp.StatusCode == http.StatusOK && code == 0 && bizCode == 0 {
			result := extractUploadFileResult(parsed)
			result.Raw = parsed
			result.RawHeaders = resp.Header.Clone()
			if result.Filename == "" {
				result.Filename = filename
			}
			if result.Bytes == 0 {
				result.Bytes = int64(len(req.Data))
			}
			if result.Purpose == "" {
				result.Purpose = purpose
			}
			if result.AccountID == "" {
				result.AccountID = a.AccountID
			}
			if result.ID == "" {
				return nil, errors.New("upload file succeeded without file id")
			}
			if err := c.waitForUploadedFile(ctx, a, result); err != nil {
				return nil, err
			}
			return result, nil
		}
		config.Logger.Warn("[upload_file] failed", "status", resp.StatusCode, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "account", a.AccountID, "filename", filename)
		powHeader = ""
		lastFailureMessage = failureMessage(msg, bizMsg, "upload file failed")
		if isTokenInvalid(resp.StatusCode, code, bizCode, msg, bizMsg) || isAuthIndicativeBizFailure(msg, bizMsg) {
			lastFailureKind = authFailureKind(a.UseConfigToken)
		} else {
			lastFailureKind = FailureUnknown
		}
		if a.UseConfigToken {
			if !refreshed && shouldAttemptRefresh(resp.StatusCode, code, bizCode, msg, bizMsg) {
				if c.Auth.RefreshToken(ctx, a) {
					refreshed = true
					attempts++
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
	if lastFailureKind != FailureUnknown {
		return nil, &RequestFailure{Op: "upload file", Kind: lastFailureKind, Message: lastFailureMessage}
	}
	return nil, errors.New("upload file failed")
}

func buildUploadMultipartBody(filename, contentType string, data []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, escapeMultipartFilename(filename)))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

func escapeMultipartFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	filename = strings.ReplaceAll(filename, `\`, "_")
	filename = strings.ReplaceAll(filename, `"`, "_")
	if filename == "." || filename == "" {
		return "upload.bin"
	}
	return filename
}

func (c *Client) doUpload(ctx context.Context, doer trans.Doer, _ trans.Doer, url string, headers map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err == nil {
		return resp, nil
	}
	return nil, err
}

func extractUploadFileResult(resp map[string]any) *UploadFileResult {
	result := &UploadFileResult{Status: "uploaded"}
	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	searchMaps := []map[string]any{resp, data, bizData}
	for _, parent := range []map[string]any{resp, data, bizData} {
		if parent == nil {
			continue
		}
		for _, key := range []string{"file", "biz_data", "data"} {
			if nested, ok := parent[key].(map[string]any); ok {
				searchMaps = append(searchMaps, nested)
			}
		}
	}
	for _, m := range searchMaps {
		if m == nil {
			continue
		}
		if result.ID == "" {
			result.ID = firstNonEmptyString(m, "id", "file_id")
		}
		if result.Filename == "" {
			result.Filename = firstNonEmptyString(m, "name", "filename", "file_name")
		}
		if result.Status == "uploaded" {
			if status := firstNonEmptyString(m, "status", "file_status"); status != "" {
				result.Status = status
			}
		}
		if !result.IsImage {
			result.IsImage = firstBool(m, "is_image", "isImage")
		}
		if result.Purpose == "" {
			result.Purpose = firstNonEmptyString(m, "purpose")
		}
		if result.AccountID == "" {
			result.AccountID = firstNonEmptyString(m, "account_id", "accountId", "owner_account_id", "ownerAccountId")
		}
		if result.Bytes == 0 {
			result.Bytes = firstPositiveInt64(m, "bytes", "size", "file_size")
		}
	}
	return result
}

func firstBool(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch v := m[key].(type) {
		case bool:
			return v
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "y":
				return true
			}
		}
	}
	return false
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, _ := m[key].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstPositiveInt64(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v := toInt64(m[key], 0); v > 0 {
			return v
		}
	}
	return 0
}
