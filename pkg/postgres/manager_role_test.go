package postgres_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

func TestEnsureManagerRole_createsAndIsIdempotent(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	db := preflightAdminConn(t, host)
	defer db.Close()

	role := fmt.Sprintf("ensure_manager_%d", time.Now().UnixNano())
	defer func() {
		_, _ = db.Exec(fmt.Sprintf("DROP ROLE %s", role))
	}()

	require.NoError(t, postgres.EnsureManagerRole(log, db, role), "first call should create the role")

	var exists bool
	require.NoError(t, db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists))
	assert.True(t, exists, "role should exist after EnsureManagerRole")

	assert.NoError(t, postgres.EnsureManagerRole(log, db, role), "second call should be a no-op")
}

func TestEnsureManagerRole_emptyName(t *testing.T) {
	log := test.SetLogger(t)
	err := postgres.EnsureManagerRole(log, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role name is empty")
}
