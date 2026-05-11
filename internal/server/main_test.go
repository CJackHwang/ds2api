package server

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// All server tests call NewApp() without a real admin credential.
	// Set the fail-closed opt-in so tests can start the router normally.
	os.Setenv("DS2API_ALLOW_DEFAULT_ADMIN_KEY", "true")
	os.Exit(m.Run())
}
