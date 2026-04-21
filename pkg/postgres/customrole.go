package postgres

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"

	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
)

// isPermissionDenied returns true if err is a PostgreSQL insufficient_privilege error (SQLSTATE 42501).
func isPermissionDenied(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "42501"
}

// CustomRoleFunction defines a SECURITY DEFINER function to create in a database.
type CustomRoleFunction struct {
	// Name is the function name (created in the public schema).
	Name string
	// Args is the argument list (e.g. "role_name text"). Empty means no arguments.
	Args string
	// Returns is the return type (e.g. "void", "boolean", "TABLE(plan text)").
	Returns string
	// OwningRole is the PostgreSQL role that will own the function. If empty,
	// the database owner is used.
	OwningRole string
	// Body is the PL/pgSQL statements (without BEGIN/END).
	Body string
}

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

// EnsureCustomRole creates a PostgreSQL role if it does not exist and
// synchronises server-level role grants to exactly match grantRoles:
// roles no longer in the list are revoked and missing ones are granted.
// The role is created with NOLOGIN.
func EnsureCustomRole(log logr.Logger, db *sql.DB, roleName string, grantRoles []string) error {
	log = log.WithValues("role", roleName)
	log.Info("Ensuring custom role")

	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s NOLOGIN", pq.QuoteIdentifier(roleName)))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return fmt.Errorf("create role %s: %w", roleName, err)
		}
		log.Info("Role already exists", "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info("Role created")
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
			log.Info("Revoked role", "role", r)
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
	log.Info("Granted roles", "roles", toGrant)
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

// SyncDatabaseGrants synchronises the role's table privileges in the
// currently-connected database to exactly match grants. It computes the diff
// between current and desired (schema, table, privilege) tuples and issues only
// the necessary GRANT/REVOKE statements, avoiding any access outage window.
//
// GRANT/REVOKE statements are executed with SET ROLE to the object owner so
// that they succeed even when the controller's connection role (e.g.
// iam_creator) does not directly own the objects.
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

	// Look up object owners so we can SET ROLE before GRANT/REVOKE.
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
			if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; GRANT USAGE ON SCHEMA %s TO %s; RESET ROLE",
				pq.QuoteIdentifier(owner),
				pq.QuoteIdentifier(schema), pq.QuoteIdentifier(roleName))); err != nil {
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
		if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; GRANT %s ON TABLE %s.%s TO %s; RESET ROLE",
			pq.QuoteIdentifier(owner),
			privList,
			pq.QuoteIdentifier(tk.schema),
			pq.QuoteIdentifier(tk.table),
			pq.QuoteIdentifier(roleName))); err != nil {
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
		if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; REVOKE %s ON TABLE %s.%s FROM %s; RESET ROLE",
			pq.QuoteIdentifier(owner),
			privList,
			pq.QuoteIdentifier(tk.schema),
			pq.QuoteIdentifier(tk.table),
			pq.QuoteIdentifier(roleName))); err != nil {
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
			if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; REVOKE USAGE ON SCHEMA %s FROM %s; RESET ROLE",
				pq.QuoteIdentifier(owner),
				pq.QuoteIdentifier(schema), pq.QuoteIdentifier(roleName))); err != nil {
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
			quotedOwner := pq.QuoteIdentifier(owner)
			if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s; RESET ROLE",
				quotedOwner, quotedSchema, quotedRole)); err != nil {
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
		if _, err := db.Exec(fmt.Sprintf("SET ROLE %s; REVOKE USAGE ON SCHEMA %s FROM %s; RESET ROLE",
			pq.QuoteIdentifier(schemaOwner), quotedSchema, quotedRole)); err != nil {
			if !isPermissionDenied(err) {
				return fmt.Errorf("revoke usage on schema %s from %s: %w", schema, roleName, err)
			}
			log.Info("Skipping schema USAGE revoke: permission denied", "schema", schema, "role", roleName)
		}
		log.Info("Revoked schema grants", "schema", schema)
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
	log.Info("Dropped role")
	return nil
}

// UntargetableDatabases contains PostgreSQL system databases that must not be
// modified by the controller even when explicitly listed in spec.databases.
var UntargetableDatabases = map[string]struct{}{
	"rdsadmin":  {},
	"template0": {},
	"template1": {},
}

// UserDatabases returns the names of all non-template databases on the server,
// excluding system databases (postgres, rdsadmin) and template databases.
func UserDatabases(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT datname FROM pg_database
		WHERE datistemplate = false AND datname NOT IN ('postgres', 'rdsadmin')
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

// execWithRole runs fn inside a transaction with SET LOCAL ROLE so every
// statement in fn executes with owner's privileges. The role resets
// automatically when the transaction ends (commit or rollback).
func execWithRole(db *sql.DB, owner string, fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if _, err := tx.Exec("SET LOCAL ROLE " + pq.QuoteIdentifier(owner)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set role %s: %w", owner, err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// randomDollarTag returns a random dollar-quoting tag of the form $f_<hex>$
// that is safe to use as a PL/pgSQL function body delimiter.
func randomDollarTag() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return "$f_" + hex.EncodeToString(b) + "$"
}

// managedFunctionPrefix returns the prefix used to name functions managed by
// this role. Hyphens in the role name are replaced with underscores and a
// double underscore separates the role prefix from the user-supplied name.
// Example: role "vault-admin", func "disable_pgaudit" → "vault_admin__disable_pgaudit"
func managedFunctionPrefix(roleName string) string {
	return strings.ReplaceAll(roleName, "-", "_") + "__"
}

// managedFunctionName returns the full PostgreSQL function name for a managed function.
func managedFunctionName(roleName, funcName string) string {
	return managedFunctionPrefix(roleName) + funcName
}

// managedFunctionKey identifies a managed function by name, identity arguments, and owner.
type managedFunctionKey struct {
	name         string
	identityArgs string
	owner        string
}

// desiredFunctionKey identifies a desired function by name and canonical argument signature.
type desiredFunctionKey struct {
	name         string
	identityArgs string
}

// managedFunctions returns all functions in the public schema whose name starts
// with the managed prefix for roleName.
func managedFunctions(db *sql.DB, roleName string) ([]managedFunctionKey, error) {
	prefix := managedFunctionPrefix(roleName)
	rows, err := db.Query(`
		SELECT p.proname, pg_get_function_identity_arguments(p.oid), r.rolname
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		JOIN pg_roles r ON r.oid = p.proowner
		WHERE n.nspname = 'public'
		  AND starts_with(p.proname, $1)
		  AND position('__' in substring(p.proname from length($1)+1)) = 0`, prefix)
	if err != nil {
		return nil, fmt.Errorf("query managed functions for %s: %w", roleName, err)
	}
	defer rows.Close()
	var funcs []managedFunctionKey
	for rows.Next() {
		var f managedFunctionKey
		if err := rows.Scan(&f.name, &f.identityArgs, &f.owner); err != nil {
			return nil, fmt.Errorf("scan managed function: %w", err)
		}
		funcs = append(funcs, f)
	}
	return funcs, rows.Err()
}

// databaseOwner returns the role name that owns the currently-connected database.
func databaseOwner(db *sql.DB) (string, error) {
	var owner string
	if err := db.QueryRow(`
		SELECT r.rolname
		FROM pg_database d
		JOIN pg_roles r ON r.oid = d.datdba
		WHERE d.datname = current_database()`).Scan(&owner); err != nil {
		return "", fmt.Errorf("query database owner: %w", err)
	}
	return owner, nil
}

// SyncDatabaseFunctions reconciles SECURITY DEFINER functions in the
// currently-connected database. For each desired function it creates or
// replaces the function (as the owning role), ensures ownership is correct via
// ALTER FUNCTION, and grants EXECUTE to roleName. Functions are named with a
// role-based prefix (<rolename>__<funcname>) so they can be identified for
// cleanup. If a function's OwningRole is empty, the database owner is used.
// Each DDL operation runs inside a transaction to prevent SET LOCAL ROLE leaking
// into the connection pool on error.
func SyncDatabaseFunctions(log logr.Logger, db *sql.DB, roleName string, functions []CustomRoleFunction) error {
	for _, f := range functions {
		if err := validateFunction(f); err != nil {
			return err
		}
	}

	quotedRole := pq.QuoteIdentifier(roleName)

	// Resolve the database owner once for functions that omit owningRole.
	var dbOwner string
	for _, f := range functions {
		if f.OwningRole == "" {
			var err error
			dbOwner, err = databaseOwner(db)
			if err != nil {
				return err
			}
			break
		}
	}

	// Build a map of currently-managed functions so we can detect ownership
	// changes before attempting CREATE OR REPLACE.
	current, err := managedFunctions(db, roleName)
	if err != nil {
		return err
	}
	// currentOwner maps a function name to its current owner. Only the first
	// overload per name is tracked; args-change overloads are handled by the
	// stale-overload cleanup below.
	currentOwner := make(map[string]string, len(current))
	for _, f := range current {
		if _, seen := currentOwner[f.name]; !seen {
			currentOwner[f.name] = f.owner
		}
	}

	desired := make(map[desiredFunctionKey]struct{})
	for _, f := range functions {
		owner := f.OwningRole
		if owner == "" {
			owner = dbOwner
		}

		pgName := managedFunctionName(roleName, f.Name)
		qualifiedName := fmt.Sprintf("public.%s(%s)", pq.QuoteIdentifier(pgName), f.Args)
		tag := randomDollarTag()

		// If the existing function is owned by a different role, drop it first so
		// that CREATE OR REPLACE (which requires owning the function) can succeed.
		if prev, exists := currentOwner[pgName]; exists && prev != owner {
			if err := execWithRole(db, prev, func(tx *sql.Tx) error {
				_, err := tx.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS public.%s(%s)",
					pq.QuoteIdentifier(pgName), f.Args))
				return err
			}); err != nil {
				return fmt.Errorf("drop function %s before owner change: %w", qualifiedName, err)
			}
		}

		if err := execWithRole(db, owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf(
				"CREATE OR REPLACE FUNCTION public.%s(%s) RETURNS %s LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS %s\nBEGIN\n%s\nEND;\n%s",
				pq.QuoteIdentifier(pgName), f.Args, f.Returns, tag, f.Body, tag))
			return err
		}); err != nil {
			return fmt.Errorf("create function %s as %s: %w", qualifiedName, owner, err)
		}
		log.Info("Created/replaced function", "function", qualifiedName, "owner", owner)

		// Query canonical args from pg_catalog so comparisons match the format
		// that managedFunctions returns.
		var canonicalArgs string
		if err := db.QueryRow(`
			SELECT pg_get_function_identity_arguments(p.oid)
			FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public' AND p.proname = $1
			ORDER BY p.oid DESC LIMIT 1`, pgName).Scan(&canonicalArgs); err != nil {
			return fmt.Errorf("lookup identity args for %s: %w", pgName, err)
		}

		// ALTER FUNCTION sets ownership idempotently so that a changed owningRole
		// takes effect even when CREATE OR REPLACE does not transfer ownership.
		if _, err := db.Exec(fmt.Sprintf("ALTER FUNCTION public.%s(%s) OWNER TO %s",
			pq.QuoteIdentifier(pgName), canonicalArgs, pq.QuoteIdentifier(owner))); err != nil {
			return fmt.Errorf("set owner on function %s: %w", qualifiedName, err)
		}

		if err := execWithRole(db, owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("GRANT EXECUTE ON FUNCTION public.%s(%s) TO %s",
				pq.QuoteIdentifier(pgName), canonicalArgs, quotedRole))
			return err
		}); err != nil {
			return fmt.Errorf("grant execute on %s to %s: %w", qualifiedName, roleName, err)
		}
		log.Info("Granted EXECUTE", "function", qualifiedName, "role", roleName)

		desired[desiredFunctionKey{pgName, canonicalArgs}] = struct{}{}
	}

	// Drop functions that are managed but no longer desired (including stale
	// overloads whose arg signature changed).
	for _, f := range current {
		if _, ok := desired[desiredFunctionKey{f.name, f.identityArgs}]; !ok {
			if err := execWithRole(db, f.owner, func(tx *sql.Tx) error {
				_, err := tx.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS public.%s(%s)",
					pq.QuoteIdentifier(f.name), f.identityArgs))
				return err
			}); err != nil {
				return fmt.Errorf("drop function public.%s(%s): %w", f.name, f.identityArgs, err)
			}
			log.Info("Dropped managed function", "function", f.name, "args", f.identityArgs)
		}
	}

	return nil
}

// DropManagedFunctions drops all functions in the currently-connected database
// whose name starts with the managed prefix for roleName. Each drop runs inside
// a transaction with SET LOCAL ROLE to the function owner. Used during CR
// deletion cleanup.
func DropManagedFunctions(log logr.Logger, db *sql.DB, roleName string) error {
	funcs, err := managedFunctions(db, roleName)
	if err != nil {
		return err
	}
	for _, f := range funcs {
		if err := execWithRole(db, f.owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS public.%s(%s)",
				pq.QuoteIdentifier(f.name), f.identityArgs))
			return err
		}); err != nil {
			return fmt.Errorf("drop function public.%s(%s): %w", f.name, f.identityArgs, err)
		}
		log.Info("Dropped managed function", "function", f.name, "args", f.identityArgs)
	}
	return nil
}

// validateFunction checks that a CustomRoleFunction has the required fields.
func validateFunction(f CustomRoleFunction) error {
	if f.Name == "" {
		return ctlerrors.NewInvalid(fmt.Errorf("function name must not be empty"))
	}
	// "__" is the separator between the role prefix and the function name in the
	// generated PostgreSQL identifier. Allowing it in the user-supplied name would
	// break the cleanup query that uses the absence of "__" after the prefix to
	// distinguish functions owned by this role from those of longer-prefixed roles.
	if strings.Contains(f.Name, "__") {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: name must not contain \"__\"", f.Name))
	}
	if f.Returns == "" {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: returns must not be empty", f.Name))
	}
	if f.Body == "" {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: body must not be empty", f.Name))
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
