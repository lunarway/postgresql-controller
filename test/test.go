package test

import (
	"os"
	"testing"
)

// Integration skips the test if no integration PostgreSQL host name is
// available as an environment variable.
// If it is available its value is returned.
func Integration(t *testing.T) string {
	host := os.Getenv("POSTGRESQL_CONTROLLER_INTEGRATION_HOST")
	if host == "" {
		t.Skip("Integration tests not enabled as POSTGRESQL_CONTROLLER_INTEGRATION_HOST is empty")
	}
	return host
}
