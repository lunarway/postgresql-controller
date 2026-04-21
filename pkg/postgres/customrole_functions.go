package postgres

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"

	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
)

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

// isSafeArgs reports whether s is safe to interpolate as the argument list of a
// CREATE FUNCTION statement. A closing parenthesis at depth 0 would escape the
// argument list, enabling injection. Statement terminators and comment markers
// are also rejected.
func isSafeArgs(s string) bool {
	if strings.ContainsAny(s, ";'") || strings.Contains(s, "--") || strings.Contains(s, "/*") {
		return false
	}
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return depth == 0
}

// isSafeReturns reports whether s is safe to interpolate as the RETURNS type
// expression of a CREATE FUNCTION statement. Spaces outside balanced
// parentheses could inject extra function options (e.g. "SET search_path TO
// public" would override the hardcoded search_path). Only a leading "SETOF "
// prefix is permitted to carry a space outside parentheses.
func isSafeReturns(s string) bool {
	if strings.ContainsAny(s, ";'") || strings.Contains(s, "--") || strings.Contains(s, "/*") {
		return false
	}
	remaining := s
	if strings.HasPrefix(strings.ToUpper(remaining), "SETOF ") {
		remaining = remaining[6:]
	}
	depth := 0
	for _, r := range remaining {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return false
			}
			depth--
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				return false
			}
		}
	}
	return depth == 0
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
		// Run as owner (not the connection user) — PostgreSQL requires the caller
		// to be the current owner or a superuser.
		if err := execWithRole(db, owner, func(tx *sql.Tx) error {
			_, err := tx.Exec(fmt.Sprintf("ALTER FUNCTION public.%s(%s) OWNER TO %s",
				pq.QuoteIdentifier(pgName), canonicalArgs, pq.QuoteIdentifier(owner)))
			return err
		}); err != nil {
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
	if !isSafeArgs(f.Args) {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: args contains unsafe SQL characters or unbalanced parentheses", f.Name))
	}
	if !isSafeReturns(f.Returns) {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: returns contains unsafe SQL characters or spaces outside parentheses", f.Name))
	}
	if f.Body == "" {
		return ctlerrors.NewInvalid(fmt.Errorf("function %q: body must not be empty", f.Name))
	}
	return nil
}
