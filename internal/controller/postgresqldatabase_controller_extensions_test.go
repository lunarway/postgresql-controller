package controller

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestPostgreSQLDatabase_Reconcile_globalExtensions verifies that global extensions
// are merged with database-specific extensions and actually installed on PostgreSQL.
func TestPostgreSQLDatabase_Reconcile_globalExtensions(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch        = time.Now().UnixNano()
		namespace    = "default"
		databaseName = fmt.Sprintf("database_ext_%d", epoch)
		managerRole  = "postgres_role_manager"
	)

	// Create the schema for our tests
	s := runtime.NewScheme()
	err := scheme.AddToScheme(s)
	require.NoError(t, err, "add scheme failed")
	err = lunarwayv1alpha1.AddToScheme(s)
	require.NoError(t, err, "add lunar scheme failed")

	tests := []struct {
		name               string
		globalExtensions   []string
		databaseExtensions []lunarwayv1alpha1.PostgreSQLDatabaseExtension
		expectedExtensions []string
	}{
		{
			name:               "only global extensions",
			globalExtensions:   []string{"pgcrypto", "uuid-ossp"},
			databaseExtensions: nil,
			expectedExtensions: []string{"pgcrypto", "uuid-ossp"},
		},
		{
			name:             "only database extensions",
			globalExtensions: nil,
			databaseExtensions: []lunarwayv1alpha1.PostgreSQLDatabaseExtension{
				{ExtensionName: "pg_trgm"},
			},
			expectedExtensions: []string{"pg_trgm"},
		},
		{
			name:             "both global and database extensions",
			globalExtensions: []string{"pgcrypto", "uuid-ossp"},
			databaseExtensions: []lunarwayv1alpha1.PostgreSQLDatabaseExtension{
				{ExtensionName: "pg_trgm"},
			},
			expectedExtensions: []string{"pg_trgm", "pgcrypto", "uuid-ossp"},
		},
		{
			name:             "duplicate extension deduplicated",
			globalExtensions: []string{"pgcrypto", "uuid-ossp"},
			databaseExtensions: []lunarwayv1alpha1.PostgreSQLDatabaseExtension{
				{ExtensionName: "pgcrypto"},
			},
			expectedExtensions: []string{"pgcrypto", "uuid-ossp"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use a unique database name for each test case
			dbName := fmt.Sprintf("%s_%d", databaseName, time.Now().UnixNano())

			databaseResource := &lunarwayv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbName,
					Namespace: namespace,
				},
				Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
					Host: lunarwayv1alpha1.ResourceVar{
						Value: "localhost",
					},
					Name: dbName,
					User: lunarwayv1alpha1.ResourceVar{
						Value: dbName,
					},
					Password: &lunarwayv1alpha1.ResourceVar{
						Value: "test-password",
					},
					Extensions: tc.databaseExtensions,
				},
			}

			objs := []runtime.Object{databaseResource}
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).WithStatusSubresource(databaseResource).Build()

			// Create a controller with global extensions configured
			hostCredentials := map[string]postgres.Credentials{
				"localhost": {
					User:     "iam_creator",
					Password: "iam_creator",
				},
			}

			r := &PostgreSQLDatabaseReconciler{
				Client:            cl,
				Log:               ctrl.Log.WithName(t.Name()),
				HostCredentials:   hostCredentials,
				ManagerRoleName:   managerRole,
				SuperuserRoleName: "iam_creator",
				GlobalExtensions:  tc.globalExtensions,
			}

			// Seed the database
			seededDatabase(t, host, dbName, dbName, managerRole)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dbName,
					Namespace: namespace,
				},
			}

			// Reconcile to install extensions
			_, err := r.Reconcile(context.Background(), req)
			require.NoError(t, err, "reconcile failed")

			// Verify extensions were actually installed
			conn, err := postgres.Connect(postgres.ConnectionString{
				Database: dbName,
				Host:     host,
				Password: "test-password",
				User:     dbName,
			})
			require.NoError(t, err, "failed to connect to database")
			defer conn.Close()

			installedExtensions := getInstalledExtensionsFromDB(t, conn)

			// Check that exactly the expected extensions are installed
			assert.ElementsMatch(t, tc.expectedExtensions, installedExtensions, "installed extensions do not match expected")
		})
	}
}

// TestPostgreSQLDatabase_Reconcile_extensionsAlreadyInstalled verifies that reconciliation
// handles already-installed extensions correctly and doesn't fail when extensions exist.
func TestPostgreSQLDatabase_Reconcile_extensionsAlreadyInstalled(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch        = time.Now().UnixNano()
		namespace    = "default"
		databaseName = fmt.Sprintf("database_ext_preinstalled_%d", epoch)
		managerRole  = "postgres_role_manager"
	)

	// Create the schema for our tests
	s := runtime.NewScheme()
	err := scheme.AddToScheme(s)
	require.NoError(t, err, "add scheme failed")
	err = lunarwayv1alpha1.AddToScheme(s)
	require.NoError(t, err, "add lunar scheme failed")

	// Test case: some extensions are already installed before reconciliation
	t.Run("some extensions already installed", func(t *testing.T) {
		dbName := fmt.Sprintf("%s_%d", databaseName, time.Now().UnixNano())

		// Seed the database
		seededDatabase(t, host, dbName, dbName, managerRole)

		// Pre-install pgcrypto extension before reconciliation
		conn, err := postgres.Connect(postgres.ConnectionString{
			Database: dbName,
			Host:     host,
			Password: "iam_creator",
			User:     "iam_creator",
		})
		require.NoError(t, err, "failed to connect to database")
		_, err = conn.Exec(fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA %s", dbName))
		require.NoError(t, err, "failed to pre-install pgcrypto")
		conn.Close()

		databaseResource := &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dbName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Host: lunarwayv1alpha1.ResourceVar{
					Value: "localhost",
				},
				Name: dbName,
				User: lunarwayv1alpha1.ResourceVar{
					Value: dbName,
				},
				Password: &lunarwayv1alpha1.ResourceVar{
					Value: "test-password",
				},
				Extensions: []lunarwayv1alpha1.PostgreSQLDatabaseExtension{
					{ExtensionName: "pg_trgm"},
				},
			},
		}

		objs := []runtime.Object{databaseResource}
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).WithStatusSubresource(databaseResource).Build()

		hostCredentials := map[string]postgres.Credentials{
			"localhost": {
				User:     "iam_creator",
				Password: "iam_creator",
			},
		}

		r := &PostgreSQLDatabaseReconciler{
			Client:            cl,
			Log:               ctrl.Log.WithName(t.Name()),
			HostCredentials:   hostCredentials,
			ManagerRoleName:   managerRole,
			SuperuserRoleName: "iam_creator",
			GlobalExtensions:  []string{"pgcrypto", "uuid-ossp"},
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      dbName,
				Namespace: namespace,
			},
		}

		// Reconcile - should not fail even though pgcrypto is already installed
		_, err = r.Reconcile(context.Background(), req)
		require.NoError(t, err, "reconcile should not fail with pre-installed extensions")

		// Verify all extensions are installed
		conn, err = postgres.Connect(postgres.ConnectionString{
			Database: dbName,
			Host:     host,
			Password: "test-password",
			User:     dbName,
		})
		require.NoError(t, err, "failed to connect to database")
		defer conn.Close()

		installedExtensions := getInstalledExtensionsFromDB(t, conn)
		expectedExtensions := []string{"pgcrypto", "uuid-ossp", "pg_trgm"}

		assert.ElementsMatch(t, expectedExtensions, installedExtensions, "all extensions should be installed")
	})
}

// getInstalledExtensionsFromDB queries PostgreSQL for installed extensions
func getInstalledExtensionsFromDB(t *testing.T, conn *sql.DB) []string {
	t.Helper()

	rows, err := conn.Query("SELECT extname FROM pg_extension WHERE extname != 'plpgsql'")
	require.NoError(t, err, "failed to query extensions")
	defer rows.Close()

	var extensions []string
	for rows.Next() {
		var extname string
		err = rows.Scan(&extname)
		require.NoError(t, err, "failed to scan extension name")
		extensions = append(extensions, extname)
	}

	require.NoError(t, rows.Err(), "error iterating rows")
	return extensions
}
