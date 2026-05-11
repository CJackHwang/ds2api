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
