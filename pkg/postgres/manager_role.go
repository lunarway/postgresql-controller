package postgres

import (
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"
)

// EnsureManagerRole creates the management role on db if it does not
// already exist. The connecting user must have privileges to create roles.
// Idempotent: a duplicate_object error is treated as a no-op.
func EnsureManagerRole(log logr.Logger, db *sql.DB, role string) error {
	if role == "" {
		return fmt.Errorf("ensure manager role: role name is empty")
	}
	return tryExec(log, db, tryExecReq{
		objectType: "management role",
		errorCode:  "duplicate_object",
		query:      fmt.Sprintf("CREATE ROLE %s", role),
	})
}
