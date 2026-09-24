package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

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
