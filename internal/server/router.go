package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ds2api/internal/account"
	"ds2api/internal/adapter/claude"
	"ds2api/internal/adapter/gemini"
	"ds2api/internal/adapter/openai"
	"ds2api/internal/admin"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
	"ds2api/internal/webui"
)

type App struct {
	Store    *config.Store
	Pool     *account.Pool
	Resolver *auth.Resolver
	DS       *deepseek.Client
	Router   http.Handler
}

func NewApp() (*App, error) {
	store, err := config.LoadStoreWithError()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	pool := account.NewPool(store)
	var dsClient *deepseek.Client
	resolver := auth.NewResolver(store, pool, func(ctx context.Context, acc config.Account) (string, error) {
		return dsClient.Login(ctx, acc)
	})
	dsClient = deepseek.NewClient(store, resolver)
	if err := dsClient.PreloadPow(context.Background()); err != nil {
		config.Logger.Warn("[PoW] init failed", "error", err)
	} else {
		config.Logger.Info("[PoW] pure Go solver ready")
	}

	openaiHandler := &openai.Handler{Store: store, Auth: resolver, DS: dsClient}
	claudeHandler := &claude.Handler{Store: store, Auth: resolver, DS: dsClient, OpenAI: openaiHandler}
	geminiHandler := &gemini.Handler{Store: store, Auth: resolver, DS: dsClient, OpenAI: openaiHandler}
	adminHandler := &admin.Handler{Store: store, Pool: pool, DS: dsClient, OpenAI: openaiHandler}
	webuiHandler := webui.NewHandler()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors)
	r.Use(timeout(0))
	r.Use(adminNoStore)
	r.Use(trackAPICallMetrics(store))

	healthzHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
	readyzHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
	r.Get("/healthz", healthzHandler)
	r.Head("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler)
	r.Head("/readyz", readyzHandler)
	openai.RegisterRoutes(r, openaiHandler)
	claude.RegisterRoutes(r, claudeHandler)
	gemini.RegisterRoutes(r, geminiHandler)
	r.Route("/admin", func(ar chi.Router) {
		admin.RegisterRoutes(ar, adminHandler)
	})
	webui.RegisterRoutes(r, webuiHandler)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/admin/") && webuiHandler.HandleAdminFallback(w, req) {
			return
		}
		http.NotFound(w, req)
	})

	return &App{Store: store, Pool: pool, Resolver: resolver, DS: dsClient, Router: r}, nil
}

func timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.Timeout(d)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Ds2-Target-Account, X-Vercel-Protection-Bypass")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func WriteUnhandledError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "api_error", "message": "Internal Server Error", "detail": err.Error()}})
}

func adminNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/admin/assets/") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		next.ServeHTTP(w, r)
	})
}

func trackAPICallMetrics(store *config.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !shouldTrackAPICall(r) || store == nil {
				next.ServeHTTP(w, r)
				return
			}
			rec := &statusCapturingResponseWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			store.RecordCall(status < http.StatusBadRequest)
		})
	}
}

func shouldTrackAPICall(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/v1/chat/completions",
		"/v1/responses",
		"/v1/files",
		"/v1/embeddings",
		"/anthropic/v1/messages",
		"/v1/messages",
		"/messages":
		return true
	}
	return strings.Contains(r.URL.Path, ":generateContent") || strings.Contains(r.URL.Path, ":streamGenerateContent")
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusCapturingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusCapturingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijack")
	}
	return hijacker.Hijack()
}

func (w *statusCapturingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *statusCapturingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	readerFrom, ok := w.ResponseWriter.(io.ReaderFrom)
	if !ok {
		return io.Copy(w.ResponseWriter, r)
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return readerFrom.ReadFrom(r)
}
