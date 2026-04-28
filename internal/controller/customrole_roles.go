package controller

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"

	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

func (r *CustomRoleReconciler) reconcileRoleOnHost(log logr.Logger, adminDB *sql.DB, roleName string, grantRoles []string) error {
	if err := postgres.EnsureCustomRole(log, adminDB, roleName, grantRoles); err != nil {
		return fmt.Errorf("ensure role: %w", err)
	}
	return nil
}

func (r *CustomRoleReconciler) cleanupRole(_ context.Context, log logr.Logger, roleName string) error {
	for host, creds := range r.HostCredentials {
		if err := r.cleanupRoleOnHost(log, host, creds, roleName); err != nil {
			return fmt.Errorf("cleanup on host %s: %w", host, err)
		}
	}
	return nil
}

func (r *CustomRoleReconciler) cleanupRoleOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string) error {
	adminConnStr := postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     creds.User,
		Password: creds.Password,
		Params:   creds.Params,
	}
	adminDB, err := postgres.Connect(adminConnStr)
	if err != nil {
		return fmt.Errorf("connect to host: %w", err)
	}
	defer adminDB.Close()

	databases, err := postgres.UserDatabases(adminDB)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
	}
	for _, dbName := range databases {
		connStr := postgres.ConnectionString{
			Host:     host,
			Database: dbName,
			User:     creds.User,
			Password: creds.Password,
			Params:   creds.Params,
		}
		db, err := postgres.Connect(connStr)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dbName, err)
		}
		dropErr := postgres.DropManagedFunctions(log, db, roleName)
		if dropErr != nil {
			db.Close()
			return fmt.Errorf("drop functions in database %s: %w", dbName, dropErr)
		}
		revokeErr := postgres.RevokeAllDatabaseGrants(log, db, roleName)
		if closeErr := db.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close database connection", "database", dbName)
		}
		if revokeErr != nil {
			return fmt.Errorf("revoke grants in database %s: %w", dbName, revokeErr)
		}
	}

	// Drop functions in the postgres database. Grants are never applied there
	// so there is nothing to revoke.
	if err := postgres.DropManagedFunctions(log, adminDB, roleName); err != nil {
		return fmt.Errorf("drop functions in postgres database: %w", err)
	}

	return postgres.DropCustomRole(log, adminDB, roleName)
}
