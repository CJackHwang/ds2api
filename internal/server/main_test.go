package server

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// All server tests call NewApp() without a real admin credential.
	// Set the fail-closed opt-in so tests can start the router normally.
	// Individual tests that need to exercise the fail-closed branch can
	// clear the variable via t.Setenv at test scope.
	if err := os.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "true"); err != nil {
		panic("setenv DS2API_ALLOW_DEFAULT_ADMIN_KEY: " + err.Error())
	}
	os.Exit(m.Run())
}
