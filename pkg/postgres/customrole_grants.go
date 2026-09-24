package postgres

import (
	"database/sql"
	"errors"
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
			return ctlerrors.NewInvalid(fmt.Errorf("invalid privilege %q: must be one of SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER", p))
		}
	}
	return nil
}

// grantKey identifies a single privilege on a specific table.
type grantKey struct {
	schema    string
	table     string
	privilege string
}

// isPermissionDenied returns true if err is a PostgreSQL insufficient_privilege error (SQLSTATE 42501).
func isPermissionDenied(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "42501"
}

// SyncDatabaseGrants synchronises the role's table privileges in the
// currently-connected database to exactly match grants. It computes the diff
// between current and desired (schema, table, privilege) tuples and issues only
// the necessary GRANT/REVOKE statements, avoiding any access outage window.
//
// GRANT/REVOKE statements run via execWithRole, which sets the session role to
// the object owner inside a transaction so they succeed even when the
// controller's connection role (e.g. iam_creator) does not directly own the
// objects and the role resets automatically on any exit path.
func SyncDatabaseGrants(log logr.Logger, db *sql.DB, roleName string, grants []CustomRoleGrant) error {
	for _, g := range grants {
		if err := validatePrivileges(g.Privileges); err != nil {
			return err
		}
	}

	currentGrants, err := currentTableGrants(db, roleName)
	if err != nil {
		return err
	}
	currentSet := make(map[grantKey]struct{}, len(currentGrants))
	for _, g := range currentGrants {
		currentSet[g] = struct{}{}
	}

	desiredGrants, err := expandGrants(log, db, grants)
	if err != nil {
		return err
	}
	desiredSet := make(map[grantKey]struct{}, len(desiredGrants))
	for _, g := range desiredGrants {
		desiredSet[g] = struct{}{}
	}

	currentSchemas, err := currentGrantedSchemas(db, roleName)
	if err != nil {
		return err
	}
	currentSchemaSet := make(map[string]struct{}, len(currentSchemas))
	for _, s := range currentSchemas {
		currentSchemaSet[s] = struct{}{}
	}
	desiredSchemaSet := make(map[string]struct{})
	for _, g := range desiredGrants {
		desiredSchemaSet[g.schema] = struct{}{}
	}

	// Look up object owners so we can set the role before GRANT/REVOKE.
	schemaOwners, err := schemaOwnerMap(db)
	if err != nil {
		return err
	}
	tblOwners, err := tableOwnerMap(db)
	if err != nil {
		return err
	}

	type tableKey struct{ schema, table string }

	// 1. Grant USAGE on schemas not yet accessible.
	for schema := range desiredSchemaSet {
		if _, ok := currentSchemaSet[schema]; !ok {
			owner, ok := schemaOwners[schema]
			if !ok {
				log.Info("Skipping schema USAGE grant: owner not found", "schema", schema, "role", roleName)
				continue
			}
			if err := execWithRole(db, owner, func(tx *sql.Tx) error {
				_, err := tx.Exec(fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s",
					pq.QuoteIdentifier(schema), pq.QuoteIdentifier(roleName)))
				return err
			}); err != nil {
				if isPermissionDenied(err) {
					log.Info("Skipping schema USAGE grant: permission denied", "schema", schema, "role", roleName)
					continue
				}
				return fmt.Errorf("grant usage on schema %s to %s: %w", schema, roleName, err)
			}
			log.Info("Granted USAGE on schema", "schema", schema)
		}
	}

	// 2. Grant new table privileges, batched per (schema, table).
	toGrant := make(map[tableKey][]string)
	for key := range desiredSet {
		if _, ok := currentSet[key]; !ok {
			tk := tableKey{key.schema, key.table}
			toGrant[tk] = append(toGrant[tk], key.privilege)
		}
	}
	for tk, privs := range toGrant {
		owner := tblOwners[tk.schema][tk.table]
		if owner == "" {
			log.Info("Skipping table grant: owner not found", "schema", tk.schema, "table", tk.table, "privileges", privs, "role", roleName)
			continue
		}
		privList := strings.Join(privs, ", ")
		if err := execWithRole(db, owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("GRANT %s ON TABLE %s.%s TO %s",
				privList,
				pq.QuoteIdentifier(tk.schema),
				pq.QuoteIdentifier(tk.table),
				pq.QuoteIdentifier(roleName)))
			return err
		}); err != nil {
			if isPermissionDenied(err) {
				log.Info("Skipping table grant: permission denied", "schema", tk.schema, "table", tk.table, "privileges", privs, "role", roleName)
				continue
			}
			return fmt.Errorf("grant %s on %s.%s to %s: %w", privList, tk.schema, tk.table, roleName, err)
		}
		log.Info("Granted privileges", "schema", tk.schema, "table", tk.table, "privileges", privs)
	}

	// 3. Revoke removed table privileges, batched per (schema, table).
	toRevoke := make(map[tableKey][]string)
	for key := range currentSet {
		if _, ok := desiredSet[key]; !ok {
			tk := tableKey{key.schema, key.table}
			toRevoke[tk] = append(toRevoke[tk], key.privilege)
		}
	}
	for tk, privs := range toRevoke {
		for _, p := range privs {
			if _, ok := allowedTablePrivileges[p]; !ok {
				log.Info("Revoking unrecognized privilege type from database catalog", "privilege", p, "schema", tk.schema, "table", tk.table)
			}
		}
		owner := tblOwners[tk.schema][tk.table]
		if owner == "" {
			log.Info("Skipping table revoke: owner not found", "schema", tk.schema, "table", tk.table, "privileges", privs, "role", roleName)
			continue
		}
		privList := strings.Join(privs, ", ")
		if err := execWithRole(db, owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("REVOKE %s ON TABLE %s.%s FROM %s",
				privList,
				pq.QuoteIdentifier(tk.schema),
				pq.QuoteIdentifier(tk.table),
				pq.QuoteIdentifier(roleName)))
			return err
		}); err != nil {
			if isPermissionDenied(err) {
				log.Info("Skipping table revoke: permission denied", "schema", tk.schema, "table", tk.table, "privileges", privs, "role", roleName)
				continue
			}
			return fmt.Errorf("revoke %s on %s.%s from %s: %w", privList, tk.schema, tk.table, roleName, err)
		}
		log.Info("Revoked privileges", "schema", tk.schema, "table", tk.table, "privileges", privs)
	}

	// 4. Revoke USAGE on schemas that no longer have any desired grants.
	for _, schema := range currentSchemas {
		if _, ok := desiredSchemaSet[schema]; !ok {
			owner, ok := schemaOwners[schema]
			if !ok {
				log.Info("Skipping schema USAGE revoke: owner not found", "schema", schema, "role", roleName)
				continue
			}
			if err := execWithRole(db, owner, func(tx *sql.Tx) error {
				_, err := tx.Exec(fmt.Sprintf("REVOKE USAGE ON SCHEMA %s FROM %s",
					pq.QuoteIdentifier(schema), pq.QuoteIdentifier(roleName)))
				return err
			}); err != nil {
				if isPermissionDenied(err) {
					log.Info("Skipping schema USAGE revoke: permission denied", "schema", schema, "role", roleName)
					continue
				}
				return fmt.Errorf("revoke usage on schema %s from %s: %w", schema, roleName, err)
			}
			log.Info("Revoked USAGE on schema", "schema", schema)
		}
	}

	return nil
}

// schemaOwnerMap returns a map of schema name to its owner role name for all
// user-defined schemas in the currently-connected database.
func schemaOwnerMap(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT n.nspname, r.rolname
		FROM pg_namespace n
		JOIN pg_roles r ON r.oid = n.nspowner
		WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'`)
	if err != nil {
		return nil, fmt.Errorf("query schema owners: %w", err)
	}
	defer rows.Close()
	owners := make(map[string]string)
	for rows.Next() {
		var schema, owner string
		if err := rows.Scan(&schema, &owner); err != nil {
			return nil, fmt.Errorf("scan schema owner: %w", err)
		}
		owners[schema] = owner
	}
	return owners, rows.Err()
}

// tableOwnerMap returns a nested map of schema -> table -> owner role name
// for all user-defined tables in the currently-connected database.
func tableOwnerMap(db *sql.DB) (map[string]map[string]string, error) {
	rows, err := db.Query(`
		SELECT schemaname, tablename, tableowner FROM pg_tables
		WHERE schemaname NOT LIKE 'pg_%' AND schemaname <> 'information_schema'`)
	if err != nil {
		return nil, fmt.Errorf("query table owners: %w", err)
	}
	defer rows.Close()
	owners := make(map[string]map[string]string)
	for rows.Next() {
		var schema, table, owner string
		if err := rows.Scan(&schema, &table, &owner); err != nil {
			return nil, fmt.Errorf("scan table owner: %w", err)
		}
		if owners[schema] == nil {
			owners[schema] = make(map[string]string)
		}
		owners[schema][table] = owner
	}
	return owners, rows.Err()
}

// currentTableGrants returns all table privileges held by roleName in the
// currently-connected database. Uses pg_catalog directly so results are not
// filtered by the current session's role membership.
// aclexplode returns privilege_type as full text names (SELECT, UPDATE, …),
// so they are selected as-is without any CASE conversion.
func currentTableGrants(db *sql.DB, roleName string) ([]grantKey, error) {
	rows, err := db.Query(`
		SELECT n.nspname, c.relname, a.privilege_type
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace,
		    aclexplode(c.relacl) AS a(grantor, grantee, privilege_type, is_grantable)
		WHERE a.grantee = (SELECT oid FROM pg_roles WHERE rolname = $1)
		  AND c.relkind = 'r'
		  AND n.nspname NOT LIKE 'pg_%'
		  AND n.nspname <> 'information_schema'`, roleName)
	if err != nil {
		return nil, fmt.Errorf("query table grants for %s: %w", roleName, err)
	}
	defer rows.Close()
	var grants []grantKey
	for rows.Next() {
		var g grantKey
		if err := rows.Scan(&g.schema, &g.table, &g.privilege); err != nil {
			return nil, fmt.Errorf("scan table grant: %w", err)
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// resolveTables returns the tables to apply a grant to within schema.
// If table is empty or "*" it returns all regular tables in the schema.
// For a concrete table name it checks existence; returns nil (not an error)
// if the table is not present in this database so callers can skip it.
func resolveTables(db *sql.DB, schema, table string) ([]string, error) {
	if table != "" && table != "*" {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname = $1 AND tablename = $2)`, schema, table).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check table %s.%s existence: %w", schema, table, err)
		}
		if !exists {
			return nil, nil
		}
		return []string{table}, nil
	}
	rows, err := db.Query(`
		SELECT tablename FROM pg_tables
		WHERE schemaname = $1
		ORDER BY tablename`, schema)
	if err != nil {
		return nil, fmt.Errorf("query tables in schema %s: %w", schema, err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// expandGrants resolves all CustomRoleGrant entries to concrete
// (schema, table, privilege) tuples against the current database.
// Missing schemas or tables are skipped with a warning log rather than
// causing an error, so that a grant targeting objects absent from one
// database does not block processing of other databases.
func expandGrants(log logr.Logger, db *sql.DB, grants []CustomRoleGrant) ([]grantKey, error) {
	var result []grantKey
	for _, grant := range grants {
		schemas, err := resolveSchemas(db, grant.Schema)
		if err != nil {
			return nil, fmt.Errorf("resolve schemas: %w", err)
		}
		if len(schemas) == 0 {
			log.Info("Schema not found in this database, skipping grant", "schema", grant.Schema)
			continue
		}
		for _, schema := range schemas {
			tables, err := resolveTables(db, schema, grant.Table)
			if err != nil {
				return nil, fmt.Errorf("resolve tables in schema %s: %w", schema, err)
			}
			if len(tables) == 0 && grant.Table != "" && grant.Table != "*" {
				log.Info("Table not found in this database, skipping grant", "schema", schema, "table", grant.Table)
				continue
			}
			for _, table := range tables {
				for _, priv := range grant.Privileges {
					result = append(result, grantKey{
						schema:    schema,
						table:     table,
						privilege: strings.ToUpper(priv),
					})
				}
			}
		}
	}
	return result, nil
}

// currentGrantedSchemas returns the names of schemas on which roleName has USAGE.
func currentGrantedSchemas(db *sql.DB, roleName string) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT n.nspname
		FROM pg_namespace n,
		     aclexplode(n.nspacl) AS a
		WHERE n.nspacl IS NOT NULL
		  AND a.grantee = (SELECT oid FROM pg_roles WHERE rolname = $1)
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
	schemaOwners, err := schemaOwnerMap(db)
	if err != nil {
		return err
	}
	tblOwners, err := tableOwnerMap(db)
	if err != nil {
		return err
	}
	quotedRole := pq.QuoteIdentifier(roleName)
	for _, schema := range schemas {
		quotedSchema := pq.QuoteIdentifier(schema)

		// Collect unique table owners for this schema so the bulk revoke
		// covers tables regardless of which role owns them.
		owners := make(map[string]struct{})
		for _, owner := range tblOwners[schema] {
			owners[owner] = struct{}{}
		}
		if owner, ok := schemaOwners[schema]; ok {
			owners[owner] = struct{}{}
		}

		if len(owners) == 0 {
			log.Info("Skipping schema revoke: no owners found", "schema", schema, "role", roleName)
			continue
		}

		for owner := range owners {
			if err := execWithRole(db, owner, func(tx *sql.Tx) error {
				_, err := tx.Exec(fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s",
					quotedSchema, quotedRole))
				return err
			}); err != nil {
				if !isPermissionDenied(err) {
					return fmt.Errorf("revoke table privileges on schema %s from %s: %w", schema, roleName, err)
				}
				log.Info("Skipping bulk table revoke: permission denied", "schema", schema, "owner", owner, "role", roleName)
			}
		}

		schemaOwner, ok := schemaOwners[schema]
		if !ok {
			continue
		}
		if err := execWithRole(db, schemaOwner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("REVOKE USAGE ON SCHEMA %s FROM %s",
				quotedSchema, quotedRole))
			return err
		}); err != nil {
			if !isPermissionDenied(err) {
				return fmt.Errorf("revoke usage on schema %s from %s: %w", schema, roleName, err)
			}
			log.Info("Skipping schema USAGE revoke: permission denied", "schema", schema, "role", roleName)
		}
		log.Info("Revoked schema grants", "schema", schema)
	}
	return nil
}

// resolveSchemas returns the schemas to apply a grant to. If schema is empty or
// "*" it returns all user-defined schemas in the current database.
// For a concrete schema name it checks existence; returns nil (not an error)
// if the schema is not present in this database so callers can skip it.
func resolveSchemas(db *sql.DB, schema string) ([]string, error) {
	if schema != "" && schema != "*" {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = $1)`, schema).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check schema %s existence: %w", schema, err)
		}
		if !exists {
			return nil, nil
		}
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
