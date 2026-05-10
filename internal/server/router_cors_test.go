package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSPreflightAllowsThirdPartyRequestedHeaders(t *testing.T) {
	handler := corsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "app://obsidian.md")
	req.Header.Set("Access-Control-Request-Headers", "authorization, x-stainless-os, x-stainless-runtime, x-ds2-internal-token")
	req.Header.Set("Access-Control-Request-Private-Network", "true")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "app://obsidian.md" {
		t.Fatalf("expected origin echo, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("expected private network allow header, got %q", got)
	}

	allowHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, want := range []string{"authorization", "x-stainless-os", "x-stainless-runtime"} {
		if !strings.Contains(allowHeaders, want) {
			t.Fatalf("expected allow headers to include %q, got %q", want, rec.Header().Get("Access-Control-Allow-Headers"))
		}
	}
	if strings.Contains(allowHeaders, "x-ds2-internal-token") {
		t.Fatalf("expected internal-only header to stay blocked, got %q", rec.Header().Get("Access-Control-Allow-Headers"))
	}

	vary := strings.ToLower(rec.Header().Get("Vary"))
	for _, want := range []string{"origin", "access-control-request-headers", "access-control-request-private-network"} {
		if !strings.Contains(vary, want) {
			t.Fatalf("expected vary to include %q, got %q", want, rec.Header().Get("Vary"))
		}
	}
}

func TestCORSAllowlistBlocksUnknownOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	setCORSHeaders(rec, req, []string{"https://trusted.example.com"})

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no ACAO header for blocked origin, got %q", got)
	}
}

func TestCORSAllowlistPermitsKnownOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://trusted.example.com")
	rec := httptest.NewRecorder()

	setCORSHeaders(rec, req, []string{"https://trusted.example.com"})

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://trusted.example.com" {
		t.Fatalf("expected origin reflected, got %q", got)
	}
}

func TestCORSAllowlistBlockedOriginStillSetsVary(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	setCORSHeaders(rec, req, []string{"https://trusted.example.com"})

	if got := rec.Header().Get("Vary"); !strings.Contains(strings.ToLower(got), "origin") {
		t.Fatalf("expected Vary to include Origin for blocked request, got %q", got)
	}
}

func TestCORSEmptyAllowlistEchoesOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://any.example.com")
	rec := httptest.NewRecorder()

	setCORSHeaders(rec, req, nil)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://any.example.com" {
		t.Fatalf("expected origin echoed when no allowlist, got %q", got)
	}
}

func TestBuildCORSAllowHeadersKeepsDefaultsWithoutRequest(t *testing.T) {
	got := strings.ToLower(buildCORSAllowHeaders(nil))
	for _, want := range []string{"content-type", "x-goog-api-key", "anthropic-version", "x-ds2-source"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected default allow headers to include %q, got %q", want, got)
		}
	}
}

func TestAppCORSPreflightIsUnifiedAcrossInterfaces(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"],"accounts":[{"email":"u@example.com","password":"p"}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error: %v", err)
	}

	cases := []struct {
		name    string
		path    string
		headers string
	}{
		{
			name:    "openai",
			path:    "/v1/chat/completions",
			headers: "authorization, x-stainless-os",
		},
		{
			name:    "claude",
			path:    "/anthropic/v1/messages",
			headers: "x-api-key, anthropic-version, x-stainless-os",
		},
		{
			name:    "gemini",
			path:    "/v1beta/models/gemini-2.5-pro:generateContent",
			headers: "x-goog-api-key, x-client-version",
		},
		{
			name:    "admin",
			path:    "/admin/login",
			headers: "content-type, x-requested-with",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, tc.path, nil)
			req.Header.Set("Origin", "app://obsidian.md")
			req.Header.Set("Access-Control-Request-Headers", tc.headers)

			rec := httptest.NewRecorder()
			app.Router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected %s preflight status 204, got %d", tc.path, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "app://obsidian.md" {
				t.Fatalf("expected origin echo for %s, got %q", tc.path, got)
			}
			allowHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
			for _, want := range splitCORSRequestHeaders(tc.headers) {
				if !strings.Contains(allowHeaders, strings.ToLower(want)) {
					t.Fatalf("expected allow headers for %s to include %q, got %q", tc.path, want, rec.Header().Get("Access-Control-Allow-Headers"))
				}
			}
		})
	}
}
