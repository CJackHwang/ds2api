package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/deepseek"
)

type autoDeleteModeDSStub struct {
	resp          *http.Response
	singleCalls   int
	allCalls      int
	lastSessionID string
	lastCtxErr    error
}

func (m *autoDeleteModeDSStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *autoDeleteModeDSStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (m *autoDeleteModeDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ deepseek.UploadFileRequest, _ int) (*deepseek.UploadFileResult, error) {
	return &deepseek.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m *autoDeleteModeDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return m.resp, nil
}

func (m *autoDeleteModeDSStub) DeleteSessionForToken(_ context.Context, _ string, sessionID string) (*deepseek.DeleteSessionResult, error) {
	m.singleCalls++
	m.lastSessionID = sessionID
	return &deepseek.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (m *autoDeleteModeDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	m.allCalls++
	return nil
}

func (m *autoDeleteModeDSStub) DeleteSessionForTokenCtx(ctx context.Context, _ string, sessionID string) (*deepseek.DeleteSessionResult, error) {
	m.singleCalls++
	m.lastSessionID = sessionID
	m.lastCtxErr = ctx.Err()
	return &deepseek.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func TestChatCompletionsAutoDeleteModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantSingle int
		wantAll    int
	}{
		{name: "none", mode: "none"},
		{name: "single", mode: "single", wantSingle: 1},
		{name: "all", mode: "all", wantAll: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &autoDeleteModeDSStub{
				resp: makeOpenAISSEHTTPResponse(
					`data: {"p":"response/content","v":"hello"}`,
					"data: [DONE]",
				),
			}
			h := &Handler{
				Store: mockOpenAIConfig{
					wideInput:      true,
					autoDeleteMode: tc.mode,
				},
				Auth: streamStatusAuthStub{},
				DS:   ds,
			}

			reqBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":false}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
			req.Header.Set("Authorization", "Bearer direct-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.ChatCompletions(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if ds.singleCalls != tc.wantSingle {
				t.Fatalf("single delete calls=%d want=%d", ds.singleCalls, tc.wantSingle)
			}
			if ds.allCalls != tc.wantAll {
				t.Fatalf("all delete calls=%d want=%d", ds.allCalls, tc.wantAll)
			}
			if tc.wantSingle > 0 && ds.lastSessionID != "session-id" {
				t.Fatalf("expected single delete for session-id, got %q", ds.lastSessionID)
			}
		})
	}
}

type autoDeleteCtxDSStub struct {
	autoDeleteModeDSStub
}

func (m *autoDeleteCtxDSStub) DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*deepseek.DeleteSessionResult, error) {
	return m.DeleteSessionForTokenCtx(ctx, token, sessionID)
}

func (m *autoDeleteCtxDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	m.allCalls++
	return nil
}

func TestAutoDeleteRemoteSessionIgnoresCanceledParentContext(t *testing.T) {
	ds := &autoDeleteCtxDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			wideInput:      true,
			autoDeleteMode: "single",
		},
		DS: ds,
	}
	a := &auth.RequestAuth{DeepSeekToken: "token", AccountID: "acct"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h.autoDeleteRemoteSession(ctx, a, "session-id")

	if ds.singleCalls != 1 {
		t.Fatalf("single delete calls=%d want=1", ds.singleCalls)
	}
	if ds.lastCtxErr != nil {
		t.Fatalf("delete ctx should not inherit cancellation, got %v", ds.lastCtxErr)
	}
}

func TestAutoDeleteRemoteSessionSkipsDuplicateBulkDeleteWhenLeaseHeld(t *testing.T) {
	ds := &autoDeleteModeDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			wideInput:          true,
			autoDeleteMode:     "all",
			autoDeleteLeaseSet: true,
			autoDeleteLease:    false,
		},
		DS: ds,
	}

	h.autoDeleteRemoteSession(context.Background(), &auth.RequestAuth{
		DeepSeekToken: "token",
		AccountID:     "acct",
	}, "session-id")

	if ds.allCalls != 0 {
		t.Fatalf("all delete calls=%d want=0", ds.allCalls)
	}
}
