package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"

	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
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

// allowedTablePrivileges is the set of valid PostgreSQL table-level privilege keywords.
var allowedTablePrivileges = map[string]struct{}{
	"SELECT":     {},
	"INSERT":     {},
	"UPDATE":     {},
	"DELETE":     {},
	"TRUNCATE":   {},
	"REFERENCES": {},
	"TRIGGER":    {},
	"ALL":        {},
}

// validatePrivileges returns an error if privs is empty or contains any value
// that is not a recognised PostgreSQL table-level privilege keyword.
// Comparison is case-insensitive.
func validatePrivileges(privs []string) error {
	if len(privs) == 0 {
		return ctlerrors.NewInvalid(fmt.Errorf("privileges must not be empty"))
	}
	for _, p := range privs {
		if _, ok := allowedTablePrivileges[strings.ToUpper(p)]; !ok {
			return ctlerrors.NewInvalid(fmt.Errorf("invalid privilege %q: must be one of SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, ALL", p))
		}
	}
	return nil
}

// EnsureCustomRole creates a PostgreSQL role if it does not exist and
// synchronises server-level role grants to exactly match grantRoles:
// roles no longer in the list are revoked and missing ones are granted.
// The role is created with NOLOGIN.
func EnsureCustomRole(log logr.Logger, db *sql.DB, roleName string, grantRoles []string) error {
	log = log.WithValues("role", roleName)
	log.V(1).Info("Ensuring custom role")

	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s NOLOGIN", pq.QuoteIdentifier(roleName)))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return fmt.Errorf("create role %s: %w", roleName, err)
		}
		log.V(1).Info("Role already exists", "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.V(1).Info("Role created")
	}

	current, err := currentGrantedRoles(db, roleName)
	if err != nil {
		return fmt.Errorf("query granted roles for %s: %w", roleName, err)
	}

	desiredSet := make(map[string]struct{}, len(grantRoles))
	for _, r := range grantRoles {
		desiredSet[r] = struct{}{}
	}

	// Revoke roles no longer in the desired set.
	for _, r := range current {
		if _, ok := desiredSet[r]; !ok {
			_, err := db.Exec(fmt.Sprintf("REVOKE %s FROM %s", pq.QuoteIdentifier(r), pq.QuoteIdentifier(roleName)))
			if err != nil {
				return fmt.Errorf("revoke role %s from %s: %w", r, roleName, err)
			}
			log.V(1).Info("Revoked role", "role", r)
		}
	}

	// Grant roles not yet present.
	currentSet := make(map[string]struct{}, len(current))
	for _, r := range current {
		currentSet[r] = struct{}{}
	}
	var toGrant []string
	for _, r := range grantRoles {
		if _, ok := currentSet[r]; !ok {
			toGrant = append(toGrant, r)
		}
	}
	if len(toGrant) == 0 {
		return nil
	}
	quotedRoles := make([]string, len(toGrant))
	for i, r := range toGrant {
		quotedRoles[i] = pq.QuoteIdentifier(r)
	}
	_, err = db.Exec(fmt.Sprintf("GRANT %s TO %s", strings.Join(quotedRoles, ", "), pq.QuoteIdentifier(roleName)))
	if err != nil {
		return fmt.Errorf("grant roles %s to %s: %w", strings.Join(toGrant, ", "), roleName, err)
	}
	log.V(1).Info("Granted roles", "roles", toGrant)
	return nil
}

// currentGrantedRoles returns the names of roles currently granted to roleName.
func currentGrantedRoles(db *sql.DB, roleName string) ([]string, error) {
	rows, err := db.Query(`
		SELECT r.rolname
		FROM pg_auth_members m
		JOIN pg_roles r ON r.oid = m.roleid
		JOIN pg_roles u ON u.oid = m.member
		WHERE u.rolname = $1`, roleName)
	if err != nil {
		return nil, fmt.Errorf("query granted roles: %w", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan role name: %w", err)
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

// SyncDatabaseGrants synchronises the role's privileges in the currently-connected
// database to exactly match grants. Schemas where the role previously had USAGE are
// fully revoked first, then the desired grants are re-applied, so removed privileges
// and removed grants both converge to the desired state.
func SyncDatabaseGrants(log logr.Logger, db *sql.DB, roleName string, grants []CustomRoleGrant) error {
	// Revoke all existing schema/table grants for this role.
	currentSchemas, err := currentGrantedSchemas(db, roleName)
	if err != nil {
		return err
	}
	quotedRole := pq.QuoteIdentifier(roleName)
	for _, schema := range currentSchemas {
		quotedSchema := pq.QuoteIdentifier(schema)
		if _, err := db.Exec(fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s", quotedSchema, quotedRole)); err != nil {
			return fmt.Errorf("revoke table privileges on schema %s from %s: %w", schema, roleName, err)
		}
		if _, err := db.Exec(fmt.Sprintf("REVOKE USAGE ON SCHEMA %s FROM %s", quotedSchema, quotedRole)); err != nil {
			return fmt.Errorf("revoke usage on schema %s from %s: %w", schema, roleName, err)
		}
		log.V(1).Info("Revoked schema grants", "schema", schema)
	}

	// Re-apply desired grants.
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

// currentGrantedSchemas returns the names of schemas on which roleName has USAGE.
func currentGrantedSchemas(db *sql.DB, roleName string) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT n.nspname
		FROM pg_namespace n,
		     aclexplode(COALESCE(n.nspacl, acldefault('n', n.nspowner))) AS a
		WHERE a.grantee = (SELECT oid FROM pg_roles WHERE rolname = $1)
		  AND a.privilege_type = 'USAGE'
		  AND n.nspname NOT LIKE 'pg_%'
		  AND n.nspname <> 'information_schema'`, roleName)
	if err != nil {
		return nil, fmt.Errorf("query granted schemas for %s: %w", roleName, err)
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

// RevokeAllDatabaseGrants revokes all schema USAGE and table privileges that
// roleName holds in the currently-connected database. It is used during CR
// deletion to clean up before the role is dropped.
func RevokeAllDatabaseGrants(log logr.Logger, db *sql.DB, roleName string) error {
	schemas, err := currentGrantedSchemas(db, roleName)
	if err != nil {
		return err
	}
	quotedRole := pq.QuoteIdentifier(roleName)
	for _, schema := range schemas {
		quotedSchema := pq.QuoteIdentifier(schema)
		if _, err := db.Exec(fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s", quotedSchema, quotedRole)); err != nil {
			return fmt.Errorf("revoke table privileges on schema %s from %s: %w", schema, roleName, err)
		}
		if _, err := db.Exec(fmt.Sprintf("REVOKE USAGE ON SCHEMA %s FROM %s", quotedSchema, quotedRole)); err != nil {
			return fmt.Errorf("revoke usage on schema %s from %s: %w", schema, roleName, err)
		}
		log.V(1).Info("Revoked schema grants", "schema", schema)
	}
	return nil
}

// DropCustomRole drops the PostgreSQL role. All database-level grants must be
// revoked (via RevokeAllDatabaseGrants) on every database before calling this.
func DropCustomRole(log logr.Logger, db *sql.DB, roleName string) error {
	log = log.WithValues("role", roleName)
	if _, err := db.Exec(fmt.Sprintf("DROP ROLE IF EXISTS %s", pq.QuoteIdentifier(roleName))); err != nil {
		return fmt.Errorf("drop role %s: %w", roleName, err)
	}
	log.V(1).Info("Dropped role")
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
	if err := validatePrivileges(privileges); err != nil {
		return err
	}

	// Normalise keywords to uppercase for clarity; PostgreSQL is case-insensitive
	// for keywords but this keeps generated SQL consistent.
	upper := make([]string, len(privileges))
	for i, p := range privileges {
		upper[i] = strings.ToUpper(p)
	}
	privs := strings.Join(upper, ", ")

	quotedRole := pq.QuoteIdentifier(roleName)
	quotedSchema := pq.QuoteIdentifier(schema)

	_, err := db.Exec(fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", quotedSchema, quotedRole))
	if err != nil {
		return fmt.Errorf("grant usage on schema %s to %s: %w", schema, roleName, err)
	}
	log.V(1).Info("Granted USAGE on schema", "schema", schema)

	if table == "" || table == "*" {
		_, err = db.Exec(fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", privs, quotedSchema, quotedRole))
		if err != nil {
			return fmt.Errorf("grant %s on all tables in schema %s to %s: %w", privs, schema, roleName, err)
		}
		log.V(1).Info("Granted privileges on all tables in schema", "privileges", privs, "schema", schema)
	} else {
		quotedTable := pq.QuoteIdentifier(table)
		_, err = db.Exec(fmt.Sprintf("GRANT %s ON TABLE %s.%s TO %s", privs, quotedSchema, quotedTable, quotedRole))
		if err != nil {
			return fmt.Errorf("grant %s on table %s.%s to %s: %w", privs, schema, table, roleName, err)
		}
		log.V(1).Info("Granted privileges on table", "privileges", privs, "schema", schema, "table", table)
	}

	return nil
}
