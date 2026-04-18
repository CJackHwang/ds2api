package metrics

import (
	"net/http"
	"strings"
	"sync/atomic"
)

type RequestStats struct {
	success int64
	failed  int64
}

func NewRequestStats() *RequestStats {
	return &RequestStats{}
}

func (s *RequestStats) Snapshot() (success int64, failed int64) {
	if s == nil {
		return 0, 0
	}
	return atomic.LoadInt64(&s.success), atomic.LoadInt64(&s.failed)
}

func (s *RequestStats) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldTrackPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= http.StatusBadRequest {
			atomic.AddInt64(&s.failed, 1)
			return
		}
		atomic.AddInt64(&s.success, 1)
	})
}

func shouldTrackPath(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" || p == "/" {
		return false
	}
	if strings.HasPrefix(p, "/admin") || strings.HasPrefix(p, "/healthz") || strings.HasPrefix(p, "/readyz") {
		return false
	}
	// Track business API calls only.
	return strings.HasPrefix(p, "/v1") || strings.HasPrefix(p, "/v1beta") || strings.HasPrefix(p, "/anthropic") || strings.HasPrefix(p, "/messages")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

