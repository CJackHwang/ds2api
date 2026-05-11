package config

import (
	"encoding/json"
	"testing"
)

func TestParserV2Mode_Default(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "")
	s := &Store{}
	if got := s.ParserV2Mode(); got != "off" {
		t.Errorf("expected off, got %q", got)
	}
}

func TestParserV2Mode_EnvVarShadow(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "shadow")
	s := &Store{}
	if got := s.ParserV2Mode(); got != "shadow" {
		t.Errorf("expected shadow, got %q", got)
	}
}

func TestParserV2Mode_EnvVarEnforce(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "enforce")
	s := &Store{}
	if got := s.ParserV2Mode(); got != "enforce" {
		t.Errorf("expected enforce, got %q", got)
	}
}

func TestParserV2Mode_InvalidEnvFallsToOff(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "bogus")
	s := &Store{}
	if got := s.ParserV2Mode(); got != "off" {
		t.Errorf("expected off for invalid env, got %q", got)
	}
}

func TestParserV2Mode_InvalidEnvOverridesConfigToOff(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "bogus")
	s := &Store{cfg: Config{ParserV2: ParserV2Config{Mode: "shadow"}}}
	if got := s.ParserV2Mode(); got != "off" {
		t.Errorf("expected off for invalid env overriding config, got %q", got)
	}
}

func TestParserV2Mode_ConfigFile(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "")
	s := &Store{cfg: Config{ParserV2: ParserV2Config{Mode: "shadow"}}}
	if got := s.ParserV2Mode(); got != "shadow" {
		t.Errorf("expected shadow from config, got %q", got)
	}
}

func TestParserV2Mode_EnforceFromConfig(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "")
	s := &Store{cfg: Config{ParserV2: ParserV2Config{Mode: "enforce"}}}
	if got := s.ParserV2Mode(); got != "enforce" {
		t.Errorf("expected enforce from config, got %q", got)
	}
}

func TestParserV2Mode_EnvOverridesConfig(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "off")
	s := &Store{cfg: Config{ParserV2: ParserV2Config{Mode: "shadow"}}}
	if got := s.ParserV2Mode(); got != "off" {
		t.Errorf("expected env to override config, got %q", got)
	}
}

func TestParserV2Mode_EnvCaseInsensitive(t *testing.T) {
	t.Setenv("DS2API_PARSER_V2", "SHADOW")
	s := &Store{}
	if got := s.ParserV2Mode(); got != "shadow" {
		t.Errorf("expected shadow (case-insensitive env), got %q", got)
	}
}

func TestParserV2Config_RoundTrip(t *testing.T) {
	cfg := Config{ParserV2: ParserV2Config{Mode: "shadow"}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cfg2 Config
	if err := json.Unmarshal(b, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg2.ParserV2.Mode != "shadow" {
		t.Errorf("expected shadow after round-trip, got %q", cfg2.ParserV2.Mode)
	}
}

func TestParserV2Config_Omitted(t *testing.T) {
	cfg := Config{}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(b); containsStr(got, "parser_v2") {
		t.Errorf("parser_v2 should be omitted when mode is empty, got: %s", got)
	}
}
