// Package observe provides request-scoped structured observability for
// completion requests. The Middleware injects a *RequestMetrics into context;
// downstream code (completionruntime, handlers) populates fields via context
// helpers. On request completion, the middleware emits a single structured
// slog line containing all captured metrics.
//
// Field naming convention (shared with Tool Parser M2 / Context Engine M2):
//
//	request_id, route, model_alias, account_id, ttft_ms,
//	upstream_ttft_ms, retry_count, account_switch_count
package observe

import (
	"context"
	"sync"
	"time"
)

type contextKey struct{}

// RequestMetrics accumulates per-request observability fields.
// All mutating methods are goroutine-safe.
type RequestMetrics struct {
	mu sync.Mutex

	// Route is the matched chi route pattern (e.g. "/v1/chat/completions").
	Route string
	// ModelAlias is the user-facing model name from the request.
	ModelAlias string
	// AccountID is the redacted identifier of the account used.
	AccountID string
	// Surface identifies the protocol surface (e.g. "openai.chat", "claude").
	Surface string

	// RetryCount is the number of empty-output retry attempts.
	RetryCount int
	// AccountSwitchCount is the number of times the request switched accounts.
	AccountSwitchCount int

	// StartTime is when the request entered the middleware.
	StartTime time.Time
	// UpstreamResponseAt is when the upstream HTTP response headers arrived.
	UpstreamResponseAt time.Time
	// FirstByteAt is when the first byte was written to the client.
	FirstByteAt time.Time
}

// WithMetrics returns a new context carrying the given metrics.
func WithMetrics(ctx context.Context, m *RequestMetrics) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}

// FromContext returns the *RequestMetrics from ctx, or nil if not present.
func FromContext(ctx context.Context) *RequestMetrics {
	m, _ := ctx.Value(contextKey{}).(*RequestMetrics)
	return m
}

// SetRoute records the matched route pattern.
func SetRoute(ctx context.Context, route string) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.Route = route
		m.mu.Unlock()
	}
}

// SetModel records the model alias from the request.
func SetModel(ctx context.Context, model string) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.ModelAlias = model
		m.mu.Unlock()
	}
}

// SetAccount records the redacted account identifier.
func SetAccount(ctx context.Context, accountID string) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.AccountID = redactAccountID(accountID)
		m.mu.Unlock()
	}
}

// SetSurface records the protocol surface name.
func SetSurface(ctx context.Context, surface string) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.Surface = surface
		m.mu.Unlock()
	}
}

// IncrRetry increments the retry counter.
func IncrRetry(ctx context.Context) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.RetryCount++
		m.mu.Unlock()
	}
}

// IncrAccountSwitch increments the account switch counter.
func IncrAccountSwitch(ctx context.Context) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		m.AccountSwitchCount++
		m.mu.Unlock()
	}
}

// SetUpstreamResponseAt records when upstream response headers arrived.
func SetUpstreamResponseAt(ctx context.Context, t time.Time) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		if m.UpstreamResponseAt.IsZero() {
			m.UpstreamResponseAt = t
		}
		m.mu.Unlock()
	}
}

// SetFirstByteAt records when the first byte was written to the client.
func SetFirstByteAt(ctx context.Context, t time.Time) {
	if m := FromContext(ctx); m != nil {
		m.mu.Lock()
		if m.FirstByteAt.IsZero() {
			m.FirstByteAt = t
		}
		m.mu.Unlock()
	}
}

// redactAccountID masks the account identifier for safe logging.
// Keeps only the first 4 characters followed by "***".
func redactAccountID(id string) string {
	if len(id) <= 4 {
		return "***"
	}
	return id[:4] + "***"
}
