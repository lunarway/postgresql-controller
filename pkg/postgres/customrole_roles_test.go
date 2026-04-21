package postgres_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

func TestEnsureCustomRole_createsRole(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer db.Close()

	roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())

	err = postgres.EnsureCustomRole(log, db, roleName, nil)
	require.NoError(t, err)

	assert.True(t, roleExists(t, db, roleName), "role should exist")
	assert.False(t, roleCanLogin(t, db, roleName), "role should not have login")
}

func TestEnsureCustomRole_idempotent(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer db.Close()

	roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())

	require.NoError(t, postgres.EnsureCustomRole(log, db, roleName, nil))
	require.NoError(t, postgres.EnsureCustomRole(log, db, roleName, nil), "second call should be idempotent")
}

func TestEnsureCustomRole_grantsRoles(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer db.Close()

	roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())

	err = postgres.EnsureCustomRole(log, db, roleName, []string{"pg_monitor"})
	require.NoError(t, err)

	granted := grantedRoles(t, db, roleName)
	assert.Contains(t, granted, "pg_monitor")
}

func TestEnsureCustomRole_revokesRemovedRole(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer db.Close()

	roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())

	// Grant pg_monitor.
	require.NoError(t, postgres.EnsureCustomRole(log, db, roleName, []string{"pg_monitor"}))
	require.Contains(t, grantedRoles(t, db, roleName), "pg_monitor")

	// Re-sync with empty list — pg_monitor should be revoked.
	require.NoError(t, postgres.EnsureCustomRole(log, db, roleName, nil))
	assert.NotContains(t, grantedRoles(t, db, roleName), "pg_monitor", "pg_monitor should be revoked")
}

// roleExists returns true if a role with the given name exists in pg_roles.
func roleExists(t *testing.T, db *sql.DB, roleName string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", roleName).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// grantedRoles returns the list of roles granted to the given role.
func grantedRoles(t *testing.T, db *sql.DB, roleName string) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT r.rolname
		FROM pg_auth_members m
		JOIN pg_roles r ON r.oid = m.roleid
		JOIN pg_roles u ON u.oid = m.member
		WHERE u.rolname = $1`, roleName)
	require.NoError(t, err)
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		roles = append(roles, name)
	}
	require.NoError(t, rows.Err())
	return roles
}
