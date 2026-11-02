package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

// EnsureIAMLoginRole creates a PostgreSQL role with LOGIN capability if it does
// not already exist, then synchronises its granted server-level roles to exactly
// match grantRoles (revokes extras, grants missing ones).
//
// Unlike EnsureCustomRole (which uses NOLOGIN), this targets RDS IAM
// authentication where the role must be able to log in. Callers should include
// "rds_iam" in grantRoles to enable IAM token-based authentication.
func EnsureIAMLoginRole(log logr.Logger, db *sql.DB, roleName string, grantRoles []string) error {
	log = log.WithValues("role", roleName)
	log.Info("Ensuring IAM login role")

	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN", pq.QuoteIdentifier(roleName)))
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
			if _, err := db.Exec(fmt.Sprintf("REVOKE %s FROM %s", pq.QuoteIdentifier(r), pq.QuoteIdentifier(roleName))); err != nil {
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
	quoted := make([]string, len(toGrant))
	for i, r := range toGrant {
		quoted[i] = pq.QuoteIdentifier(r)
	}
	if _, err := db.Exec(fmt.Sprintf("GRANT %s TO %s", strings.Join(quoted, ", "), pq.QuoteIdentifier(roleName))); err != nil {
		return fmt.Errorf("grant roles %v to %s: %w", toGrant, roleName, err)
	}
	log.Info("Granted roles", "roles", toGrant)
	return nil
}
