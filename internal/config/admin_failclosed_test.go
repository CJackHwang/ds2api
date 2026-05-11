package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// ─── AllowDefaultAdminKey accessor ────────────────────────────────────────────

func TestAllowDefaultAdminKey_DefaultIsFalse(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	s := &Store{}
	if s.AllowDefaultAdminKey() {
		t.Fatal("default must be false (fail-closed)")
	}
}

func TestAllowDefaultAdminKey_NilStoreIsFalse(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	var s *Store
	if s.AllowDefaultAdminKey() {
		t.Fatal("nil store must return false")
	}
}

func TestAllowDefaultAdminKey_EnvTruthy(t *testing.T) {
	for _, raw := range []string{"1", "true", "True", "TRUE", "yes", "on"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", raw)
			s := &Store{}
			if !s.AllowDefaultAdminKey() {
				t.Errorf("env=%q: expected true", raw)
			}
		})
	}
}

func TestAllowDefaultAdminKey_EnvFalsy(t *testing.T) {
	for _, raw := range []string{"0", "false", "no", "off"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", raw)
			trueVal := true
			s := &Store{cfg: Config{Admin: AdminConfig{AllowDefaultAdminKey: &trueVal}}}
			if s.AllowDefaultAdminKey() {
				t.Errorf("env=%q should override config true → false", raw)
			}
		})
	}
}

func TestAllowDefaultAdminKey_ConfigTrue(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	trueVal := true
	s := &Store{cfg: Config{Admin: AdminConfig{AllowDefaultAdminKey: &trueVal}}}
	if !s.AllowDefaultAdminKey() {
		t.Fatal("config true should allow")
	}
}

func TestAllowDefaultAdminKey_ConfigFalse(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	falseVal := false
	s := &Store{cfg: Config{Admin: AdminConfig{AllowDefaultAdminKey: &falseVal}}}
	if s.AllowDefaultAdminKey() {
		t.Fatal("config false should deny")
	}
}

func TestAllowDefaultAdminKey_EnvInvalidFallsToConfig(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "bogus")
	trueVal := true
	s := &Store{cfg: Config{Admin: AdminConfig{AllowDefaultAdminKey: &trueVal}}}
	if !s.AllowDefaultAdminKey() {
		t.Fatal("invalid env value should fall through to config (true)")
	}
}

// ─── AdminConfig JSON round-trip ──────────────────────────────────────────────

func TestAdminConfig_AllowDefaultAdminKey_RoundTripTrue(t *testing.T) {
	trueVal := true
	cfg := Config{Admin: AdminConfig{AllowDefaultAdminKey: &trueVal}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"allow_default_admin_key":true`) {
		t.Fatalf("expected explicit true in JSON, got: %s", b)
	}
	var cfg2 Config
	if err := json.Unmarshal(b, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg2.Admin.AllowDefaultAdminKey == nil || !*cfg2.Admin.AllowDefaultAdminKey {
		t.Fatalf("round-trip failed: got %#v", cfg2.Admin.AllowDefaultAdminKey)
	}
}

func TestAdminConfig_AllowDefaultAdminKey_OmittedWhenUnset(t *testing.T) {
	cfg := Config{}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "allow_default_admin_key") {
		t.Fatalf("expected field to be omitted when nil, got: %s", b)
	}
}

func TestAdminConfig_AllowDefaultAdminKey_CloneIsolation(t *testing.T) {
	trueVal := true
	src := Config{Admin: AdminConfig{AllowDefaultAdminKey: &trueVal}}
	dst := src.Clone()
	*src.Admin.AllowDefaultAdminKey = false
	if dst.Admin.AllowDefaultAdminKey == nil || !*dst.Admin.AllowDefaultAdminKey {
		t.Fatal("clone must not alias source pointer")
	}
}
