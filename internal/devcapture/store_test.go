package devcapture

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewFromEnvDefaults(t *testing.T) {
	t.Setenv("DS2API_DEV_PACKET_CAPTURE_LIMIT", "")
	t.Setenv("DS2API_DEV_PACKET_CAPTURE_MAX_BODY_BYTES", "")
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")

	s := NewFromEnv()
	if s.Limit() != 20 {
		t.Fatalf("expected default limit 20, got %d", s.Limit())
	}
	if s.MaxBodyBytes() != 5*1024*1024 {
		t.Fatalf("expected default max body bytes 5MB, got %d", s.MaxBodyBytes())
	}
}

func TestNewFromEnvHonorsOverrides(t *testing.T) {
	t.Setenv("DS2API_DEV_PACKET_CAPTURE_LIMIT", "7")
	t.Setenv("DS2API_DEV_PACKET_CAPTURE_MAX_BODY_BYTES", "8192")
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	s := NewFromEnv()
	if s.Limit() != 7 {
		t.Fatalf("expected override limit 7, got %d", s.Limit())
	}
	if s.MaxBodyBytes() != 8192 {
		t.Fatalf("expected override max body bytes 8192, got %d", s.MaxBodyBytes())
	}
}

func TestStorePushKeepsNewestWithinLimit(t *testing.T) {
	s := &Store{enabled: true, limit: 2, maxBodyBytes: 1024}
	for i := 0; i < 3; i++ {
		session := s.Start("test", "http://x", "", map[string]any{"seq": i})
		if session == nil {
			t.Fatal("expected session")
		}
		rc := session.WrapBody(io.NopCloser(strings.NewReader("ok")), 200)
		_, _ = io.ReadAll(rc)
		_ = rc.Close()
	}
	items := s.Snapshot()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !strings.Contains(items[0].RequestBody, `"seq":2`) {
		t.Fatalf("expected newest first, got %#v", items[0].RequestBody)
	}
	if !strings.Contains(items[1].RequestBody, `"seq":1`) {
		t.Fatalf("expected second newest, got %#v", items[1].RequestBody)
	}
}

func TestWrapBodyTruncatesByLimit(t *testing.T) {
	s := &Store{enabled: true, limit: 5, maxBodyBytes: 4}
	session := s.Start("test", "http://x", "acc1", map[string]any{"x": 1})
	if session == nil {
		t.Fatal("expected session")
	}
	rc := session.WrapBody(io.NopCloser(strings.NewReader("abcdef")), 200)
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	items := s.Snapshot()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ResponseBody != "abcd" {
		t.Fatalf("expected truncated body, got %q", items[0].ResponseBody)
	}
	if !items[0].ResponseTruncated {
		t.Fatal("expected truncated flag true")
	}
	if items[0].AccountID != "acc1" {
		t.Fatalf("expected account id, got %q", items[0].AccountID)
	}
}

func TestWrapBodyRedactsAPIKeyInResponseBody(t *testing.T) {
	s := &Store{enabled: true, limit: 5, maxBodyBytes: 1024}
	session := s.Start("test", "http://x", "acc1", map[string]any{"model": "gpt-4"})
	if session == nil {
		t.Fatal("expected session")
	}
	errBody := `{"error":{"type":"authentication_error","api_key":"sk-secret123","message":"invalid key"}}`
	rc := session.WrapBody(io.NopCloser(strings.NewReader(errBody)), 401)
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	items := s.Snapshot()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if strings.Contains(items[0].ResponseBody, "sk-secret123") {
		t.Fatalf("expected api_key to be redacted in response body, got %q", items[0].ResponseBody)
	}
	if !strings.Contains(items[0].ResponseBody, "<redacted>") {
		t.Fatalf("expected <redacted> marker in response body, got %q", items[0].ResponseBody)
	}
}

func TestWrapBodyTruncatesUTF8WithoutBreakingRune(t *testing.T) {
	s := &Store{enabled: true, limit: 5, maxBodyBytes: 5}
	session := s.Start("test", "http://x", "acc1", map[string]any{"x": 1})
	if session == nil {
		t.Fatal("expected session")
	}
	rc := session.WrapBody(io.NopCloser(strings.NewReader("😀xy")), 200)
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	items := s.Snapshot()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !utf8.ValidString(items[0].ResponseBody) {
		t.Fatalf("expected valid utf-8 response body, got %q", items[0].ResponseBody)
	}
	if items[0].ResponseBody != "😀x" {
		t.Fatalf("expected rune-safe truncation, got %q", items[0].ResponseBody)
	}
	if !items[0].ResponseTruncated {
		t.Fatal("expected truncated flag true")
	}
}
