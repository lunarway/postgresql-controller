package postgres

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// ReservedSystemDatabases lists the PostgreSQL/RDS-internal databases that the
// controller must never touch, even when the user explicitly names them in
// spec.databases. Note that "postgres" is intentionally absent: it is a valid
// explicit target (functions live there) and is handled as a special case in
// the reconcilers rather than being filtered out here.
var ReservedSystemDatabases = map[string]struct{}{
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
