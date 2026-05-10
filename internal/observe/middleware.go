package observe

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

// Middleware injects a *RequestMetrics into the request context and emits a
// structured log line after the handler returns. Only requests where a model
// was set (i.e. completion routes) produce the log entry; other routes are
// passed through silently.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := &RequestMetrics{StartTime: time.Now()}
			ctx := WithMetrics(r.Context(), m)
			next.ServeHTTP(w, r.WithContext(ctx))

			// Only emit for completion routes (model was populated).
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.ModelAlias == "" {
				return
			}

			// Derive route from chi context if not explicitly set.
			route := m.Route
			if route == "" {
				if rctx := chi.RouteContext(r.Context()); rctx != nil {
					route = rctx.RoutePattern()
				}
			}
			if route == "" {
				route = r.URL.Path
			}

			requestID := strings.TrimSpace(chiMiddleware.GetReqID(ctx))

			elapsed := time.Since(m.StartTime)
			ttftMs := elapsed.Milliseconds()
			if !m.FirstByteAt.IsZero() {
				ttftMs = m.FirstByteAt.Sub(m.StartTime).Milliseconds()
			}
			var upstreamTTFBMs int64
			if !m.UpstreamResponseAt.IsZero() {
				upstreamTTFBMs = m.UpstreamResponseAt.Sub(m.StartTime).Milliseconds()
			}

			logger.Info("[completion_request]",
				"request_id", requestID,
				"route", route,
				"model_alias", m.ModelAlias,
				"account_id", m.AccountID,
				"surface", m.Surface,
				"ttft_ms", ttftMs,
				"upstream_ttft_ms", upstreamTTFBMs,
				"retry_count", m.RetryCount,
				"account_switch_count", m.AccountSwitchCount,
				"total_ms", elapsed.Milliseconds(),
			)
		})
	}
}
