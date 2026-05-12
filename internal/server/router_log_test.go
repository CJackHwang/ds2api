package server

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestFilteredLogFormatterRedactsSensitiveQueryParams(t *testing.T) {
	var buf bytes.Buffer
	formatter := &filteredLogFormatter{
		base: &middleware.DefaultLogFormatter{
			Logger:  log.New(&buf, "", 0),
			NoColor: true,
		},
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1beta/models/gemini-2.5-pro:generateContent?key=caller-secret&api_key=second-secret&alt=sse",
		nil,
	)

	entry := formatter.NewLogEntry(req)
	entry.Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)

	got := buf.String()
	for _, secret := range []string{"caller-secret", "second-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log line contains sensitive query value %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "key=REDACTED") || !strings.Contains(got, "api_key=REDACTED") {
		t.Fatalf("log line did not include redacted sensitive params: %s", got)
	}
	if !strings.Contains(got, "alt=sse") {
		t.Fatalf("log line did not preserve non-sensitive query param: %s", got)
	}
	if req.URL.RawQuery != "key=caller-secret&api_key=second-secret&alt=sse" {
		t.Fatalf("request was mutated, RawQuery = %q", req.URL.RawQuery)
	}
}

func TestFilteredLogFormatterRedactsSensitiveQueryParamsWhenMalformed(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		secrets   []string
		redacted  []string
		preserved []string
	}{
		{
			name:      "semicolon separator",
			target:    "/v1beta/models/gemini-2.5-pro:generateContent?key=caller-secret;alt=sse",
			secrets:   []string{"caller-secret"},
			redacted:  []string{"key=REDACTED"},
			preserved: []string{"alt=sse"},
		},
		{
			name:     "bad escape in sensitive value",
			target:   "/v1beta/models/gemini-2.5-pro:generateContent?api_key=second-secret%ZZ",
			secrets:  []string{"second-secret"},
			redacted: []string{"api_key=REDACTED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatter := &filteredLogFormatter{
				base: &middleware.DefaultLogFormatter{
					Logger:  log.New(&buf, "", 0),
					NoColor: true,
				},
			}
			req := httptest.NewRequest(http.MethodPost, tt.target, nil)

			entry := formatter.NewLogEntry(req)
			entry.Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)

			got := buf.String()
			for _, secret := range tt.secrets {
				if strings.Contains(got, secret) {
					t.Fatalf("log line contains sensitive query value %q: %s", secret, got)
				}
			}
			for _, want := range tt.redacted {
				if !strings.Contains(got, want) {
					t.Fatalf("log line missing redacted query %q: %s", want, got)
				}
			}
			for _, want := range tt.preserved {
				if !strings.Contains(got, want) {
					t.Fatalf("log line missing preserved query %q: %s", want, got)
				}
			}
		})
	}
}
