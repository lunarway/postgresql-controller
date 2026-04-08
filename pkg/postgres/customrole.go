package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

// CustomRoleGrant defines schema/table privileges to apply to a role within a database.
type CustomRoleGrant struct {
	// Schema is the schema to grant privileges on. Empty or "*" means all user-defined schemas.
	Schema string
	// Table is the table to grant privileges on. Empty or "*" means all tables in the schema.
	Table string
	// Privileges is a list of PostgreSQL privilege keywords (e.g. SELECT, INSERT).
	Privileges []string
}

// EnsureCustomRole creates a PostgreSQL role if it does not exist and applies
// server-level role grants. The role is created with NOLOGIN.
func EnsureCustomRole(log logr.Logger, db *sql.DB, roleName string, grantRoles []string) error {
	log = log.WithValues("role", roleName)
	log.V(1).Info("Ensuring custom role")

	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s NOLOGIN", roleName))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return fmt.Errorf("create role %s: %w", roleName, err)
		}
		log.V(1).Info("Role already exists", "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.V(1).Info("Role created")
	}

	if len(grantRoles) == 0 {
		return nil
	}

	joined := strings.Join(grantRoles, ", ")
	_, err = db.Exec(fmt.Sprintf("GRANT %s TO %s", joined, roleName))
	if err != nil {
		return fmt.Errorf("grant roles %s to %s: %w", joined, roleName, err)
	}
	log.V(1).Info("Granted roles", "roles", grantRoles)

	return nil
}

// ApplyDatabaseGrants applies schema/table privilege grants to a role within the
// already-connected database. Empty or "*" for Schema means all user-defined schemas;
// empty or "*" for Table means all tables in the schema.
func ApplyDatabaseGrants(log logr.Logger, db *sql.DB, roleName string, grants []CustomRoleGrant) error {
	if len(grants) == 0 {
		return nil
	}

	for _, grant := range grants {
		schemas, err := resolveSchemas(db, grant.Schema)
		if err != nil {
			return fmt.Errorf("resolve schemas for grant: %w", err)
		}
		for _, schema := range schemas {
			if err := applyPrivilegeGrant(log, db, roleName, schema, grant.Table, grant.Privileges); err != nil {
				return err
			}
		}
	}
	return nil
}

// UserDatabases returns the names of all non-template databases on the server,
// excluding the postgres maintenance database.
func UserDatabases(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT datname FROM pg_database
		WHERE datistemplate = false AND datname <> 'postgres'
		ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("query databases: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan database name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// resolveSchemas returns the schemas to apply a grant to. If schema is empty or
// "*" it returns all user-defined schemas in the current database.
func resolveSchemas(db *sql.DB, schema string) ([]string, error) {
	if schema != "" && schema != "*" {
		return []string{schema}, nil
	}
	rows, err := db.Query(`
		SELECT nspname FROM pg_catalog.pg_namespace
		WHERE nspname NOT LIKE 'pg_%' AND nspname <> 'information_schema'
		ORDER BY nspname`)
	if err != nil {
		return nil, fmt.Errorf("query schemas: %w", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan schema name: %w", err)
		}
		schemas = append(schemas, name)
	}
	return schemas, rows.Err()
}

func applyPrivilegeGrant(log logr.Logger, db *sql.DB, roleName, schema, table string, privileges []string) error {
	privs := strings.Join(privileges, ", ")

	_, err := db.Exec(fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", schema, roleName))
	if err != nil {
		return fmt.Errorf("grant usage on schema %s to %s: %w", schema, roleName, err)
	}
	log.V(1).Info("Granted USAGE on schema", "schema", schema)

	if table == "" || table == "*" {
		_, err = db.Exec(fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", privs, schema, roleName))
		if err != nil {
			return fmt.Errorf("grant %s on all tables in schema %s to %s: %w", privs, schema, roleName, err)
		}
		log.V(1).Info("Granted privileges on all tables in schema", "privileges", privs, "schema", schema)
	} else {
		_, err = db.Exec(fmt.Sprintf("GRANT %s ON TABLE %s.%s TO %s", privs, schema, table, roleName))
		if err != nil {
			return fmt.Errorf("grant %s on table %s.%s to %s: %w", privs, schema, table, roleName, err)
		}
		log.V(1).Info("Granted privileges on table", "privileges", privs, "schema", schema, "table", table)
	}

	return nil
}
