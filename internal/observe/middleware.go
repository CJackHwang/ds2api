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

			// Copy fields under lock, then release before I/O.
			m.mu.Lock()
			modelAlias := m.ModelAlias
			accountID := m.AccountID
			surface := m.Surface
			retryCount := m.RetryCount
			accountSwitchCount := m.AccountSwitchCount
			startTime := m.StartTime
			firstByteAt := m.FirstByteAt
			upstreamResponseAt := m.UpstreamResponseAt
			routeField := m.Route
			parserV2Mode := m.ParserV2Mode
			toolCallCount := m.ToolCallCount
			shadowDiffRan := m.ShadowDiffRanCount
			shadowDiffHit := m.ShadowDiffHitCount
			suppressedCharCount := m.SuppressedCharCount
			m.mu.Unlock()

			// Only emit for completion routes (model was populated).
			if modelAlias == "" {
				return
			}

			// Derive route from chi context if not explicitly set.
			if routeField == "" {
				if rctx := chi.RouteContext(r.Context()); rctx != nil {
					routeField = rctx.RoutePattern()
				}
			}
			if routeField == "" {
				routeField = r.URL.Path
			}

			requestID := strings.TrimSpace(chiMiddleware.GetReqID(ctx))

			elapsed := time.Since(startTime)
			ttftMs := elapsed.Milliseconds()
			if !firstByteAt.IsZero() {
				ttftMs = firstByteAt.Sub(startTime).Milliseconds()
			}
			var upstreamTTFBMs int64
			if !upstreamResponseAt.IsZero() {
				upstreamTTFBMs = upstreamResponseAt.Sub(startTime).Milliseconds()
			}

			logger.Info("[completion_request]",
				"request_id", requestID,
				"route", routeField,
				"model_alias", modelAlias,
				"account_id", accountID,
				"surface", surface,
				"ttft_ms", ttftMs,
				"upstream_ttft_ms", upstreamTTFBMs,
				"retry_count", retryCount,
				"account_switch_count", accountSwitchCount,
				"total_ms", elapsed.Milliseconds(),
				"parser_v2_mode", parserV2Mode,
				"parser_tool_call_count", toolCallCount,
				"parser_shadow_diff_ran", shadowDiffRan,
				"parser_shadow_diff_hit", shadowDiffHit,
				"parser_suppressed_char_count", suppressedCharCount,
			)
		})
	}
}
