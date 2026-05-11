package config

import (
	"encoding/json"
	"testing"
)

func TestContextEngineMode_Default(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "")
	s := &Store{}
	if got := s.ContextEngineMode(); got != "off" {
		t.Errorf("expected off, got %q", got)
	}
}

func TestContextEngineMode_EnvVarShadow(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "shadow")
	s := &Store{}
	if got := s.ContextEngineMode(); got != "shadow" {
		t.Errorf("expected shadow, got %q", got)
	}
}

func TestContextEngineMode_EnvVarEnforce(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "enforce")
	s := &Store{}
	if got := s.ContextEngineMode(); got != "enforce" {
		t.Errorf("expected enforce, got %q", got)
	}
}

func TestContextEngineMode_InvalidEnvFallsToOff(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "bogus")
	s := &Store{}
	if got := s.ContextEngineMode(); got != "off" {
		t.Errorf("expected off for invalid env, got %q", got)
	}
}

func TestContextEngineMode_InvalidEnvOverridesConfigToOff(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "bogus")
	s := &Store{cfg: Config{ContextEngine: ContextEngineConfig{Mode: "shadow"}}}
	if got := s.ContextEngineMode(); got != "off" {
		t.Errorf("expected off for invalid env overriding config, got %q", got)
	}
}

func TestContextEngineMode_ConfigFile(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "")
	s := &Store{cfg: Config{ContextEngine: ContextEngineConfig{Mode: "shadow"}}}
	if got := s.ContextEngineMode(); got != "shadow" {
		t.Errorf("expected shadow from config, got %q", got)
	}
}

func TestContextEngineMode_EnvOverridesConfig(t *testing.T) {
	t.Setenv("DS2API_CONTEXT_ENGINE", "off")
	s := &Store{cfg: Config{ContextEngine: ContextEngineConfig{Mode: "shadow"}}}
	if got := s.ContextEngineMode(); got != "off" {
		t.Errorf("expected env to override config, got %q", got)
	}
}

func TestContextEngineConfig_RoundTrip(t *testing.T) {
	cfg := Config{ContextEngine: ContextEngineConfig{Mode: "shadow"}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cfg2 Config
	if err := json.Unmarshal(b, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg2.ContextEngine.Mode != "shadow" {
		t.Errorf("expected shadow after round-trip, got %q", cfg2.ContextEngine.Mode)
	}
}

func TestContextEngineConfig_Omitted(t *testing.T) {
	cfg := Config{}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(b); contains(got, "context_engine") {
		t.Errorf("context_engine should be omitted when mode is empty, got: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
