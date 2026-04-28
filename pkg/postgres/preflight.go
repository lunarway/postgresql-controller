package postgres

import (
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"
)

// Preflight verifies the invariants the controller relies on for every
// reconciliation cycle. It returns a descriptive error naming the violated
// assumption so operators can fix the underlying setup.
//
// Performed in order:
//   - The database connection is alive.
//   - The connecting user is a member of superuserRole. The role is
//     expected to exist on the server.
func Preflight(log logr.Logger, db *sql.DB, superuserRole string) error {
	if superuserRole == "" {
		return fmt.Errorf("preflight: superuser role name is empty (configure --superuser-role-name)")
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("preflight: ping database: %w", err)
	}

	if err := assertSuperuserRoleMembership(db, superuserRole); err != nil {
		return err
	}

	log.V(1).Info("Preflight checks passed", "superuserRole", superuserRole)
	return nil
}

func assertSuperuserRoleMembership(db *sql.DB, superuserRole string) error {
	var (
		currentUser string
		isMember    bool
	)
	err := db.QueryRow(`
		SELECT
			current_user,
			EXISTS(
				SELECT 1
				FROM pg_roles r
				WHERE r.rolname = $1
				  AND pg_has_role(current_user, r.oid, 'MEMBER')
			)
	`, superuserRole).Scan(&currentUser, &isMember)
	if err != nil {
		return fmt.Errorf("preflight: query connecting user privileges: %w", err)
	}
	if isMember {
		return nil
	}
	return fmt.Errorf("preflight: connecting user %q is not a member of %s - grant %s to it before running the controller", currentUser, superuserRole, superuserRole)
}
