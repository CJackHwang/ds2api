package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestLoginHandlerUpgradesLegacySha256ToBcrypt(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", "") // must be empty so store is file-backed
	sum := sha256.Sum256([]byte("legacy-pwd"))
	legacy := "sha256:" + hex.EncodeToString(sum[:])

	// Write config to a temp file so the store is file-backed and Update can
	// persist the bcrypt upgrade. Env-only stores skip the upgrade (see Fix #1).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgContent := `{"admin":{"password_hash":"` + legacy + `"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("DS2API_CONFIG_PATH", cfgPath)

	store := config.LoadStore()
	h := &Handler{Store: store}

	body := []byte(`{"admin_key":"legacy-pwd"}`)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := strings.TrimSpace(store.AdminPasswordHash())
	if !strings.HasPrefix(stored, "bcrypt:") {
		t.Fatalf("expected stored hash to be upgraded to bcrypt, got %q", stored)
	}

	// Subsequent login with the same password (now stored as bcrypt) succeeds.
	if !authn.VerifyAdminCredential("legacy-pwd", store) {
		t.Fatal("expected post-migration credential to verify against bcrypt hash")
	}

	// Wrong password is rejected against the new bcrypt hash.
	if authn.VerifyAdminCredential("legacy-pwd-wrong", store) {
		t.Fatal("expected wrong password to fail after migration")
	}
}

func TestLoginHandlerSkipsUpgradeOnEnvOnlyStore(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	sum := sha256.Sum256([]byte("legacy-pwd"))
	legacy := "sha256:" + hex.EncodeToString(sum[:])
	// Load from env without writeback: Update is a no-op on disk.
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+legacy+`"}}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "")

	store := config.LoadStore()
	h := &Handler{Store: store}

	body := []byte(`{"admin_key":"legacy-pwd"}`)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body)))

	// Login must succeed even though upgrade is skipped.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Hash must remain sha256 (upgrade was skipped to protect token validity).
	stored := strings.TrimSpace(store.AdminPasswordHash())
	if !strings.HasPrefix(stored, "sha256:") {
		t.Fatalf("expected hash to remain sha256 on env-only store, got %q", stored)
	}
}

func TestLoginHandlerDoesNotRewriteBcryptHash(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	original := authn.HashAdminPassword("modern-pwd")
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+original+`"}}`)

	store := config.LoadStore()
	h := &Handler{Store: store}

	body := []byte(`{"admin_key":"modern-pwd"}`)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(store.AdminPasswordHash()); got != original {
		t.Fatalf("expected bcrypt hash to be preserved verbatim; before=%q after=%q", original, got)
	}
}

func TestGetVercelConfigFallsBackToSavedConfig(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"],"vercel":{"token":"saved-token","project_id":"saved-project","team_id":"saved-team"}}`)
	t.Setenv("VERCEL_TOKEN", "")
	t.Setenv("VERCEL_PROJECT_ID", "")
	t.Setenv("VERCEL_TEAM_ID", "")
	h := &Handler{Store: config.LoadStore()}

	rec := httptest.NewRecorder()
	h.getVercelConfig(rec, httptest.NewRequest(http.MethodGet, "/admin/vercel/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["has_token"] != true {
		t.Fatalf("expected saved token to be detected: %#v", payload)
	}
	if payload["token_source"] != "config" || payload["project_id"] != "saved-project" || payload["team_id"] != "saved-team" {
		t.Fatalf("unexpected preconfig payload: %#v", payload)
	}
	if payload["token_preview"] == "saved-token" {
		t.Fatal("token preview leaked the full token")
	}
}
