package claude

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	dsclient "ds2api/internal/deepseek/client"
)

type claudeCurrentInputAuth struct{}

type claudeHistoryConfig struct {
	aliases map[string]string
}

func (m claudeHistoryConfig) ModelAliases() map[string]string { return m.aliases }
func (claudeHistoryConfig) CurrentInputFileEnabled() bool     { return false }
func (claudeHistoryConfig) CurrentInputFileMinChars() int     { return 0 }

func (claudeCurrentInputAuth) Determine(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{
		DeepSeekToken: "direct-token",
		CallerID:      "caller:test",
		TriedAccounts: map[string]bool{},
	}, nil
}

func TestClaudeDirectRecordsResponseHistory(t *testing.T) {
	ds := &claudeCurrentInputDS{}
	historyStore := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{
		Store:       claudeHistoryConfig{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		Auth:        claudeCurrentInputAuth{},
		DS:          ds,
		ChatHistory: historyStore,
	}
	reqBody := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello from claude"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
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
	if item.Surface != "claude.messages" {
		t.Fatalf("unexpected surface: %q", item.Surface)
	}
	if item.Model != "claude-sonnet-4-6" {
		t.Fatalf("unexpected model: %q", item.Model)
	}
	if item.UserInput != "hello from claude" {
		t.Fatalf("unexpected user input: %q", item.UserInput)
	}
	if item.Content != "ok" {
		t.Fatalf("expected raw upstream content, got %q", item.Content)
	}
}

func (claudeCurrentInputAuth) Release(*auth.RequestAuth) {}

type claudeCurrentInputDS struct {
	uploads []dsclient.UploadFileRequest
	payload map[string]any
}

func (d *claudeCurrentInputDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	return "session-id", nil
}

func (d *claudeCurrentInputDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "pow", nil
}

func (d *claudeCurrentInputDS) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	d.uploads = append(d.uploads, req)
	id := "file-claude-history"
	if len(d.uploads) > 1 {
		id = "file-claude-tools"
	}
	return &dsclient.UploadFileResult{ID: id}, nil
}

func (d *claudeCurrentInputDS) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	d.payload = payload
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"p\":\"response/content\",\"v\":\"ok\"}\n")),
	}, nil
}

func TestClaudeDirectAppliesCurrentInputFile(t *testing.T) {
	ds := &claudeCurrentInputDS{}
	historyStore := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{
		Store:       mockClaudeConfig{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		Auth:        claudeCurrentInputAuth{},
		DS:          ds,
		ChatHistory: historyStore,
	}
	reqBody := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello from claude"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploads) != 1 {
		t.Fatalf("expected one current input upload, got %d", len(ds.uploads))
	}
	if ds.uploads[0].Filename != "DS2API_HISTORY.txt" {
		t.Fatalf("unexpected upload filename: %q", ds.uploads[0].Filename)
	}
	refIDs, _ := ds.payload["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-claude-history" {
		t.Fatalf("expected uploaded history ref id, got %#v", ds.payload["ref_file_ids"])
	}
	prompt, _ := ds.payload["prompt"].(string)
	if !strings.Contains(prompt, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation prompt, got %q", prompt)
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot history: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	full, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("get history item: %v", err)
	}
	if full.HistoryText != string(ds.uploads[0].Data) {
		t.Fatalf("expected uploaded current input file to be persisted in history text")
	}
	if len(full.Messages) != 1 || !strings.Contains(full.Messages[0].Content, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected persisted message to match upstream continuation prompt, got %#v", full.Messages)
	}
}

func TestClaudeCurrentInputFileUploadsToolsSeparately(t *testing.T) {
	ds := &claudeCurrentInputDS{}
	h := &Handler{
		Store: mockClaudeConfig{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"}},
		Auth:  claudeCurrentInputAuth{},
		DS:    ds,
	}
	reqBody := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello from claude"}],"tools":[{"name":"search","description":"Search docs","input_schema":{"type":"object"}}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploads) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploads))
	}
	if ds.uploads[0].Filename != "DS2API_HISTORY.txt" || ds.uploads[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("unexpected upload filenames: %#v", ds.uploads)
	}
	historyText := string(ds.uploads[0].Data)
	if strings.Contains(historyText, "You have access to these tools") || strings.Contains(historyText, "Description: Search docs") {
		t.Fatalf("history transcript should not embed tool descriptions, got %q", historyText)
	}
	toolsText := string(ds.uploads[1].Data)
	if !strings.Contains(toolsText, "# DS2API_TOOLS.txt") || !strings.Contains(toolsText, "Tool: search") || !strings.Contains(toolsText, "Description: Search docs") {
		t.Fatalf("expected tools transcript to include tool schema, got %q", toolsText)
	}
	refIDs, _ := ds.payload["ref_file_ids"].([]any)
	if len(refIDs) < 2 || refIDs[0] != "file-claude-history" || refIDs[1] != "file-claude-tools" {
		t.Fatalf("expected history and tools ref ids first, got %#v", ds.payload["ref_file_ids"])
	}
	prompt, _ := ds.payload["prompt"].(string)
	if !strings.Contains(prompt, "DS2API_TOOLS.txt") || !strings.Contains(prompt, "TOOL CALL FORMAT") {
		t.Fatalf("expected live prompt to reference tools file and retain format instructions, got %q", prompt)
	}
	if strings.Contains(prompt, "Description: Search docs") {
		t.Fatalf("live prompt should not inline tool descriptions, got %q", prompt)
	}
}
