package contextengine

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestMaybeShadow_NoopWhenOff(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	MaybeShadow("off", []map[string]any{
		{"role": "user", "content": "hello"},
	}, logger)

	if buf.Len() != 0 {
		t.Errorf("expected no log output for mode=off, got: %s", buf.String())
	}
}

func TestMaybeShadow_NoopWhenNilLogger(t *testing.T) {
	// Should not panic even with nil logger.
	MaybeShadow("shadow", []map[string]any{
		{"role": "user", "content": "hello"},
	}, nil)
}

func TestMaybeShadow_LogsOnShadow(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	msgs := []map[string]any{
		{"role": "system", "content": "You are helpful."},
		{"role": "user", "content": "hi"},
	}
	MaybeShadow("shadow", msgs, logger)

	out := buf.String()
	if !strings.Contains(out, "[context_engine_shadow]") {
		t.Errorf("expected [context_engine_shadow] in log, got: %s", out)
	}
	if !strings.Contains(out, "plan_id=") {
		t.Errorf("expected plan_id in log, got: %s", out)
	}
	if !strings.Contains(out, "segments_included=2") {
		t.Errorf("expected segments_included=2 in log, got: %s", out)
	}
}

// TestRedactWarnings_ScrubsSensitiveFields guards the PR #12 review fix:
// Compile diagnostics that incidentally carry JSON-shaped credentials or PII
// must not reach the admin-exposed PlanBuffer unredacted.
func TestRedactWarnings_ScrubsSensitiveFields(t *testing.T) {
	in := []string{
		`segment rejected: body={"api_key":"sk-live-abc","email":"u@x.com"}`,
		`ok`,
	}
	out := redactWarnings(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	if strings.Contains(out[0], "sk-live-abc") || strings.Contains(out[0], "u@x.com") {
		t.Errorf("sensitive payload leaked through redactWarnings: %q", out[0])
	}
	if !strings.Contains(out[0], "<redacted>") {
		t.Errorf("expected <redacted> marker, got: %q", out[0])
	}
	if out[1] != "ok" {
		t.Errorf("plain warning must not be rewritten, got: %q", out[1])
	}
}

func TestRedactWarnings_NilPreserved(t *testing.T) {
	if out := redactWarnings(nil); out != nil {
		t.Errorf("expected nil passthrough, got %#v", out)
	}
	empty := []string{}
	if out := redactWarnings(empty); len(out) != 0 {
		t.Errorf("expected empty passthrough, got %#v", out)
	}
}

func TestMaybeShadow_NoopWhenEnforce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	MaybeShadow("enforce", []map[string]any{
		{"role": "user", "content": "hello"},
	}, logger)

	// enforce mode is not yet implemented; shadow.go treats it as no-op.
	if buf.Len() != 0 {
		t.Errorf("expected no log output for mode=enforce (not yet active), got: %s", buf.String())
	}
}
