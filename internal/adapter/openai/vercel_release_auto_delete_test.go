package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
)

type vercelReleaseAuthStub struct {
	released int
}

func (s *vercelReleaseAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return nil, auth.ErrNoAccount
}

func (s *vercelReleaseAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return nil, auth.ErrNoAccount
}

func (s *vercelReleaseAuthStub) Release(_ *auth.RequestAuth) {
	s.released++
}

func TestVercelReleaseAppliesAutoDeleteMode(t *testing.T) {
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
			t.Setenv("VERCEL", "1")
			t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")

			ds := &autoDeleteModeDSStub{}
			authStub := &vercelReleaseAuthStub{}
			h := &Handler{
				Store: mockOpenAIConfig{autoDeleteMode: tc.mode},
				Auth:  authStub,
				DS:    ds,
			}

			leaseID := h.holdStreamLease(&auth.RequestAuth{DeepSeekToken: "token", AccountID: "acct"}, "session-id")
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_release=1", strings.NewReader(`{"lease_id":"`+leaseID+`"}`))
			req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
			rec := httptest.NewRecorder()

			h.handleVercelStreamRelease(rec, req)

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
			if authStub.released != 1 {
				t.Fatalf("expected auth release once, got %d", authStub.released)
			}
		})
	}
}

