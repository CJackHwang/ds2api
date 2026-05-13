package ollama

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type ollamaTestSurface struct {
	Store   ConfigReader
	handler *Handler
}

func (h *ollamaTestSurface) apiHandler() *Handler {
	if h.handler == nil {
		h.handler = &Handler{Store: h.Store}
	}
	return h.handler
}

func registerOllamaTestRoutes(r chi.Router, h *ollamaTestSurface) {
	r.Get("/api/version", h.apiHandler().GetVersion)
	r.Get("/api/tags", h.apiHandler().ListOllamaModels)
	r.Post("/api/show", h.apiHandler().GetOllamaModel)
}

func TestGetOllamaVersionRoute(t *testing.T) {
	h := &ollamaTestSurface{}
	r := chi.NewRouter()
	registerOllamaTestRoutes(r, h)
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetOllamaModelsRoute(t *testing.T) {
	h := &ollamaTestSurface{}
	r := chi.NewRouter()
	registerOllamaTestRoutes(r, h)
	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetOllamaModelRoute(t *testing.T) {
	h := &ollamaTestSurface{}
	r := chi.NewRouter()
	registerOllamaTestRoutes(r, h)

	t.Run("direct", func(t *testing.T) {
		body := `{"model":"deepseek-v4-flash"}`
		req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("expected valid json body, got err=%v body=%s", err, rec.Body.String())
		}
		if _, ok := payload["id"]; !ok {
			t.Fatalf("expected response has lowercase id field, body=%s", rec.Body.String())
		}
		if _, ok := payload["ID"]; ok {
			t.Fatalf("expected response does not expose uppercase ID field, body=%s", rec.Body.String())
		}
	})

	t.Run("direct_nothinking", func(t *testing.T) {
		body := `{"model":"deepseek-v4-flash-nothinking"}`
		req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("direct_expert", func(t *testing.T) {
		body := `{"model":"deepseek-v4-pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("direct_vision", func(t *testing.T) {
		body := `{"model":"deepseek-v4-vision"}`
		req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestGetOllamaModelRouteNotFound(t *testing.T) {
	h := &ollamaTestSurface{}
	r := chi.NewRouter()
	registerOllamaTestRoutes(r, h)

	body := `{"model":"not-exists"}`
	req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}
