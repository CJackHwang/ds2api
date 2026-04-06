package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

func (c *Client) CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	clients := c.requestClientsForAuth(ctx, a)
	headers := c.authHeaders(a.DeepSeekToken)
	headers["x-ds-pow-response"] = powResp
	captureSession := c.capture.Start("deepseek_completion", DeepSeekCompletionURL, a.AccountID, payload)
	attempts := 0
	for attempts < maxAttempts {
		resp, err := c.streamPost(ctx, clients.stream, DeepSeekCompletionURL, headers, payload)
		if err != nil {
			attempts++
			time.Sleep(time.Second)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			if captureSession != nil {
				resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
			}
			resp = c.wrapCompletionWithAutoContinue(ctx, a, payload, powResp, resp)
			return resp, nil
		}
		if captureSession != nil {
			resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
		}
		_ = resp.Body.Close()
		attempts++
		time.Sleep(time.Second)
	}
	return nil, errors.New("completion failed")
}

func (c *Client) streamPost(ctx context.Context, doer trans.Doer, url string, headers map[string]string, payload any) (*http.Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	clients := c.requestClientsFromContext(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err != nil {
		config.Logger.Warn("[deepseek] fingerprint stream request failed, fallback to std transport", "url", url, "error", err)
		req2, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if reqErr != nil {
			return nil, reqErr
		}
		for k, v := range headers {
			req2.Header.Set(k, v)
		}
		return clients.fallbackS.Do(req2)
	}
	return resp, nil
}
