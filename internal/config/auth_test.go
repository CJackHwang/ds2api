package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// ─── AllowGeminiQueryKey accessor ────────────────────────────────────────

func TestAllowGeminiQueryKey_DefaultTrue(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "")
	s := &Store{}
	if !s.AllowGeminiQueryKey() {
		t.Fatal("expected default to be true (legacy behaviour)")
	}
}

func TestAllowGeminiQueryKey_NilStoreReturnsTrue(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "")
	var s *Store
	if !s.AllowGeminiQueryKey() {
		t.Fatal("expected nil store to fall back to default true")
	}
}

func TestAllowGeminiQueryKey_ConfigFalse(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "")
	falseVal := false
	s := &Store{cfg: Config{Auth: AuthConfig{AllowGeminiQueryKey: &falseVal}}}
	if s.AllowGeminiQueryKey() {
		t.Fatal("expected config false to be honoured")
	}
}

func TestAllowGeminiQueryKey_ConfigTrueExplicit(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "")
	trueVal := true
	s := &Store{cfg: Config{Auth: AuthConfig{AllowGeminiQueryKey: &trueVal}}}
	if !s.AllowGeminiQueryKey() {
		t.Fatal("expected explicit config true to be honoured")
	}
}

func TestAllowGeminiQueryKey_EnvOverridesConfigToFalse(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "false")
	trueVal := true
	s := &Store{cfg: Config{Auth: AuthConfig{AllowGeminiQueryKey: &trueVal}}}
	if s.AllowGeminiQueryKey() {
		t.Fatal("expected env=false to override config=true")
	}
}

func TestAllowGeminiQueryKey_EnvOverridesConfigToTrue(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "1")
	falseVal := false
	s := &Store{cfg: Config{Auth: AuthConfig{AllowGeminiQueryKey: &falseVal}}}
	if !s.AllowGeminiQueryKey() {
		t.Fatal("expected env=1 to override config=false")
	}
}

func TestAllowGeminiQueryKey_EnvAliases(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "yes": true, "ON": true,
		"0": false, "false": false, "no": false, "OFF": false,
	}
	for raw, want := range cases {
		raw, want := raw, want
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", raw)
			s := &Store{}
			if got := s.AllowGeminiQueryKey(); got != want {
				t.Errorf("env=%q: got %v want %v", raw, got, want)
			}
		})
	}
}

func TestAllowGeminiQueryKey_EnvInvalidFallsToDefault(t *testing.T) {
	t.Setenv("DS2API_ALLOW_GEMINI_QUERY_KEY", "bogus")
	falseVal := false
	s := &Store{cfg: Config{Auth: AuthConfig{AllowGeminiQueryKey: &falseVal}}}
	// Invalid env value is ignored; config wins.
	if s.AllowGeminiQueryKey() {
		t.Fatal("expected invalid env to fall through to config (false)")
	}
}

// ─── AuthConfig JSON round-trip ──────────────────────────────────────────

func TestAuthConfig_RoundTripFalse(t *testing.T) {
	falseVal := false
	cfg := Config{Auth: AuthConfig{AllowGeminiQueryKey: &falseVal}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"allow_gemini_query_key":false`) {
		t.Fatalf("expected explicit false in marshalled JSON, got: %s", b)
	}
	var cfg2 Config
	if err := json.Unmarshal(b, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg2.Auth.AllowGeminiQueryKey == nil || *cfg2.Auth.AllowGeminiQueryKey != false {
		t.Fatalf("expected round-trip false, got %#v", cfg2.Auth.AllowGeminiQueryKey)
	}
}

func TestAuthConfig_OmittedWhenUnset(t *testing.T) {
	cfg := Config{}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "auth") {
		t.Fatalf("expected auth to be omitted when unset, got: %s", b)
	}
}

func TestAuthConfig_CloneIsolation(t *testing.T) {
	v := false
	src := Config{Auth: AuthConfig{AllowGeminiQueryKey: &v}}
	dst := src.Clone()
	// Mutating the source must not affect the clone.
	*src.Auth.AllowGeminiQueryKey = true
	if dst.Auth.AllowGeminiQueryKey == nil || *dst.Auth.AllowGeminiQueryKey != false {
		t.Fatalf("clone should not alias source pointer; got %#v", dst.Auth.AllowGeminiQueryKey)
	}
}
