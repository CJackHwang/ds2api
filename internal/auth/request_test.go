package auth

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

func newTestResolver(t *testing.T) *Resolver {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"account-token"}]
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	return NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "fresh-token", nil
	})
}

func TestDetermineWithXAPIKeyUsesDirectToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/anthropic/v1/messages", nil)
	req.Header.Set("x-api-key", "direct-token")

	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if auth.UseConfigToken {
		t.Fatalf("expected direct token mode")
	}
	if auth.DeepSeekToken != "direct-token" {
		t.Fatalf("unexpected token: %q", auth.DeepSeekToken)
	}
	if auth.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
}

func TestDetermineWithXAPIKeyManagedKeyAcquiresAccount(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/anthropic/v1/messages", nil)
	req.Header.Set("x-api-key", "managed-key")

	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	defer r.Release(auth)
	if !auth.UseConfigToken {
		t.Fatalf("expected managed key mode")
	}
	if auth.AccountID != "acc@example.com" {
		t.Fatalf("unexpected account id: %q", auth.AccountID)
	}
	if auth.DeepSeekToken != "fresh-token" {
		t.Fatalf("unexpected account token: %q", auth.DeepSeekToken)
	}
	if auth.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
}

func TestDetermineCallerWithManagedKeySkipsAccountAcquire(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodGet, "/v1/responses/resp_1", nil)
	req.Header.Set("x-api-key", "managed-key")

	a, err := r.DetermineCaller(req)
	if err != nil {
		t.Fatalf("determine caller failed: %v", err)
	}
	if a.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
	if a.UseConfigToken {
		t.Fatalf("expected no config-token lease for caller-only auth")
	}
	if a.AccountID != "" {
		t.Fatalf("expected empty account id, got %q", a.AccountID)
	}
}

func TestCallerTokenIDStable(t *testing.T) {
	a := callerTokenID("token-a")
	b := callerTokenID("token-a")
	c := callerTokenID("token-b")
	if a == "" || b == "" || c == "" {
		t.Fatalf("expected non-empty caller ids")
	}
	if a != b {
		t.Fatalf("expected stable caller id, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different caller id for different tokens")
	}
}

func TestDetermineMissingToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := r.Determine(req)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if err != ErrUnauthorized {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetermineWithQueryKeyUsesDirectToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=direct-query-key", nil)

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a.UseConfigToken {
		t.Fatalf("expected direct token mode")
	}
	if a.DeepSeekToken != "direct-query-key" {
		t.Fatalf("unexpected token: %q", a.DeepSeekToken)
	}
}

func TestDetermineWithXGoogAPIKeyUsesDirectToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", nil)
	req.Header.Set("x-goog-api-key", "goog-header-key")

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a.UseConfigToken {
		t.Fatalf("expected direct token mode")
	}
	if a.DeepSeekToken != "goog-header-key" {
		t.Fatalf("unexpected token: %q", a.DeepSeekToken)
	}
}

func TestDetermineWithAPIKeyQueryParamUsesDirectToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?api_key=direct-api-key", nil)

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a.UseConfigToken {
		t.Fatalf("expected direct token mode")
	}
	if a.DeepSeekToken != "direct-api-key" {
		t.Fatalf("unexpected token: %q", a.DeepSeekToken)
	}
}

func TestDetermineHeaderTokenPrecedenceOverQueryKey(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=query-key", nil)
	req.Header.Set("x-api-key", "managed-key")

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	defer r.Release(a)
	if !a.UseConfigToken {
		t.Fatalf("expected managed key mode from header token")
	}
	if a.AccountID == "" {
		t.Fatalf("expected managed account to be acquired")
	}
}

func TestDetermineCallerMissingToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodGet, "/v1/responses/resp_1", nil)

	_, err := r.DetermineCaller(req)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if err != ErrUnauthorized {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetermineManagedAccountForcesRefreshEverySixHours(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"seed-token"}]
	}`)
	store := config.LoadStore()
	if err := store.UpdateAccountToken("acc@example.com", "seed-token"); err != nil {
		t.Fatalf("update token failed: %v", err)
	}
	pool := account.NewPool(store)

	var loginCount int32
	resolver := NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		n := atomic.AddInt32(&loginCount, 1)
		return "fresh-token-" + string(rune('0'+n)), nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "managed-key")

	a1, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a1.DeepSeekToken != "seed-token" {
		t.Fatalf("expected initial token without forced refresh, got %q", a1.DeepSeekToken)
	}
	resolver.Release(a1)
	if got := atomic.LoadInt32(&loginCount); got != 0 {
		t.Fatalf("expected no login before refresh interval, got %d", got)
	}

	resolver.mu.Lock()
	resolver.tokenRefreshedAt["acc@example.com"] = time.Now().Add(-7 * time.Hour)
	resolver.mu.Unlock()

	a2, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine after interval failed: %v", err)
	}
	defer resolver.Release(a2)
	if a2.DeepSeekToken != "fresh-token-1" {
		t.Fatalf("expected refreshed token after interval, got %q", a2.DeepSeekToken)
	}
	if got := atomic.LoadInt32(&loginCount); got != 1 {
		t.Fatalf("expected exactly one forced refresh login, got %d", got)
	}
}

func TestDetermineManagedAccountUsesUpdatedRefreshInterval(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"seed-token"}],
		"runtime":{"token_refresh_interval_hours":6}
	}`)
	store := config.LoadStore()
	if err := store.UpdateAccountToken("acc@example.com", "seed-token"); err != nil {
		t.Fatalf("update token failed: %v", err)
	}
	pool := account.NewPool(store)

	var loginCount int32
	resolver := NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		n := atomic.AddInt32(&loginCount, 1)
		return "fresh-token-" + string(rune('0'+n)), nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "managed-key")

	a1, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a1.DeepSeekToken != "seed-token" {
		t.Fatalf("expected initial token without forced refresh, got %q", a1.DeepSeekToken)
	}
	resolver.Release(a1)
	if got := atomic.LoadInt32(&loginCount); got != 0 {
		t.Fatalf("expected no login before runtime update, got %d", got)
	}

	if err := store.Update(func(c *config.Config) error {
		c.Runtime.TokenRefreshIntervalHours = 1
		return nil
	}); err != nil {
		t.Fatalf("update runtime failed: %v", err)
	}

	resolver.mu.Lock()
	resolver.tokenRefreshedAt["acc@example.com"] = time.Now().Add(-2 * time.Hour)
	resolver.mu.Unlock()

	a2, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine after runtime update failed: %v", err)
	}
	defer resolver.Release(a2)
	if a2.DeepSeekToken != "fresh-token-1" {
		t.Fatalf("expected refreshed token after runtime update, got %q", a2.DeepSeekToken)
	}
	if got := atomic.LoadInt32(&loginCount); got != 1 {
		t.Fatalf("expected exactly one login after runtime update, got %d", got)
	}
}

func TestDetermineManagedAccountRetriesOtherAccountOnLoginFailure(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"bad@example.com","password":"pwd"},
			{"email":"good@example.com","password":"pwd","token":"good-token"}
		]
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	resolver := NewResolver(store, pool, func(_ context.Context, acc config.Account) (string, error) {
		if acc.Email == "bad@example.com" {
			return "", errors.New("stale account")
		}
		return "fresh-good-token", nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "managed-key")

	a, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	defer resolver.Release(a)
	if a.AccountID != "good@example.com" {
		t.Fatalf("expected fallback to good account, got %q", a.AccountID)
	}
	if a.DeepSeekToken == "" {
		t.Fatal("expected non-empty token from fallback account")
	}
	if !a.TriedAccounts["bad@example.com"] {
		t.Fatalf("expected bad account to be tracked as tried")
	}
}

func TestDetermineTargetAccountDoesNotFallbackOnLoginFailure(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"bad@example.com","password":"pwd"},
			{"email":"good@example.com","password":"pwd","token":"good-token"}
		]
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	resolver := NewResolver(store, pool, func(_ context.Context, acc config.Account) (string, error) {
		if acc.Email == "bad@example.com" {
			return "", errors.New("stale account")
		}
		return "fresh-good-token", nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "managed-key")
	req.Header.Set("X-Ds2-Target-Account", "bad@example.com")

	_, err := resolver.Determine(req)
	if err == nil {
		t.Fatal("expected determine to fail for broken target account")
	}
}

func TestDetermineManagedAccountReturnsLastEnsureErrorWhenAllFail(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"bad1@example.com","password":"pwd"},
			{"email":"bad2@example.com","password":"pwd"}
		]
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	ensureErr := errors.New("all credentials stale")
	resolver := NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "", ensureErr
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "managed-key")

	_, err := resolver.Determine(req)
	if err == nil {
		t.Fatal("expected determine to fail")
	}
	if !errors.Is(err, ensureErr) {
		t.Fatalf("expected ensure error, got %v", err)
	}
	if errors.Is(err, ErrNoAccount) {
		t.Fatalf("expected auth-style ensure error, got ErrNoAccount")
	}
}

// ─── query-key opt-in toggle (PR4: feat/m1-governance-querykey-toggle) ───

// newTestResolverWithQueryKeyDisabled builds a resolver whose store has
// config.auth.allow_gemini_query_key=false, so Gemini-compatible `?key=` /
// `?api_key=` query parameters are rejected as credential fallbacks.
func newTestResolverWithQueryKeyDisabled(t *testing.T) *Resolver {
	t.Helper()
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"account-token"}],
		"auth":{"allow_gemini_query_key":false}
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	return NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "fresh-token", nil
	})
}

func TestDetermineRejectsQueryKeyWhenDisabledByConfig(t *testing.T) {
	r := newTestResolverWithQueryKeyDisabled(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=direct-query-key", nil)

	_, err := r.Determine(req)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized when query-key disabled, got %v", err)
	}
}

func TestDetermineRejectsAPIKeyQueryParamWhenDisabledByConfig(t *testing.T) {
	r := newTestResolverWithQueryKeyDisabled(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?api_key=direct-api-key", nil)

	_, err := r.Determine(req)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized when api_key disabled, got %v", err)
	}
}

func TestDetermineHeaderStillWorksWhenQueryKeyDisabled(t *testing.T) {
	r := newTestResolverWithQueryKeyDisabled(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=ignored", nil)
	req.Header.Set("x-goog-api-key", "header-key")

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("Determine: %v", err)
	}
	if a.UseConfigToken {
		t.Fatalf("expected direct token mode")
	}
	if a.DeepSeekToken != "header-key" {
		t.Fatalf("expected header credential to win, got %q", a.DeepSeekToken)
	}
}

func TestDetermineQueryKeyEnvOverrideDisablesEvenWhenConfigPermits(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "false")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"account-token"}],
		"auth":{"allow_gemini_query_key":true}
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	r := NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "fresh-token", nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=direct", nil)
	if _, err := r.Determine(req); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized when env disables query key, got %v", err)
	}
}

func TestDetermineQueryKeyEnvOverrideEnablesEvenWhenConfigForbids(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "true")
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[{"email":"acc@example.com","password":"pwd","token":"account-token"}],
		"auth":{"allow_gemini_query_key":false}
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	r := NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "fresh-token", nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=direct", nil)
	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("Determine: %v", err)
	}
	if a.DeepSeekToken != "direct" {
		t.Fatalf("expected query key to be accepted via env override, got %q", a.DeepSeekToken)
	}
}
