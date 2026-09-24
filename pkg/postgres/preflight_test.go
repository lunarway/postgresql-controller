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

// The integration test fixture exposes "iam_creator" as a pre-existing role
// the connection user is implicitly a member of, so we use it as the
// configured superuser role to avoid provisioning RDS-specific roles.
const preflightSuperuserRole = "iam_creator"

func TestPreflight_sunshine(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db := preflightAdminConn(t, host)
	defer db.Close()

	err := postgres.Preflight(log, db, preflightSuperuserRole)
	assert.NoError(t, err, "preflight should pass when all assumptions are met")
}

func TestPreflight_emptySuperuserRole(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db := preflightAdminConn(t, host)
	defer db.Close()

	err := postgres.Preflight(log, db, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "superuser role name is empty")
}

func TestPreflight_userLacksSuperuserRoleMembership(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB := preflightAdminConn(t, host)
	defer adminDB.Close()

	// A login role that has not been granted the configured superuser role
	// must be rejected.
	plebUser := fmt.Sprintf("preflight_pleb_%d", time.Now().UnixNano())
	plebPassword := "pleb"
	_, err := adminDB.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEROLE NOCREATEDB", plebUser, plebPassword))
	require.NoError(t, err, "create unprivileged role")
	defer func() {
		_, _ = adminDB.Exec(fmt.Sprintf("DROP ROLE %s", plebUser))
	}()

	plebDB, err := postgres.Connect(postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     plebUser,
		Password: plebPassword,
	})
	require.NoError(t, err, "connect as unprivileged user")
	defer plebDB.Close()

	err = postgres.Preflight(log, plebDB, preflightSuperuserRole)
	require.Error(t, err)
	assert.Contains(t, err.Error(), preflightSuperuserRole)
	assert.Contains(t, err.Error(), plebUser)
}

func preflightAdminConn(t *testing.T, host string) *sql.DB {
	t.Helper()
	db, err := postgres.Connect(postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err, "connect to database as admin")
	return db
}
