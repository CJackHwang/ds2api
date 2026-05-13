package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/admin"
	"ds2api/internal/httpapi/claude"
	"ds2api/internal/httpapi/gemini"
	"ds2api/internal/httpapi/ollama"
	"ds2api/internal/httpapi/openai/chat"
	"ds2api/internal/httpapi/openai/embeddings"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/responses"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/webui"
)

type App struct {
	Store    *config.Store
	Pool     *account.Pool
	Resolver *auth.Resolver
	DS       *dsclient.Client
	Router   http.Handler
}

func NewApp() (*App, error) {
	store, err := config.LoadStoreWithError()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	pool := account.NewPool(store)
	var dsClient *dsclient.Client
	resolver := auth.NewResolver(store, pool, func(ctx context.Context, acc config.Account) (string, error) {
		return dsClient.Login(ctx, acc)
	})
	dsClient = dsclient.NewClient(store, resolver)
	if err := dsClient.PreloadPow(context.Background()); err != nil {
		config.Logger.Warn("[PoW] init failed", "error", err)
	} else {
		config.Logger.Info("[PoW] pure Go solver ready")
	}
	chatHistoryStore := chathistory.New(config.ChatHistoryPath())
	if err := chatHistoryStore.Err(); err != nil {
		config.Logger.Warn("[chat_history] unavailable", "path", chatHistoryStore.Path(), "error", err)
	}

	modelsHandler := &shared.ModelsHandler{Store: store}
	chatHandler := &chat.Handler{Store: store, Auth: resolver, DS: dsClient, ChatHistory: chatHistoryStore}
	responsesHandler := &responses.Handler{Store: store, Auth: resolver, DS: dsClient, ChatHistory: chatHistoryStore}
	filesHandler := &files.Handler{Store: store, Auth: resolver, DS: dsClient, ChatHistory: chatHistoryStore}
	embeddingsHandler := &embeddings.Handler{Store: store, Auth: resolver, DS: dsClient, ChatHistory: chatHistoryStore}
	claudeHandler := &claude.Handler{Store: store, Auth: resolver, DS: dsClient, OpenAI: chatHandler, ChatHistory: chatHistoryStore}
	geminiHandler := &gemini.Handler{Store: store, Auth: resolver, DS: dsClient, OpenAI: chatHandler, ChatHistory: chatHistoryStore}
	adminHandler := &admin.Handler{Store: store, Pool: pool, DS: dsClient, OpenAI: chatHandler, ChatHistory: chatHistoryStore}
	ollamaHandler := &ollama.Handler{Store: store}
	webuiHandler := webui.NewHandler()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(filteredLogger())
	r.Use(middleware.Recoverer)
	r.Use(cors)
	r.Use(requestbody.ValidateJSONUTF8)
	r.Use(timeout(0))

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
	r.Get("/v1/models", modelsHandler.ListModels)
	r.Get("/v1/models/{model_id}", modelsHandler.GetModel)
	r.Post("/v1/chat/completions", chatHandler.ChatCompletions)
	r.Post("/v1/responses", responsesHandler.Responses)
	r.Get("/v1/responses/{response_id}", responsesHandler.GetResponseByID)
	r.Post("/v1/files", filesHandler.UploadFile)
	r.Get("/v1/files/{file_id}", filesHandler.RetrieveFile)
	r.Post("/v1/embeddings", embeddingsHandler.Embeddings)
	// Root OpenAI aliases support clients configured with the bare DS2API service URL.
	r.Get("/models", modelsHandler.ListModels)
	r.Get("/models/{model_id}", modelsHandler.GetModel)
	r.Post("/chat/completions", chatHandler.ChatCompletions)
	r.Post("/responses", responsesHandler.Responses)
	r.Get("/responses/{response_id}", responsesHandler.GetResponseByID)
	r.Post("/files", filesHandler.UploadFile)
	r.Get("/files/{file_id}", filesHandler.RetrieveFile)
	r.Post("/embeddings", embeddingsHandler.Embeddings)
	claude.RegisterRoutes(r, claudeHandler)
	gemini.RegisterRoutes(r, geminiHandler)
	ollama.RegisterRoutes(r, ollamaHandler)
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

func filteredLogger() func(http.Handler) http.Handler {
	color := !isWindowsRuntime()
	base := &middleware.DefaultLogFormatter{
		Logger:  log.New(os.Stdout, "", log.LstdFlags),
		NoColor: !color,
	}
	return middleware.RequestLogger(&filteredLogFormatter{base: base})
}

func isWindowsRuntime() bool {
	return runtime.GOOS == "windows"
}

type filteredLogFormatter struct {
	base *middleware.DefaultLogFormatter
}

func (f *filteredLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	if r != nil && r.Method == http.MethodGet {
		path := strings.TrimSpace(r.URL.Path)
		if path == "/admin/chat-history" || strings.HasPrefix(path, "/admin/chat-history/") {
			return noopLogEntry{}
		}
	}
	if r != nil && r.URL != nil {
		if redacted, changed := redactSensitiveQueryParams(r.URL); changed {
			cloned := *r
			clonedURL := *r.URL
			clonedURL.RawQuery = redacted
			cloned.URL = &clonedURL
			cloned.RequestURI = clonedURL.RequestURI()
			return f.base.NewLogEntry(&cloned)
		}
	}
	return f.base.NewLogEntry(r)
}

type noopLogEntry struct{}

func (noopLogEntry) Write(_ int, _ int, _ http.Header, _ time.Duration, _ interface{}) {}

func (noopLogEntry) Panic(_ interface{}, _ []byte) {}

func redactSensitiveQueryParams(u *url.URL) (string, bool) {
	if u == nil || u.RawQuery == "" {
		return "", false
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return redactSensitiveRawQueryParams(u.RawQuery)
	}
	changed := false
	for name, vals := range values {
		if !isSensitiveQueryParam(name) {
			continue
		}
		for i := range vals {
			vals[i] = "REDACTED"
		}
		values[name] = vals
		changed = true
	}
	if !changed {
		return "", false
	}
	return values.Encode(), true
}

func redactSensitiveRawQueryParams(rawQuery string) (string, bool) {
	if rawQuery == "" {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(rawQuery))
	changed := false
	start := 0
	for i := 0; i <= len(rawQuery); i++ {
		if i < len(rawQuery) && rawQuery[i] != '&' && rawQuery[i] != ';' {
			continue
		}
		segment := rawQuery[start:i]
		b.WriteString(redactSensitiveRawQuerySegment(segment, &changed))
		if i < len(rawQuery) {
			b.WriteByte(rawQuery[i])
		}
		start = i + 1
	}
	if !changed {
		return "", false
	}
	return b.String(), true
}

func redactSensitiveRawQuerySegment(segment string, changed *bool) string {
	if segment == "" {
		return segment
	}
	name := segment
	valueStart := -1
	if eq := strings.IndexByte(segment, '='); eq >= 0 {
		name = segment[:eq]
		valueStart = eq + 1
	}
	decodedName, err := url.QueryUnescape(name)
	if err != nil {
		decodedName = name
	}
	if !isSensitiveQueryParam(decodedName) {
		return segment
	}
	if changed != nil {
		*changed = true
	}
	if valueStart < 0 {
		return name + "=REDACTED"
	}
	return segment[:valueStart] + "REDACTED"
}

func isSensitiveQueryParam(name string) bool {
	return strings.EqualFold(name, "key") || strings.EqualFold(name, "api_key")
}

var defaultCORSAllowHeaders = []string{
	"Content-Type",
	"Authorization",
	"X-API-Key",
	"X-Ds2-Target-Account",
	"X-Ds2-Source",
	"X-Vercel-Protection-Bypass",
	"X-Goog-Api-Key",
	"Anthropic-Version",
	"Anthropic-Beta",
}

var blockedCORSRequestHeaders = map[string]struct{}{
	"x-ds2-internal-token": {},
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		addVaryHeaderToken(w.Header(), "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", buildCORSAllowHeaders(r))
	w.Header().Set("Access-Control-Max-Age", "600")
	addVaryHeaderToken(w.Header(), "Access-Control-Request-Headers")
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Access-Control-Request-Private-Network")), "true") {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		addVaryHeaderToken(w.Header(), "Access-Control-Request-Private-Network")
	}
}

func buildCORSAllowHeaders(r *http.Request) string {
	names := make([]string, 0, len(defaultCORSAllowHeaders)+4)
	seen := make(map[string]struct{}, len(defaultCORSAllowHeaders)+4)
	for _, name := range defaultCORSAllowHeaders {
		appendCORSHeaderName(&names, seen, name)
	}
	if r == nil {
		return strings.Join(names, ", ")
	}
	for _, name := range splitCORSRequestHeaders(r.Header.Get("Access-Control-Request-Headers")) {
		appendCORSHeaderName(&names, seen, name)
	}
	return strings.Join(names, ", ")
}

func splitCORSRequestHeaders(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if !isValidCORSHeaderToken(name) {
			continue
		}
		if _, blocked := blockedCORSRequestHeaders[strings.ToLower(name)]; blocked {
			continue
		}
		out = append(out, name)
	}
	return out
}

func appendCORSHeaderName(dst *[]string, seen map[string]struct{}, name string) {
	name = strings.TrimSpace(name)
	if !isValidCORSHeaderToken(name) {
		return
	}
	key := strings.ToLower(name)
	if _, blocked := blockedCORSRequestHeaders[key]; blocked {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*dst = append(*dst, name)
}

func isValidCORSHeaderToken(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		switch c {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func addVaryHeaderToken(h http.Header, token string) {
	if h == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	current := h.Values("Vary")
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(current)+1)
	for _, value := range current {
		for _, part := range strings.Split(value, ",") {
			name := strings.TrimSpace(part)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, name)
		}
	}
	key := strings.ToLower(token)
	if _, ok := seen[key]; !ok {
		merged = append(merged, token)
	}
	h.Set("Vary", strings.Join(merged, ", "))
}

func WriteUnhandledError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "api_error", "message": "Internal Server Error", "detail": err.Error()}})
}
