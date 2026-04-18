package metrics

import (
	"bufio"
	"fmt"
	"net/http"
	"net"
	"strings"
	"sync/atomic"
)

type RequestStats struct {
	success int64
	failed  int64
	redis   *redisCounterStore
}

func NewRequestStats() *RequestStats {
	return &RequestStats{redis: newRedisCounterStoreFromEnv()}
}

func (s *RequestStats) Snapshot() (success int64, failed int64) {
	if s == nil {
		return 0, 0
	}
	if s.redis != nil {
		if rs, rf, err := s.redis.Snapshot(); err == nil {
			return rs, rf
		}
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
			if s.redis != nil {
				_ = s.redis.IncrFailed()
			}
			return
		}
		atomic.AddInt64(&s.success, 1)
		if s.redis != nil {
			_ = s.redis.IncrSuccess()
		}
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
	// Track all non-admin requests so reverse-proxy prefixes (/api, etc.) are
	// included as well.
	return true
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return h.Hijack()
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	p, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
}
