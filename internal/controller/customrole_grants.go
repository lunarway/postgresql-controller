package controller

import (
	"fmt"

	"github.com/go-logr/logr"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

// reconcileGrantsOnHost applies grants to targeted user databases and cleans
// up grants in any database that is no longer in scope.
// allUserDatabases is non-nil only when targetDatabases was explicitly set,
// in which case it contains every user database for the cleanup pass.
func (r *CustomRoleReconciler) reconcileGrantsOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string, databases, allUserDatabases []string, grants []postgres.CustomRoleGrant) error {
	// Apply grants to targeted user databases. Postgres is skipped because
	// grants are never applied there.
	for _, dbName := range databases {
		if dbName == "postgres" {
			continue
		}
		if err := r.syncGrantsOnDatabase(log, host, creds, roleName, dbName, grants); err != nil {
			return fmt.Errorf("sync grants on database %s: %w", dbName, err)
		}
	}

	if allUserDatabases == nil {
		return nil
	}

	// Clean up grants in user databases that are no longer targeted.
	targetSet := make(map[string]struct{}, len(databases))
	for _, db := range databases {
		targetSet[db] = struct{}{}
	}
	for _, dbName := range allUserDatabases {
		if _, inTarget := targetSet[dbName]; inTarget {
			continue
		}
		if err := r.syncGrantsOnDatabase(log, host, creds, roleName, dbName, nil); err != nil {
			return fmt.Errorf("cleanup grants on database %s: %w", dbName, err)
		}
	}
	return nil
}

func (r *CustomRoleReconciler) syncGrantsOnDatabase(log logr.Logger, host string, adminCredentials postgres.Credentials, roleName, dbName string, grants []postgres.CustomRoleGrant) error {
	connStr := postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	db, err := postgres.Connect(log, connStr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", connStr, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error(err, "failed to close database connection", "database", dbName)
		}
	}()

	return postgres.SyncDatabaseGrants(log, db, roleName, grants)
}

func toPostgresGrants(grants []postgresqlv1alpha1.CustomRoleGrant) []postgres.CustomRoleGrant {
	result := make([]postgres.CustomRoleGrant, len(grants))
	for i, g := range grants {
		result[i] = postgres.CustomRoleGrant{
			Schema:     g.Schema,
			Table:      g.Table,
			Privileges: g.Privileges,
		}
	}
	return result
}
