package controller

import (
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

// reconcileFunctionsOnHost applies functions to targeted databases (including
// the postgres database when explicitly listed) and cleans up functions in any
// database that is no longer in scope.
// allUserDatabases is non-nil only when targetDatabases was explicitly set.
// When nil (all-databases mode), the postgres database is always cleaned up
// because it is never included in the auto-discovered user database list.
func (r *CustomRoleReconciler) reconcileFunctionsOnHost(log logr.Logger, host string, creds postgres.Credentials, adminDB *sql.DB, roleName string, databases, allUserDatabases []string, functions []postgres.CustomRoleFunction) error {
	if allUserDatabases == nil {
		// All-databases mode: postgres was never auto-targeted, so clean it up
		// in case spec.databases previously included it.
		if err := postgres.SyncDatabaseFunctions(log, adminDB, roleName, nil); err != nil {
			return fmt.Errorf("cleanup functions on database postgres: %w", err)
		}
	}

	for _, dbName := range databases {
		if dbName == "postgres" {
			// Reuse the admin connection for the postgres database.
			if err := postgres.SyncDatabaseFunctions(log, adminDB, roleName, functions); err != nil {
				return fmt.Errorf("sync functions on database postgres: %w", err)
			}
			continue
		}
		if err := r.syncFunctionsOnDatabase(log, host, creds, roleName, dbName, functions); err != nil {
			return fmt.Errorf("sync functions on database %s: %w", dbName, err)
		}
	}

	if allUserDatabases == nil {
		return nil
	}

	// Clean up functions in databases that are no longer targeted.
	targetSet := make(map[string]struct{}, len(databases))
	for _, db := range databases {
		targetSet[db] = struct{}{}
	}
	if _, ok := targetSet["postgres"]; !ok {
		if err := postgres.SyncDatabaseFunctions(log, adminDB, roleName, nil); err != nil {
			return fmt.Errorf("cleanup functions on database postgres: %w", err)
		}
	}
	for _, dbName := range allUserDatabases {
		if _, inTarget := targetSet[dbName]; inTarget {
			continue
		}
		if err := r.syncFunctionsOnDatabase(log, host, creds, roleName, dbName, nil); err != nil {
			return fmt.Errorf("cleanup functions on database %s: %w", dbName, err)
		}
	}
	return nil
}

func (r *CustomRoleReconciler) syncFunctionsOnDatabase(log logr.Logger, host string, adminCredentials postgres.Credentials, roleName, dbName string, functions []postgres.CustomRoleFunction) error {
	connStr := postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	db, err := postgres.Connect(connStr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", connStr, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error(err, "failed to close database connection", "database", dbName)
		}
	}()

	return postgres.SyncDatabaseFunctions(log, db, roleName, functions)
}

func toPostgresFunctions(functions []postgresqlv1alpha1.CustomRoleFunction) []postgres.CustomRoleFunction {
	result := make([]postgres.CustomRoleFunction, len(functions))
	for i, f := range functions {
		result[i] = postgres.CustomRoleFunction{
			Name:       f.Name,
			Args:       f.Args,
			Returns:    f.Returns,
			OwningRole: f.OwningRole,
			Body:       f.Body,
		}
	}
	return result
}
