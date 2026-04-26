package shared

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
)

type EmptyOutputRetryOptions struct {
	Context     context.Context
	DS          DeepSeekCaller
	Auth        *auth.RequestAuth
	Payload     map[string]any
	PowResponse string
	MaxAttempts int
	Now         time.Time
}

func ShouldRetryUpstreamEmptyOutput(text string, contentFilter bool) bool {
	if contentFilter {
		return false
	}
	return strings.TrimSpace(text) == ""
}

func RetryUpstreamEmptyOutputWithTimestamp(opts EmptyOutputRetryOptions) (*http.Response, bool, error) {
	if opts.DS == nil || opts.Auth == nil || len(opts.Payload) == 0 {
		return nil, false, nil
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	payload := cloneCompletionPayload(opts.Payload)
	originalPrompt, _ := payload["prompt"].(string)
	payload["prompt"] = appendEmptyOutputRetryTimestamp(originalPrompt, now)
	resp, err := opts.DS.CallCompletion(ctx, opts.Auth, payload, opts.PowResponse, maxAttempts)
	if err != nil {
		return nil, true, err
	}
	return resp, true, nil
}

func appendEmptyOutputRetryTimestamp(prompt string, now time.Time) string {
	dt := now.UTC().Format(time.RFC3339)
	suffix := fmt.Sprintf("\n\n[retry_datetime_utc]%s[/retry_datetime_utc]", dt)
	return prompt + suffix
}

func cloneCompletionPayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload))
	for k, v := range payload {
		cloned[k] = v
	}
	return cloned
}
