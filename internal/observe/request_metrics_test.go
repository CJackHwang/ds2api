package observe

import (
	"context"
	"testing"
	"time"
)

func TestFromContextReturnsNilWhenMissing(t *testing.T) {
	if m := FromContext(context.Background()); m != nil {
		t.Fatalf("expected nil, got %v", m)
	}
}

func TestWithMetricsRoundTrip(t *testing.T) {
	m := &RequestMetrics{}
	ctx := WithMetrics(context.Background(), m)
	got := FromContext(ctx)
	if got != m {
		t.Fatalf("expected same pointer, got %v", got)
	}
}

func TestSettersPopulateFields(t *testing.T) {
	m := &RequestMetrics{}
	ctx := WithMetrics(context.Background(), m)

	SetRoute(ctx, "/v1/chat/completions")
	SetModel(ctx, "deepseek-chat")
	SetAccount(ctx, "user@example.com")
	SetSurface(ctx, "openai.chat")
	IncrRetry(ctx)
	IncrRetry(ctx)
	IncrAccountSwitch(ctx)

	now := time.Now()
	SetUpstreamResponseAt(ctx, now)
	SetFirstByteAt(ctx, now.Add(10*time.Millisecond))

	// Second set should not overwrite (first-write-wins)
	SetUpstreamResponseAt(ctx, now.Add(100*time.Millisecond))
	SetFirstByteAt(ctx, now.Add(100*time.Millisecond))

	if m.Route != "/v1/chat/completions" {
		t.Fatalf("route=%q", m.Route)
	}
	if m.ModelAlias != "deepseek-chat" {
		t.Fatalf("model=%q", m.ModelAlias)
	}
	if m.AccountID != "user***" {
		t.Fatalf("account_id=%q", m.AccountID)
	}
	if m.Surface != "openai.chat" {
		t.Fatalf("surface=%q", m.Surface)
	}
	if m.RetryCount != 2 {
		t.Fatalf("retry_count=%d", m.RetryCount)
	}
	if m.AccountSwitchCount != 1 {
		t.Fatalf("account_switch_count=%d", m.AccountSwitchCount)
	}
	if !m.UpstreamResponseAt.Equal(now) {
		t.Fatalf("upstream_response_at should be first set value")
	}
	if !m.FirstByteAt.Equal(now.Add(10 * time.Millisecond)) {
		t.Fatalf("first_byte_at should be first set value")
	}
}

func TestRedactAccountID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"user@example.com", "user***"},
		{"ab", "***"},
		{"", "***"},
		{"abcdefgh", "abcd***"},
	}
	for _, tc := range cases {
		got := redactAccountID(tc.in)
		if got != tc.want {
			t.Errorf("redactAccountID(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSettersNoopOnNilContext(t *testing.T) {
	// Should not panic when context has no metrics.
	ctx := context.Background()
	SetRoute(ctx, "/test")
	SetModel(ctx, "m")
	SetAccount(ctx, "a")
	SetSurface(ctx, "s")
	IncrRetry(ctx)
	IncrAccountSwitch(ctx)
	SetUpstreamResponseAt(ctx, time.Now())
	SetFirstByteAt(ctx, time.Now())
}
