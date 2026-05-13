package completionruntime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/httpapi/openai/shared"
)

func TestExecuteStreamWithRetryUsesSharedRetryPayloadAndUsagePrompt(t *testing.T) {
	ds := &fakeDeepSeekCaller{responses: []*http.Response{
		sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"ok"}`),
	}}
	initial := sseHTTPResponse(http.StatusOK, `data: {"response_message_id":77,"p":"response/thinking_content","v":"plan"}`)
	payload := map[string]any{"prompt": "original prompt"}
	attemptsSeen := 0
	retryPrompt := ""

	ExecuteStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, initial, payload, "pow", StreamRetryOptions{
		Surface:      "test.stream",
		Stream:       true,
		RetryEnabled: true,
		UsagePrompt:  "original prompt",
	}, StreamRetryHooks{
		ConsumeAttempt: func(resp *http.Response, allowDeferEmpty bool) (bool, bool) {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Fatalf("close failed: %v", err)
				}
			}()
			_, _ = io.ReadAll(resp.Body)
			attemptsSeen++
			return attemptsSeen == 2, attemptsSeen == 1 && allowDeferEmpty
		},
		ParentMessageID: func() int {
			return 77
		},
		OnRetryPrompt: func(prompt string) {
			retryPrompt = prompt
		},
	})

	if attemptsSeen != 2 {
		t.Fatalf("expected two stream attempts, got %d", attemptsSeen)
	}
	if len(ds.payloads) != 1 {
		t.Fatalf("expected one retry completion call, got %d", len(ds.payloads))
	}
	if got := ds.payloads[0]["parent_message_id"]; got != 77 {
		t.Fatalf("retry parent_message_id mismatch: %#v", got)
	}
	if prompt, _ := ds.payloads[0]["prompt"].(string); !strings.Contains(prompt, shared.EmptyOutputRetrySuffix) {
		t.Fatalf("expected retry suffix in payload prompt, got %q", prompt)
	}
	if !strings.Contains(retryPrompt, shared.EmptyOutputRetrySuffix) {
		t.Fatalf("expected retry suffix in usage prompt, got %q", retryPrompt)
	}
}

func TestExecuteStreamWithRetrySwitchesManagedAccountBeforeFinal429(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"acc1@test.com","password":"pwd"},
			{"email":"acc2@test.com","password":"pwd"}
		]
	}`)
	store := config.LoadStore()
	resolver := auth.NewResolver(store, account.NewPool(store), func(_ context.Context, acc config.Account) (string, error) {
		return "token-" + acc.Identifier(), nil
	})
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer managed-key")
	a, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	defer resolver.Release(a)

	ds := &fakeDeepSeekCaller{
		sessionByAccount: true,
		responses: []*http.Response{
			sseHTTPResponse(http.StatusOK, `data: {"response_message_id":12,"p":"response/thinking_content","v":"retry empty"}`),
			sseHTTPResponse(http.StatusOK, `data: {"response_message_id":21,"p":"response/content","v":"ok from second account"}`),
		},
	}
	initial := sseHTTPResponse(http.StatusOK, `data: {"response_message_id":11,"p":"response/thinking_content","v":"first empty"}`)
	payload := map[string]any{"prompt": "original prompt", "chat_session_id": "session-acc1@test.com"}
	attemptsSeen := 0
	switchedSession := ""

	ExecuteStreamWithRetry(context.Background(), ds, a, initial, payload, "pow", StreamRetryOptions{
		Surface:          "test.stream",
		Stream:           true,
		RetryEnabled:     true,
		RetryMaxAttempts: 1,
		UsagePrompt:      "original prompt",
	}, StreamRetryHooks{
		ConsumeAttempt: func(resp *http.Response, allowDeferEmpty bool) (bool, bool) {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Fatalf("close failed: %v", err)
				}
			}()
			body, _ := io.ReadAll(resp.Body)
			attemptsSeen++
			if strings.Contains(string(body), "ok from second account") {
				return true, false
			}
			if !allowDeferEmpty {
				t.Fatalf("expected empty attempt %d to be deferred before final 429", attemptsSeen)
			}
			return false, true
		},
		ParentMessageID: func() int {
			return 11 + attemptsSeen
		},
		OnAccountSwitch: func(sessionID string) {
			switchedSession = sessionID
		},
	})

	if attemptsSeen != 3 {
		t.Fatalf("expected three stream attempts, got %d", attemptsSeen)
	}
	if switchedSession != "session-acc2@test.com" {
		t.Fatalf("expected switched session id, got %q", switchedSession)
	}
	wantAccounts := []string{"acc1@test.com", "acc2@test.com"}
	if len(ds.completionAccounts) != len(wantAccounts) {
		t.Fatalf("completion accounts mismatch: got %v want %v", ds.completionAccounts, wantAccounts)
	}
	for i, want := range wantAccounts {
		if ds.completionAccounts[i] != want {
			t.Fatalf("completion account %d = %q want %q (all=%v)", i, ds.completionAccounts[i], want, ds.completionAccounts)
		}
	}
	if got := ds.payloads[1]["chat_session_id"]; got != "session-acc2@test.com" {
		t.Fatalf("switched payload session mismatch: %#v", got)
	}
	if prompt, _ := ds.payloads[1]["prompt"].(string); strings.Contains(prompt, shared.EmptyOutputRetrySuffix) {
		t.Fatalf("expected switched-account prompt without empty-output suffix, got %q", prompt)
	}
}
