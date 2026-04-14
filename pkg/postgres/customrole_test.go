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

func TestApplyDatabaseGrants_specificSchemaAllTables(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)

	// Create the database and seed it with a schema + table
	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))

	// Create the role and apply grants
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err)

	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"))
}

func TestApplyDatabaseGrants_specificTable(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)
	otherTable := fmt.Sprintf("other_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, otherTable))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: tableName, Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err)

	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "grant should apply to specified table")
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, otherTable, "SELECT"), "grant should not apply to other table")
}

func TestApplyDatabaseGrants_allSchemasAllTables(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaA := fmt.Sprintf("schema_a_%d", epoch)
	schemaB := fmt.Sprintf("schema_b_%d", epoch)
	table := fmt.Sprintf("table_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaA))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaA, table))
	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaB))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaB, table))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Empty schema = all schemas
	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err)

	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaA, table, "SELECT"))
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaB, table, "SELECT"))
}

func TestApplyDatabaseGrants_idempotent(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	grants := []postgres.CustomRoleGrant{{Schema: schemaName, Privileges: []string{"SELECT"}}}
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, grants))
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, grants), "second call should be idempotent")
}

func TestSyncDatabaseGrants_grantCombinations(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	sA := fmt.Sprintf("schema_a_%d", epoch)
	sB := fmt.Sprintf("schema_b_%d", epoch)
	sC := fmt.Sprintf("schema_c_%d", epoch)
	tX := fmt.Sprintf("table_x_%d", epoch)
	tY := fmt.Sprintf("table_y_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	for _, schema := range []string{sA, sB, sC} {
		dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schema))
		for _, table := range []string{tX, tY} {
			dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schema, table))
		}
	}

	type check struct {
		schema, table, privilege string
		want                     bool
	}

	cases := []struct {
		name   string
		grants []postgres.CustomRoleGrant
		checks []check
	}{
		{
			name:   "specific schema specific table",
			grants: []postgres.CustomRoleGrant{{Schema: sA, Table: tX, Privileges: []string{"SELECT"}}},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", false},
				{sB, tX, "SELECT", false},
				{sB, tY, "SELECT", false},
				{sC, tX, "SELECT", false},
			},
		},
		{
			name:   "specific schema all tables",
			grants: []postgres.CustomRoleGrant{{Schema: sA, Privileges: []string{"SELECT"}}},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sB, tX, "SELECT", false},
				{sB, tY, "SELECT", false},
				{sC, tX, "SELECT", false},
			},
		},
		{
			name:   "wildcard schema specific table",
			grants: []postgres.CustomRoleGrant{{Table: tX, Privileges: []string{"SELECT"}}},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sB, tX, "SELECT", true},
				{sC, tX, "SELECT", true},
				{sA, tY, "SELECT", false},
				{sB, tY, "SELECT", false},
				{sC, tY, "SELECT", false},
			},
		},
		{
			name:   "wildcard schema wildcard table",
			grants: []postgres.CustomRoleGrant{{Privileges: []string{"SELECT"}}},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sB, tX, "SELECT", true},
				{sB, tY, "SELECT", true},
				{sC, tX, "SELECT", true},
				{sC, tY, "SELECT", true},
			},
		},
		{
			name: "multiple explicit schemas specific table",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Table: tX, Privileges: []string{"SELECT"}},
				{Schema: sB, Table: tX, Privileges: []string{"SELECT"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sB, tX, "SELECT", true},
				{sA, tY, "SELECT", false},
				{sB, tY, "SELECT", false},
				{sC, tX, "SELECT", false},
				{sC, tY, "SELECT", false},
			},
		},
		{
			name: "multiple explicit schemas all tables",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Privileges: []string{"SELECT"}},
				{Schema: sB, Privileges: []string{"SELECT"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sB, tX, "SELECT", true},
				{sB, tY, "SELECT", true},
				{sC, tX, "SELECT", false},
				{sC, tY, "SELECT", false},
			},
		},
		{
			name: "single schema multiple specific tables",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Table: tX, Privileges: []string{"SELECT"}},
				{Schema: sA, Table: tY, Privileges: []string{"SELECT"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sB, tX, "SELECT", false},
				{sB, tY, "SELECT", false},
			},
		},
		{
			name: "multiple schemas multiple specific tables",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Table: tX, Privileges: []string{"SELECT"}},
				{Schema: sA, Table: tY, Privileges: []string{"SELECT"}},
				{Schema: sB, Table: tX, Privileges: []string{"SELECT"}},
				{Schema: sB, Table: tY, Privileges: []string{"SELECT"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sB, tX, "SELECT", true},
				{sB, tY, "SELECT", true},
				{sC, tX, "SELECT", false},
				{sC, tY, "SELECT", false},
			},
		},
		{
			name: "mixed specific and wildcard tables across schemas",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Table: tX, Privileges: []string{"SELECT"}},
				{Schema: sB, Privileges: []string{"SELECT"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", false},
				{sB, tX, "SELECT", true},
				{sB, tY, "SELECT", true},
				{sC, tX, "SELECT", false},
				{sC, tY, "SELECT", false},
			},
		},
		{
			name: "multiple privileges on specific table",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Table: tX, Privileges: []string{"SELECT", "INSERT", "DELETE"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tX, "INSERT", true},
				{sA, tX, "DELETE", true},
				{sA, tX, "UPDATE", false},
				{sA, tY, "SELECT", false},
				{sB, tX, "SELECT", false},
			},
		},
		{
			name: "different privileges per schema",
			grants: []postgres.CustomRoleGrant{
				{Schema: sA, Privileges: []string{"SELECT"}},
				{Schema: sB, Privileges: []string{"INSERT", "UPDATE"}},
			},
			checks: []check{
				{sA, tX, "SELECT", true},
				{sA, tY, "SELECT", true},
				{sA, tX, "INSERT", false},
				{sB, tX, "INSERT", true},
				{sB, tY, "UPDATE", true},
				{sB, tX, "SELECT", false},
				{sC, tX, "SELECT", false},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roleName := fmt.Sprintf("custom_role_%d", time.Now().UnixNano())
			require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

			require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, tc.grants))

			for _, c := range tc.checks {
				got := tablePrivilegeGranted(t, targetDB, roleName, c.schema, c.table, c.privilege)
				if c.want {
					assert.Truef(t, got, "expected %s on %s.%s to be granted", c.privilege, c.schema, c.table)
				} else {
					assert.Falsef(t, got, "expected %s on %s.%s NOT to be granted", c.privilege, c.schema, c.table)
				}
			}
		})
	}
}

func TestSyncDatabaseGrants_revokesRemovedPrivilege(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Apply SELECT and DELETE.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: tableName, Privileges: []string{"SELECT", "DELETE"}},
	}))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "DELETE"))

	// Re-sync with only SELECT — DELETE should be revoked.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: tableName, Privileges: []string{"SELECT"}},
	}))
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "SELECT should remain")
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "DELETE"), "DELETE should be revoked")
}

func TestSyncDatabaseGrants_revokesRemovedGrant(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Apply a grant on the schema.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Privileges: []string{"SELECT"}},
	}))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"))

	// Re-sync with no grants — all privileges and schema USAGE should be revoked.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, nil))
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "SELECT should be revoked")
	assert.False(t, schemaUsageGranted(t, targetDB, roleName, schemaName), "USAGE on schema should be revoked")
}

func TestSyncDatabaseGrants_partialRevokePreservesSchemaUsage(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableA := fmt.Sprintf("table_a_%d", epoch)
	tableB := fmt.Sprintf("table_b_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableA))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableB))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Grant SELECT on both tables.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Privileges: []string{"SELECT"}},
	}))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableA, "SELECT"))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableB, "SELECT"))

	// Re-sync with only tableA — tableB's grant is removed but schema USAGE must remain.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: tableA, Privileges: []string{"SELECT"}},
	}))
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableA, "SELECT"), "tableA SELECT should remain")
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableB, "SELECT"), "tableB SELECT should be revoked")
	assert.True(t, schemaUsageGranted(t, targetDB, roleName, schemaName), "USAGE on schema should be preserved")
}

func TestSyncDatabaseGrants_picksUpNewTable(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableA := fmt.Sprintf("table_a_%d", epoch)
	tableB := fmt.Sprintf("table_b_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableA))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Initial sync: only tableA exists.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Privileges: []string{"SELECT"}},
	}))
	require.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableA, "SELECT"))

	// New table added after initial sync.
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableB))

	// Re-sync with same spec — new table should be picked up.
	require.NoError(t, postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Privileges: []string{"SELECT"}},
	}))
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableA, "SELECT"), "tableA should retain SELECT")
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableB, "SELECT"), "tableB added after initial sync should get SELECT")
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

func TestUserDatabases(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	databases, err := postgres.UserDatabases(adminDB)
	require.NoError(t, err)

	assert.Contains(t, databases, dbName, "created database should appear in list")
	assert.NotContains(t, databases, "postgres", "postgres maintenance database should be excluded")
	assert.NotContains(t, databases, "rdsadmin", "RDS internal database should be excluded")
}

func TestSyncDatabaseGrants_skipsMissingSchemaAndTable(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	tableName := fmt.Sprintf("table_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, tableName))

	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Reference a schema that does not exist — should not error, just skip.
	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: "nonexistent_schema", Table: tableName, Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err, "missing schema should be silently skipped")
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "no grant should be applied")

	// Reference a table that does not exist in an existing schema — should not error, just skip.
	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: "nonexistent_table", Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err, "missing table should be silently skipped")
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "no grant should be applied")

	// Existing objects still work alongside missing ones in the same spec.
	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: tableName, Privileges: []string{"SELECT"}},
		{Schema: "nonexistent_schema", Table: tableName, Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err, "mixed existent/non-existent should not error")
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, tableName, "SELECT"), "existing table should still be granted")
}

// TestSyncDatabaseGrants_omittedSchemaAndTable_dbNamedSchema mimics a
// production-like setup where:
//   - The database is created via postgres.Database() (db-named schema)
//   - Tables exist only in the db-named schema, not in public
//   - The grant spec omits both schema and table (= wildcard all)
//
// Regression test for the hypothesis that omitted schema+table resolves to
// zero matches instead of defaulting to all.
func TestSyncDatabaseGrants_omittedSchemaAndTable_dbNamedSchema(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	tableName := fmt.Sprintf("tbl_%d", epoch)

	// Create database the production way — this creates a db-named schema.
	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	// Create a table in the db-named schema (mimicking what an application does).
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", dbName, tableName))

	// Verify public schema has no tables.
	var publicTableCount int
	require.NoError(t, targetDB.QueryRow(
		"SELECT count(*) FROM pg_tables WHERE schemaname = 'public'",
	).Scan(&publicTableCount))
	require.Equal(t, 0, publicTableCount, "public schema should have no tables")

	// Create the custom role and apply grants with both schema and table omitted.
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	err = postgres.SyncDatabaseGrants(log, targetDB, roleName, []postgres.CustomRoleGrant{
		{Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err)

	assert.True(t, schemaUsageGranted(t, targetDB, roleName, dbName),
		"USAGE should be granted on the db-named schema")
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, dbName, tableName, "SELECT"),
		"SELECT should be granted on the table in the db-named schema")
}

// TestSyncDatabaseGrants_skipsPermissionDenied verifies that when the
// controller user does not have permission to grant on a table (e.g. it is
// owned by another role), the grant is skipped with a warning rather than
// failing the entire reconciliation. Tables the controller does own should
// still be granted normally.
func TestSyncDatabaseGrants_skipsPermissionDenied(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: "postgres", User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)
	roleName := fmt.Sprintf("custom_role_%d", epoch)
	controllerUser := fmt.Sprintf("controller_%d", epoch)
	otherOwner := fmt.Sprintf("other_%d", epoch)
	schemaName := fmt.Sprintf("schema_%d", epoch)
	ownedTable := fmt.Sprintf("owned_%d", epoch)
	unownedTable := fmt.Sprintf("unowned_%d", epoch)

	// Set up the database.
	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	targetDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: "iam_creator", Password: "iam_creator",
	})
	require.NoError(t, err)
	defer targetDB.Close()

	// Create non-superuser roles: one acts as the controller, the other owns an inaccessible table.
	dbExec(t, adminDB, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", controllerUser, controllerUser))
	dbExec(t, adminDB, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", otherOwner, otherOwner))

	// Create schema owned by the controller so it can grant USAGE.
	dbExec(t, targetDB, fmt.Sprintf("CREATE SCHEMA %s AUTHORIZATION %s", schemaName, controllerUser))

	// Create two tables: one owned by the controller, one by another role.
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, ownedTable))
	dbExec(t, targetDB, fmt.Sprintf("CREATE TABLE %s.%s (id int)", schemaName, unownedTable))
	dbExec(t, targetDB, fmt.Sprintf("ALTER TABLE %s.%s OWNER TO %s", schemaName, ownedTable, controllerUser))
	dbExec(t, targetDB, fmt.Sprintf("ALTER TABLE %s.%s OWNER TO %s", schemaName, unownedTable, otherOwner))

	// Create the custom role on the admin database.
	require.NoError(t, postgres.EnsureCustomRole(log, adminDB, roleName, nil))

	// Connect as the controller — it owns the schema and one table but not the other.
	controllerDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host: host, Database: dbName, User: controllerUser, Password: controllerUser,
	})
	require.NoError(t, err)
	defer controllerDB.Close()

	// SyncDatabaseGrants should skip the unowned table and succeed on the owned one.
	err = postgres.SyncDatabaseGrants(log, controllerDB, roleName, []postgres.CustomRoleGrant{
		{Schema: schemaName, Table: ownedTable, Privileges: []string{"SELECT"}},
		{Schema: schemaName, Table: unownedTable, Privileges: []string{"SELECT"}},
	})
	require.NoError(t, err, "permission denied should be skipped, not returned as error")

	// Verify owned table got the grant.
	assert.True(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, ownedTable, "SELECT"),
		"privilege should be granted on the owned table")

	// Verify unowned table was skipped.
	assert.False(t, tablePrivilegeGranted(t, targetDB, roleName, schemaName, unownedTable, "SELECT"),
		"privilege should not have been granted on the unowned table")
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

// schemaUsageGranted returns true if roleName has USAGE on schema.
func schemaUsageGranted(t *testing.T, db *sql.DB, roleName, schema string) bool {
	t.Helper()
	var granted bool
	err := db.QueryRow(`
		SELECT EXISTS(
		    SELECT 1
		    FROM pg_namespace n,
		         aclexplode(n.nspacl) AS a(grantor, grantee, privilege_type, is_grantable)
		    WHERE a.grantee = (SELECT oid FROM pg_roles WHERE rolname = $1)
		      AND n.nspname = $2
		      AND a.privilege_type = 'USAGE'
		)`, roleName, schema).Scan(&granted)
	require.NoError(t, err)
	return granted
}

// tablePrivilegeGranted returns true if roleName has the given privilege on schema.table.
func tablePrivilegeGranted(t *testing.T, db *sql.DB, roleName, schema, table, privilege string) bool {
	t.Helper()
	var granted bool
	err := db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM information_schema.role_table_grants
		WHERE grantee = $1 AND table_schema = $2 AND table_name = $3 AND privilege_type = $4`,
		roleName, schema, table, privilege,
	).Scan(&granted)
	require.NoError(t, err)
	return granted
}
