package auth

import (
	"net/http"
	"testing"

	"ds2api/internal/config"
)

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
