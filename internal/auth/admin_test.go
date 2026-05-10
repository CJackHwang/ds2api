package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/config"
)

func TestHashAdminPasswordUsesBcryptByDefault(t *testing.T) {
	hash := HashAdminPassword("hunter2")
	if !strings.HasPrefix(hash, bcryptHashPrefix) {
		t.Fatalf("expected bcrypt-prefixed hash, got %q", hash)
	}
	if !verifyAdminPasswordHash("hunter2", hash) {
		t.Fatal("bcrypt round-trip verification failed")
	}
	if verifyAdminPasswordHash("wrong", hash) {
		t.Fatal("expected verification of wrong password to fail")
	}
	// New invocations must produce distinct hashes (random salt).
	if HashAdminPassword("hunter2") == hash {
		t.Fatal("bcrypt salt did not vary between calls")
	}
}

func TestHashAdminPasswordLongInputUsesBcrypt(t *testing.T) {
	// Passwords longer than bcrypt's 72-byte input limit must still be stored
	// as bcrypt (via the sha256 pre-hash path) rather than silently falling
	// back to the legacy sha256 algorithm.
	longPwd := strings.Repeat("x", 100)
	hash := HashAdminPassword(longPwd)
	if !strings.HasPrefix(hash, bcryptHashPrefix) {
		t.Fatalf("expected bcrypt hash for long password, got %q", hash)
	}
	if !verifyAdminPasswordHash(longPwd, hash) {
		t.Fatal("long-password bcrypt round-trip failed")
	}
	if verifyAdminPasswordHash(strings.Repeat("x", 99), hash) {
		t.Fatal("different long password should not verify")
	}
}

func TestVerifyAdminPasswordHashAcceptsLegacySha256(t *testing.T) {
	sum := sha256.Sum256([]byte("legacy-pass"))
	legacy := sha256HashPrefix + hex.EncodeToString(sum[:])
	if !verifyAdminPasswordHash("legacy-pass", legacy) {
		t.Fatal("expected legacy sha256 hash to verify against original password")
	}
	if verifyAdminPasswordHash("other", legacy) {
		t.Fatal("expected legacy sha256 hash to reject wrong password")
	}
}

func TestIsLegacyHashClassification(t *testing.T) {
	if !isLegacyHash("sha256:" + hex.EncodeToString([]byte("x"))) {
		t.Fatal("sha256-prefixed hash must be classified as legacy")
	}
	if isLegacyHash(HashAdminPassword("p")) {
		t.Fatal("freshly generated bcrypt hash must not be classified as legacy")
	}
	if isLegacyHash("") {
		t.Fatal("empty hash must not be classified as legacy")
	}
}

func TestVerifyAdminCredentialWithUpgradeRewritesLegacyHash(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	sum := sha256.Sum256([]byte("oldpass"))
	legacy := sha256HashPrefix + hex.EncodeToString(sum[:])
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+legacy+`"}}`)
	store := config.LoadStore()

	ok, upgraded := VerifyAdminCredentialWithUpgrade("oldpass", store)
	if !ok {
		t.Fatal("expected legacy sha256 credential to verify")
	}
	if !strings.HasPrefix(upgraded, bcryptHashPrefix) {
		t.Fatalf("expected upgrade hash to use bcrypt prefix, got %q", upgraded)
	}
	if !verifyAdminPasswordHash("oldpass", upgraded) {
		t.Fatal("upgraded bcrypt hash failed to verify against original password")
	}
}

func TestVerifyAdminCredentialWithUpgradeNoRewriteForBcrypt(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	bcryptHash := HashAdminPassword("modern")
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+bcryptHash+`"}}`)
	store := config.LoadStore()

	ok, upgraded := VerifyAdminCredentialWithUpgrade("modern", store)
	if !ok {
		t.Fatal("expected bcrypt credential to verify")
	}
	if upgraded != "" {
		t.Fatalf("expected no upgrade for bcrypt-stored hash, got %q", upgraded)
	}
}

func TestVerifyAdminCredentialWithUpgradeRejectsWrongCandidate(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("right")+`"}}`)
	store := config.LoadStore()

	ok, upgraded := VerifyAdminCredentialWithUpgrade("wrong", store)
	if ok {
		t.Fatal("expected wrong credential to be rejected")
	}
	if upgraded != "" {
		t.Fatalf("expected no upgrade hash on rejection, got %q", upgraded)
	}
}

func TestJWTCreateVerify(t *testing.T) {
	token, err := CreateJWT(1)
	if err != nil {
		t.Fatalf("create jwt failed: %v", err)
	}
	payload, err := VerifyJWT(token)
	if err != nil {
		t.Fatalf("verify jwt failed: %v", err)
	}
	if payload["role"] != "admin" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestVerifyAdminRequest(t *testing.T) {
	token, _ := CreateJWT(1)
	req, _ := http.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := VerifyAdminRequest(req); err != nil {
		t.Fatalf("expected token accepted: %v", err)
	}
}

func TestUsingDefaultAdminKeyReturnsTrueWhenNotSet(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	if !UsingDefaultAdminKey(nil) {
		t.Fatal("expected UsingDefaultAdminKey to return true when DS2API_ADMIN_KEY is empty and store is nil")
	}
}

func TestUsingDefaultAdminKeyReturnsFalseWhenEnvSet(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "my-secret-key")
	if UsingDefaultAdminKey(nil) {
		t.Fatal("expected UsingDefaultAdminKey to return false when DS2API_ADMIN_KEY is set")
	}
}

func TestUsingDefaultAdminKeyReturnsFalseWhenStoreHasHash(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("somepassword")+`"}}`)
	store := config.LoadStore()
	if UsingDefaultAdminKey(store) {
		t.Fatal("expected UsingDefaultAdminKey to return false when store has password_hash configured")
	}
}

func TestVerifyJWTWithStoreValidAfter(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("oldpass")+`"}}`)
	store := config.LoadStore()
	token, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create jwt failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(token, store); err != nil {
		t.Fatalf("verify before invalidation failed: %v", err)
	}
	if err := store.Update(func(c *config.Config) error {
		c.Admin.JWTValidAfterUnix = 1<<62 - 1
		return nil
	}); err != nil {
		t.Fatalf("set valid-after failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(token, store); err == nil {
		t.Fatal("expected token invalid after valid-after update")
	}
}

func TestVerifyJWTWithStoreSameSecondInvalidationAndRelogin(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("oldpass")+`"}}`)
	store := config.LoadStore()

	oldToken, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create old jwt failed: %v", err)
	}
	oldPayload, err := VerifyJWTWithStore(oldToken, store)
	if err != nil {
		t.Fatalf("verify old jwt before invalidation failed: %v", err)
	}
	oldIAT, _ := oldPayload["iat"].(float64)

	if err := store.Update(func(c *config.Config) error {
		c.Admin.JWTValidAfterUnix = int64(oldIAT)
		return nil
	}); err != nil {
		t.Fatalf("set valid-after failed: %v", err)
	}

	if _, err := VerifyJWTWithStore(oldToken, store); err == nil {
		t.Fatal("expected old token invalid when iat == valid-after")
	}

	newToken, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create new jwt failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(newToken, store); err != nil {
		t.Fatalf("expected new token valid after invalidation cutoff: %v", err)
	}
}
