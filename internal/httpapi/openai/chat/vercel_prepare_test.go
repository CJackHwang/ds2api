package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

func TestIsVercelStreamPrepareRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?__stream_prepare=1", nil)
	if !isVercelStreamPrepareRequest(req) {
		t.Fatalf("expected prepare request to be detected")
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if isVercelStreamPrepareRequest(req2) {
		t.Fatalf("expected non-prepare request")
	}
}

func TestIsVercelStreamReleaseRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?__stream_release=1", nil)
	if !isVercelStreamReleaseRequest(req) {
		t.Fatalf("expected release request to be detected")
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if isVercelStreamReleaseRequest(req2) {
		t.Fatalf("expected non-release request")
	}
}

func TestVercelInternalSecret(t *testing.T) {
	t.Run("prefer explicit secret", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")
		t.Setenv("DS2API_ADMIN_KEY", "admin-fallback")
		if got := vercelInternalSecret(); got != "stream-secret" {
			t.Fatalf("expected explicit secret, got %q", got)
		}
	})

	t.Run("fallback to admin key", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "")
		t.Setenv("DS2API_ADMIN_KEY", "admin-fallback")
		if got := vercelInternalSecret(); got != "admin-fallback" {
			t.Fatalf("expected admin key fallback, got %q", got)
		}
	})

	t.Run("default admin when env missing", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "")
		t.Setenv("DS2API_ADMIN_KEY", "")
		if got := vercelInternalSecret(); got != "admin" {
			t.Fatalf("expected default admin fallback, got %q", got)
		}
	})
}

func TestStreamLeaseLifecycle(t *testing.T) {
	h := &Handler{}
	leaseID := h.holdStreamLease(&auth.RequestAuth{UseConfigToken: false}, "test-session-id")
	if leaseID == "" {
		t.Fatalf("expected non-empty lease id")
	}
	if ok, _, _ := h.releaseStreamLease(leaseID); !ok {
		t.Fatalf("expected lease release success")
	}
	if ok, _, _ := h.releaseStreamLease(leaseID); ok {
		t.Fatalf("expected duplicate release to fail")
	}
}

func TestStreamLeaseTTL(t *testing.T) {
	t.Setenv("DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS", "120")
	if got := streamLeaseTTL(); got != 120*time.Second {
		t.Fatalf("expected ttl=120s, got %v", got)
	}
	t.Setenv("DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS", "invalid")
	if got := streamLeaseTTL(); got != 15*time.Minute {
		t.Fatalf("expected default ttl on invalid value, got %v", got)
	}
}

func TestHandleVercelStreamPrepareAppliesCurrentInputFile(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

	ds := &inlineUploadDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusAuthStub{},
		DS:   ds,
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_prepare=1", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	rec := httptest.NewRecorder()

	h.handleVercelStreamPrepare(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 current input upload, got %d", len(ds.uploadCalls))
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	payload, _ := body["payload"].(map[string]any)
	if payload == nil {
		t.Fatalf("expected payload object, got %#v", body["payload"])
	}
	promptText, _ := payload["prompt"].(string)
	if !strings.Contains(promptText, "Continue from the latest state in the attached DS2API_HISTORY.txt context.") {
		t.Fatalf("expected continuation prompt, got %s", promptText)
	}
	if strings.Contains(promptText, "first user turn") || strings.Contains(promptText, "latest user turn") {
		t.Fatalf("expected original turns hidden from prompt, got %s", promptText)
	}
	refIDs, _ := payload["ref_file_ids"].([]any)
	if len(refIDs) == 0 || refIDs[0] != "file-inline-1" {
		t.Fatalf("expected uploaded history file first in ref_file_ids, got %#v", payload["ref_file_ids"])
	}
}

type vercelReleaseAutoDeleteDSStub struct {
	resp             *http.Response
	deleteCallCount  int
	deletedSessionID string
	deletedToken     string
	deleteErr        error
}

func (m *vercelReleaseAutoDeleteDSStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *vercelReleaseAutoDeleteDSStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (m *vercelReleaseAutoDeleteDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m *vercelReleaseAutoDeleteDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return m.resp, nil
}

func (m *vercelReleaseAutoDeleteDSStub) DeleteSessionForToken(_ context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	m.deleteCallCount++
	m.deletedSessionID = sessionID
	m.deletedToken = token
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (m *vercelReleaseAutoDeleteDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

type vercelReleaseAuthStub struct{}

func (a *vercelReleaseAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, nil
}

func (a *vercelReleaseAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, nil
}

func (a *vercelReleaseAuthStub) Release(_ *auth.RequestAuth) {}

func TestHandleVercelStreamReleaseTriggersAutoDelete(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

	ds := &vercelReleaseAutoDeleteDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			autoDeleteMode: "single",
		},
		Auth: &vercelReleaseAuthStub{},
		DS:   ds,
	}

	leaseID := h.holdStreamLease(&auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, "session-to-delete")
	if leaseID == "" {
		t.Fatalf("expected non-empty lease id")
	}

	reqBody := map[string]any{"lease_id": leaseID}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_release=1", strings.NewReader(string(reqJSON)))
	req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.handleVercelStreamRelease(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.deleteCallCount != 1 {
		t.Fatalf("expected auto delete call count=1, got %d", ds.deleteCallCount)
	}
	if ds.deletedSessionID != "session-to-delete" {
		t.Fatalf("expected deleted session id=session-to-delete, got %q", ds.deletedSessionID)
	}
}

func TestHandleVercelStreamPrepareMapsCurrentInputFileManagedAuthFailureTo401(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

	ds := &inlineUploadDSStub{
		uploadErr: &dsclient.RequestFailure{Op: "upload file", Kind: dsclient.FailureManagedUnauthorized, Message: "expired token"},
	}
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: streamStatusManagedAuthStub{},
		DS:   ds,
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": historySplitTestMessages(),
		"stream":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_prepare=1", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer managed-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	rec := httptest.NewRecorder()

	h.handleVercelStreamPrepare(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Please re-login the account in admin") {
		t.Fatalf("expected managed auth error message, got %s", rec.Body.String())
	}
}
