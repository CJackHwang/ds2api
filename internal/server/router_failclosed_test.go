package server

import (
	"strings"
	"testing"
)

// TestNewApp_FailsClosedWithoutOptIn guards the PR8 contract: when no admin
// credential is configured (DS2API_ADMIN_KEY unset, admin.password_hash
// empty) AND DS2API_ALLOW_DEFAULT_ADMIN_KEY is not opted-in, NewApp() must
// refuse to start.
//
// Note: TestMain in main_test.go sets DS2API_ALLOW_DEFAULT_ADMIN_KEY=true
// for the rest of the package. This test explicitly clears the variable
// (via t.Setenv with an empty value) to exercise the fail-closed branch.
func TestNewApp_FailsClosedWithoutOptIn(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	t.Setenv("DS2API_ADMIN_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"],"accounts":[{"email":"u@example.com","password":"p"}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	app, err := NewApp()
	if err == nil {
		t.Fatalf("NewApp() expected fail-closed error, got app=%v err=nil", app)
	}
	if app != nil {
		t.Fatalf("NewApp() must return nil app on fail-closed error, got %v", app)
	}
	msg := err.Error()
	if !strings.Contains(msg, "no admin credential") {
		t.Fatalf("error message should mention 'no admin credential', got: %q", msg)
	}
	if !strings.Contains(msg, "DS2API_ALLOW_DEFAULT_ADMIN_KEY") {
		t.Fatalf("error message should reference opt-in env var, got: %q", msg)
	}
}

// TestNewApp_FailClosedBypassedByEnvOptIn documents that the insecure
// default is still permitted when the operator explicitly sets the opt-in
// env var to a truthy value. This mirrors the local/CI usage path.
func TestNewApp_FailClosedBypassedByEnvOptIn(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "true")
	t.Setenv("DS2API_ADMIN_KEY", "")
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["k1"],"accounts":[{"email":"u@example.com","password":"p"}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() with opt-in should succeed, got err=%v", err)
	}
	if app == nil {
		t.Fatal("NewApp() should return non-nil app on opt-in path")
	}
}

// TestNewApp_PasswordHashAvoidsFailClosed ensures that configuring
// admin.password_hash (the recommended production path) satisfies the
// fail-closed gate even when DS2API_ADMIN_KEY and the opt-in env var are
// both unset.
func TestNewApp_PasswordHashAvoidsFailClosed(t *testing.T) {
	t.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "")
	t.Setenv("DS2API_ADMIN_KEY", "")
	// Any non-empty password_hash is enough to flip UsingDefaultAdminKey
	// to false. The actual hash value is not validated at startup.
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[{"email":"u@example.com","password":"p"}],
		"admin":{"password_hash":"$2a$10$dummyhashforstartupcheckonly0000000000000000000000000"}
	}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() with password_hash should succeed, got err=%v", err)
	}
	if app == nil {
		t.Fatal("NewApp() should return non-nil app when password_hash is set")
	}
}
