package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	openaihistory "ds2api/internal/httpapi/openai/history"
	"ds2api/internal/promptcompat"
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

func TestIsVercelStreamSwitchRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?__stream_switch=1", nil)
	if !isVercelStreamSwitchRequest(req) {
		t.Fatalf("expected switch request to be detected")
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if isVercelStreamSwitchRequest(req2) {
		t.Fatalf("expected non-switch request")
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
	if ok, _, _, _ := h.releaseStreamLease(leaseID); !ok {
		t.Fatalf("expected lease release success")
	}
	if ok, _, _, _ := h.releaseStreamLease(leaseID); ok {
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

func TestHandleVercelStreamPrepareUploadsToolsSeparately(t *testing.T) {
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
		"model": "deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "search docs"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "search docs",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
		"stream": true,
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
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected history and tools uploads, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" || ds.uploadCalls[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("unexpected upload filenames: %#v", ds.uploadCalls)
	}
	if strings.Contains(string(ds.uploadCalls[0].Data), "Description: search docs") {
		t.Fatalf("history transcript should not embed tool descriptions, got %q", string(ds.uploadCalls[0].Data))
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	finalPrompt, _ := body["final_prompt"].(string)
	payload, _ := body["payload"].(map[string]any)
	payloadPrompt, _ := payload["prompt"].(string)
	for label, promptText := range map[string]string{"final_prompt": finalPrompt, "payload.prompt": payloadPrompt} {
		if !strings.Contains(promptText, "DS2API_TOOLS.txt") || !strings.Contains(promptText, "TOOL CALL FORMAT") {
			t.Fatalf("expected %s to reference tools file and retain tool instructions, got %q", label, promptText)
		}
		if strings.Contains(promptText, "Description: search docs") {
			t.Fatalf("expected %s not to inline tool descriptions, got %q", label, promptText)
		}
	}
	refIDs, _ := payload["ref_file_ids"].([]any)
	if len(refIDs) < 2 || refIDs[0] != "file-inline-1" || refIDs[1] != "file-inline-2" {
		t.Fatalf("expected history and tools ref ids first, got %#v", payload["ref_file_ids"])
	}
}

type vercelReleaseAutoDeleteDSStub struct {
	resp             *http.Response
	auth             *vercelReleaseAuthStub
	deleteCallCount  int
	deleteAllCount   int
	deletedSessionID string
	deletedToken     string
	deletedAllToken  string
	releasedAtDelete bool
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
	if m.auth != nil {
		m.releasedAtDelete = m.auth.releaseCallCount > 0
	}
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (m *vercelReleaseAutoDeleteDSStub) DeleteAllSessionsForToken(_ context.Context, token string) error {
	m.deleteAllCount++
	m.deletedAllToken = token
	if m.auth != nil {
		m.releasedAtDelete = m.auth.releaseCallCount > 0
	}
	return nil
}

type vercelReleaseAuthStub struct {
	releaseCallCount int
}

func (a *vercelReleaseAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, nil
}

func (a *vercelReleaseAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, nil
}

func (a *vercelReleaseAuthStub) Release(_ *auth.RequestAuth) {
	a.releaseCallCount++
}

func TestHandleVercelStreamReleaseTriggersAutoDelete(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

	authStub := &vercelReleaseAuthStub{}
	ds := &vercelReleaseAutoDeleteDSStub{auth: authStub}
	h := &Handler{
		Store: mockOpenAIConfig{
			autoDeleteMode: "single",
		},
		Auth: authStub,
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
	if ds.releasedAtDelete {
		t.Fatalf("expected auto-delete before releasing the leased account")
	}
	if authStub.releaseCallCount != 1 {
		t.Fatalf("expected leased account release after auto-delete, got %d", authStub.releaseCallCount)
	}
}

func TestHandleVercelStreamReleaseAutoDeleteAllBeforeRelease(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

	authStub := &vercelReleaseAuthStub{}
	ds := &vercelReleaseAutoDeleteDSStub{auth: authStub}
	h := &Handler{
		Store: mockOpenAIConfig{
			autoDeleteMode: "all",
		},
		Auth: authStub,
		DS:   ds,
	}

	leaseID := h.holdStreamLease(&auth.RequestAuth{DeepSeekToken: "test-token", AccountID: "test-account"}, "session-to-delete")
	reqJSON, _ := json.Marshal(map[string]any{"lease_id": leaseID})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_release=1", strings.NewReader(string(reqJSON)))
	req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.handleVercelStreamRelease(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.deleteAllCount != 1 {
		t.Fatalf("expected delete-all call count=1, got %d", ds.deleteAllCount)
	}
	if ds.deletedAllToken != "test-token" {
		t.Fatalf("expected delete-all token=test-token, got %q", ds.deletedAllToken)
	}
	if ds.releasedAtDelete {
		t.Fatalf("expected delete-all before releasing the leased account")
	}
	if authStub.releaseCallCount != 1 {
		t.Fatalf("expected leased account release after delete-all, got %d", authStub.releaseCallCount)
	}
}

func TestHandleVercelStreamSwitchReuploadsCurrentInputFile(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"acc1@test.com","password":"pwd"},
			{"email":"acc2@test.com","password":"pwd"}
		]
	}`)
	openaihistory.ResetCurrentInputToolsFileCacheForTesting()

	store := config.LoadStore()
	resolver := auth.NewResolver(store, account.NewPool(store), func(_ context.Context, acc config.Account) (string, error) {
		return "token-" + acc.Identifier(), nil
	})
	authReq := httptest.NewRequest(http.MethodPost, "/", nil)
	authReq.Header.Set("Authorization", "Bearer managed-key")
	a, err := resolver.Determine(authReq)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	defer resolver.Release(a)

	ds := &inlineUploadDSStub{createSession: "session-new"}
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth: resolver,
		DS:   ds,
	}
	stdReq := promptcompat.StandardRequest{
		RequestedModel:          "deepseek-v4-flash",
		ResolvedModel:           "deepseek-v4-flash",
		ResponseModel:           "deepseek-v4-flash",
		FinalPrompt:             "Continue from the latest state in the attached DS2API_HISTORY.txt context. Available tool descriptions and parameter schemas are attached in DS2API_TOOLS.txt; use only those tools and follow the tool-call format rules in this prompt.",
		PromptTokenText:         "# DS2API_HISTORY.txt\n\n=== 1. USER ===\nhello\n\n# DS2API_TOOLS.txt\nAvailable tool descriptions and parameter schemas for this request.\n\nYou have access to these tools:\n\nTool: search\nDescription: search docs\nParameters: {\"type\":\"object\"}\n",
		HistoryText:             "# DS2API_HISTORY.txt\n\n=== 1. USER ===\nhello\n",
		CurrentInputFileApplied: true,
		CurrentInputFileID:      "file-old",
		CurrentToolsFileID:      "file-old-tools",
		ToolsRaw: []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": "search docs",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
		RefFileIDs: []string{"file-old", "file-old-tools", "client-file"},
		Thinking:   true,
	}
	leaseID := h.holdStreamLease(a, "session-old", stdReq)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_switch=1", strings.NewReader(`{"lease_id":"`+leaseID+`"}`))
	req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	rec := httptest.NewRecorder()

	h.handleVercelStreamSwitch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 2 {
		t.Fatalf("expected current input and tools reupload on switched account, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "DS2API_HISTORY.txt" || ds.uploadCalls[1].Filename != "DS2API_TOOLS.txt" {
		t.Fatalf("unexpected reupload filenames: %#v", ds.uploadCalls)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body["deepseek_token"] != "token-acc2@test.com" {
		t.Fatalf("expected switched account token, got %#v", body["deepseek_token"])
	}
	if body["session_id"] != "session-new" {
		t.Fatalf("expected switched session id, got %#v", body["session_id"])
	}
	payload, _ := body["payload"].(map[string]any)
	refIDs, _ := payload["ref_file_ids"].([]any)
	if len(refIDs) != 3 || refIDs[0] != "file-inline-1" || refIDs[1] != "file-inline-2" || refIDs[2] != "client-file" {
		t.Fatalf("expected reuploaded current input refs plus client ref, got %#v", payload["ref_file_ids"])
	}
	promptText, _ := payload["prompt"].(string)
	if !strings.Contains(promptText, "DS2API_TOOLS.txt") {
		t.Fatalf("expected switched payload prompt to retain tools file reference, got %q", promptText)
	}
	ok, _, releasedSessionID, pendingCleanups := h.releaseStreamLease(leaseID)
	if !ok {
		t.Fatalf("expected lease to remain releasable after switch")
	}
	if releasedSessionID != "session-new" {
		t.Fatalf("expected release to use switched session, got %q", releasedSessionID)
	}
	if len(pendingCleanups) != 1 || pendingCleanups[0].SessionID != "session-old" || pendingCleanups[0].DeepSeekToken != "token-acc1@test.com" {
		t.Fatalf("expected old session cleanup to be retained, got %#v", pendingCleanups)
	}
}

func TestHandleVercelStreamSwitchReleaseCleansOldSessionAfterReuploadFailure(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"acc1@test.com","password":"pwd"},
			{"email":"acc2@test.com","password":"pwd"}
		]
	}`)
	openaihistory.ResetCurrentInputToolsFileCacheForTesting()

	store := config.LoadStore()
	resolver := auth.NewResolver(store, account.NewPool(store), func(_ context.Context, acc config.Account) (string, error) {
		return "token-" + acc.Identifier(), nil
	})
	authReq := httptest.NewRequest(http.MethodPost, "/", nil)
	authReq.Header.Set("Authorization", "Bearer managed-key")
	a, err := resolver.Determine(authReq)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}

	ds := &inlineUploadDSStub{uploadErr: errors.New("upload failed")}
	h := &Handler{
		Store: mockOpenAIConfig{
			autoDeleteMode:      "single",
			currentInputEnabled: true,
		},
		Auth: resolver,
		DS:   ds,
	}
	stdReq := promptcompat.StandardRequest{
		RequestedModel:          "deepseek-v4-flash",
		ResolvedModel:           "deepseek-v4-flash",
		ResponseModel:           "deepseek-v4-flash",
		FinalPrompt:             "Continue from the latest state in the attached DS2API_HISTORY.txt context.",
		HistoryText:             "# DS2API_HISTORY.txt\n\n=== 1. USER ===\nhello\n",
		CurrentInputFileApplied: true,
		CurrentInputFileID:      "file-old",
		RefFileIDs:              []string{"file-old"},
	}
	leaseID := h.holdStreamLease(a, "session-old", stdReq)
	switchReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_switch=1", strings.NewReader(`{"lease_id":"`+leaseID+`"}`))
	switchReq.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	switchRec := httptest.NewRecorder()

	h.handleVercelStreamSwitch(switchRec, switchReq)

	if switchRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected switch failure, got %d body=%s", switchRec.Code, switchRec.Body.String())
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_release=1", strings.NewReader(`{"lease_id":"`+leaseID+`"}`))
	releaseReq.Header.Set("X-Ds2-Internal-Token", "stream-secret")
	releaseRec := httptest.NewRecorder()

	h.handleVercelStreamRelease(releaseRec, releaseReq)

	if releaseRec.Code != http.StatusOK {
		t.Fatalf("expected release 200, got %d body=%s", releaseRec.Code, releaseRec.Body.String())
	}
	if ds.deleteCallCount != 1 {
		t.Fatalf("expected old session cleanup, got delete count %d", ds.deleteCallCount)
	}
	if ds.deletedSessionID != "session-old" {
		t.Fatalf("expected old session to be deleted, got %q", ds.deletedSessionID)
	}
	if ds.deletedToken != "token-acc1@test.com" {
		t.Fatalf("expected old account token for cleanup, got %q", ds.deletedToken)
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
