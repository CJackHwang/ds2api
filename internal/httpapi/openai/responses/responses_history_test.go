package responses

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	dsclient "ds2api/internal/deepseek/client"
)

type responsesHistoryDS struct {
	payload map[string]any
}

func (d *responsesHistoryDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	return "session-id", nil
}

func (d *responsesHistoryDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "pow", nil
}

func (d *responsesHistoryDS) UploadFile(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id"}, nil
}

func (d *responsesHistoryDS) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	d.payload = payload
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"p\":\"response/content\",\"v\":\"ok\"}\n")),
	}, nil
}

func (d *responsesHistoryDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (d *responsesHistoryDS) DeleteAllSessionsForToken(context.Context, string) error {
	return nil
}

func TestResponsesRecordsResponseHistory(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	historyStore := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	ds := &responsesHistoryDS{}
	h := &Handler{
		Store:       store,
		Auth:        resolver,
		DS:          ds,
		ChatHistory: historyStore,
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-v4-flash","input":"hello responses"}`))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.payload == nil {
		t.Fatalf("expected upstream payload to be sent")
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot history: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	item, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("get history item: %v", err)
	}
	if item.Surface != "openai.responses" {
		t.Fatalf("unexpected surface: %q", item.Surface)
	}
	if len(strings.TrimSpace(item.UserInput)) == 0 {
		t.Fatalf("unexpected empty user input")
	}
	if strings.Contains(item.UserInput, "hello responses") {
		t.Fatalf("expected original input to be replaced in user input, got %q", item.UserInput)
	}
	if !strings.Contains(item.HistoryText, "hello responses") {
		t.Fatalf("expected original input in persisted history text, got %q", item.HistoryText)
	}
	if item.Content != "ok" {
		t.Fatalf("expected raw upstream content, got %q", item.Content)
	}
}
