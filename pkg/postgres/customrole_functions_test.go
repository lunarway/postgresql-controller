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

func TestSyncDatabaseFunctions_rejectsNameWithDoubleUnderscore(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	err = postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "bad__name", Returns: "void", Body: "NULL;"},
	})
	require.Error(t, err, "function name containing __ should be rejected")
}

func TestSyncDatabaseFunctions_createsFunction(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("cr-%d", epoch)
	funcName := "myfunc"
	// The actual PG function name is <rolename>__<funcname> with the role name verbatim.
	pgName := fmt.Sprintf("cr-%d__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	err = postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{
			Name:    funcName,
			Args:    "input_val integer",
			Returns: "integer",
			Body:    "RETURN input_val * 2;",
		},
	})
	require.NoError(t, err)

	assert.True(t, functionExists(t, adminDB, "public", pgName), "function should exist with prefixed name")
	assert.True(t, functionExecuteGranted(t, adminDB, roleName, "public", pgName),
		"EXECUTE should be granted to the role")
}

func TestSyncDatabaseFunctions_idempotent(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("custom_role_%d", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	funcs := []postgres.CustomRoleFunction{{
		Name:    "myfunc",
		Returns: "void",
		Body:    "NULL;",
	}}
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, funcs))
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, funcs),
		"second call should be idempotent")
}

func TestSyncDatabaseFunctions_dropsRemovedFunction(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	pgNameA := fmt.Sprintf("custom_role_%d__func_a", epoch)
	pgNameB := fmt.Sprintf("custom_role_%d__func_b", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Create both functions.
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "func_a", Returns: "void", Body: "NULL;"},
		{Name: "func_b", Returns: "void", Body: "NULL;"},
	}))
	require.True(t, functionExists(t, adminDB, "public", pgNameA))
	require.True(t, functionExists(t, adminDB, "public", pgNameB))

	// Re-sync with only func_a — func_b should be dropped.
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "func_a", Returns: "void", Body: "NULL;"},
	}))

	assert.True(t, functionExists(t, adminDB, "public", pgNameA), "func_a should remain")
	assert.False(t, functionExists(t, adminDB, "public", pgNameB), "func_b should be dropped")
}

func TestSyncDatabaseFunctions_dropsAllWhenEmpty(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	pgName := fmt.Sprintf("custom_role_%d__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "myfunc", Returns: "void", Body: "NULL;"},
	}))
	require.True(t, functionExists(t, adminDB, "public", pgName))

	// Re-sync with no functions — all should be dropped.
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, nil))
	assert.False(t, functionExists(t, adminDB, "public", pgName), "function should be dropped")
}

func TestDropManagedFunctions(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	pgName := fmt.Sprintf("custom_role_%d__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "myfunc", Args: "x integer", Returns: "integer", Body: "RETURN x;"},
	}))
	require.True(t, functionExists(t, adminDB, "public", pgName))

	require.NoError(t, postgres.DropManagedFunctions(log, adminDB, roleName))
	assert.False(t, functionExists(t, adminDB, "public", pgName), "function should be dropped")
}

// TestSyncDatabaseFunctions_doesNotDropFunctionsOfLongerPrefixRole verifies
// that when two roles share a name prefix (e.g. role "cr-X" prefix "cr-X__"
// and role "cr-X--extra" prefix "cr-X--extra__"), syncing the shorter role
// does not accidentally drop functions that belong to the longer one.
func TestSyncDatabaseFunctions_doesNotDropFunctionsOfLongerPrefixRole(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	// roleShort has prefix "cr-<epoch>__"
	// roleLong has prefix "cr-<epoch>--extra__"
	// A naive starts_with for roleShort also matches functions owned by roleLong.
	roleShort := fmt.Sprintf("cr-%d", epoch)
	roleLong := fmt.Sprintf("cr-%d--extra", epoch)
	pgFuncLong := fmt.Sprintf("cr-%d--extra__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleShort, nil))
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleLong, nil))

	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleLong, []postgres.CustomRoleFunction{
		{Name: "myfunc", Returns: "void", Body: "NULL;"},
	}))
	require.True(t, functionExists(t, adminDB, "public", pgFuncLong), "precondition: roleLong's function must exist")

	// Sync roleShort with no functions — must NOT touch roleLong's function.
	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleShort, nil))

	assert.True(t, functionExists(t, adminDB, "public", pgFuncLong),
		"roleLong's function must not be dropped when syncing roleShort")
}

// TestDropManagedFunctions_doesNotDropFunctionsOfLongerPrefixRole mirrors the
// SyncDatabaseFunctions variant but exercises the deletion-time cleanup path.
func TestDropManagedFunctions_doesNotDropFunctionsOfLongerPrefixRole(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleShort := fmt.Sprintf("cr-%d", epoch)
	roleLong := fmt.Sprintf("cr-%d--extra", epoch)
	pgFuncLong := fmt.Sprintf("cr-%d--extra__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleShort, nil))
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleLong, nil))

	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleLong, []postgres.CustomRoleFunction{
		{Name: "myfunc", Returns: "void", Body: "NULL;"},
	}))
	require.True(t, functionExists(t, adminDB, "public", pgFuncLong), "precondition: roleLong's function must exist")

	// Drop all managed functions for roleShort — must NOT affect roleLong's function.
	require.NoError(t, postgres.DropManagedFunctions(log, adminDB, roleShort))

	assert.True(t, functionExists(t, adminDB, "public", pgFuncLong),
		"roleLong's function must not be dropped when cleaning up roleShort")
}

func TestSyncDatabaseFunctions_securityDefiner(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	pgName := fmt.Sprintf("custom_role_%d__myfunc", epoch)

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	require.NoError(t, postgres.SyncDatabaseFunctions(log, adminDB, roleName, []postgres.CustomRoleFunction{
		{Name: "myfunc", Returns: "void", Body: "NULL;"},
	}))

	// Verify the function is SECURITY DEFINER.
	var securityType string
	err = adminDB.QueryRow(`
		SELECT p.prosecdef::text
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.proname = $1`, pgName).Scan(&securityType)
	require.NoError(t, err)
	assert.Equal(t, "true", securityType, "function should be SECURITY DEFINER")
}

// functionExists returns true if a function with the given name exists in the schema.
func functionExists(t *testing.T, db *sql.DB, schema, funcName string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = $1 AND p.proname = $2
		)`, schema, funcName).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// functionExecuteGranted returns true if roleName has EXECUTE on a function in the given schema.
func functionExecuteGranted(t *testing.T, db *sql.DB, roleName, schema, funcName string) bool {
	t.Helper()
	var granted bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace,
			     aclexplode(p.proacl) AS a(grantor, grantee, privilege_type, is_grantable)
			WHERE n.nspname = $1 AND p.proname = $2
			  AND a.grantee = (SELECT oid FROM pg_roles WHERE rolname = $3)
			  AND a.privilege_type = 'EXECUTE'
		)`, schema, funcName, roleName).Scan(&granted)
	require.NoError(t, err)
	return granted
}
