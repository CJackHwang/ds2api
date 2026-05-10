package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestLoginHandlerUpgradesLegacySha256ToBcrypt(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	sum := sha256.Sum256([]byte("legacy-pwd"))
	legacy := "sha256:" + hex.EncodeToString(sum[:])
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+legacy+`"}}`)

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
