package grants

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestCrossNamespaceRevocation_KrviScenario reproduces the real-world issue
// from krvi.yaml where the same user (krvi) has PostgreSQLUser resources in
// multiple namespaces, and two of them (auth and openbanking) point to the same
// PostgreSQL host (auth-rds.hydra.svc.cluster.local.).
//
// Each namespace's reconciliation independently computes desired roles from
// only its own databases. When rolesDiff runs, it revokes any existing roles
// not in the expected list — including roles granted by the other namespace.
func TestCrossNamespaceRevocation_KrviScenario(t *testing.T) {
	sharedHost := "auth-rds.hydra.svc.cluster.local."

	// Databases known in the "auth" namespace on the shared host.
	authNamespaceDatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "hydra"),
		runningDatabase(sharedHost, "consent"),
	}

	// Databases known in the "openbanking" namespace on the shared host.
	openbankingNamespaceDatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "obie_connect"),
		runningDatabase(sharedHost, "obie_payment"),
	}

	// --- Step 1: groupAccesses for auth namespace ---
	authGranter := newTestGranter(authNamespaceDatabases)
	authAccesses, err := authGranter.groupAccesses(
		test.NewLogger(t), "auth",
		[]lunarwayv1alpha1.AccessSpec{
			{
				Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
				AllDatabases: &trueValue,
				Reason:       "I am a developer in squad Void who maintains Hydra",
			},
		},
		nil,
	)
	require.NoError(t, err)

	authSchemas := databaseSchemas(authAccesses[sharedHost])
	authRoleNames := schemaRoleNames(authSchemas)
	assert.ElementsMatch(t, []string{"hydra_read", "consent_read"}, authRoleNames)

	// --- Step 2: groupAccesses for openbanking namespace ---
	obGranter := newTestGranter(openbankingNamespaceDatabases)
	obAccesses, err := obGranter.groupAccesses(
		test.NewLogger(t), "openbanking",
		[]lunarwayv1alpha1.AccessSpec{
			{
				Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
				AllDatabases: &trueValue,
				Reason:       "I am a developer in squad Void who maintains Hydra",
			},
		},
		nil,
	)
	require.NoError(t, err)

	obSchemas := databaseSchemas(obAccesses[sharedHost])
	obRoleNames := schemaRoleNames(obSchemas)
	assert.ElementsMatch(t, []string{"obie_connect_read", "obie_payment_read"}, obRoleNames)

	// --- Step 3: Demonstrate the conflict ---
	// The two namespaces produce completely disjoint role sets for the same
	// host and the same user. When reconciled independently, each namespace's
	// rolesDiff call will revoke the other's roles because they are not in its
	// expected list.
	//
	// This is the root cause: SyncUser processes one PostgreSQLUser at a time
	// and has no awareness of other PostgreSQLUser resources for the same
	// username on the same host in different namespaces.
	for _, authRole := range authRoleNames {
		assert.NotContains(t, obRoleNames, authRole,
			"BUG CONFIRMED: auth role %q is not in openbanking's expected list and WILL be revoked when openbanking reconciles", authRole)
	}
	for _, obRole := range obRoleNames {
		assert.NotContains(t, authRoleNames, obRole,
			"BUG CONFIRMED: openbanking role %q is not in auth's expected list and WILL be revoked when auth reconciles", obRole)
	}
}

// TestCrossNamespaceRevocation_SameNamespaceSafe verifies that within a single
// namespace, multiple entries on the same host do NOT cause revocations because
// all entries are grouped together into one databaseSchemas slice.
func TestCrossNamespaceRevocation_SameNamespaceSafe(t *testing.T) {
	host := "db.host.example.com"

	databases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(host, "authentication"),
		runningDatabase(host, "mitid"),
		runningDatabase(host, "fortnox"),
	}

	// User has allDatabases AND specific database entries on the same host.
	// This mirrors the prod namespace from krvi.yaml.
	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			AllDatabases: &trueValue,
			Reason:       "I am a developer in squad Void",
		},
		{
			Host:     lunarwayv1alpha1.ResourceVar{Value: host},
			Database: lunarwayv1alpha1.ResourceVar{Value: "authentication"},
			Schema:   lunarwayv1alpha1.ResourceVar{Value: "authentication"},
			Reason:   "I am a developer in squad Void",
		},
	}

	granter := newTestGranter(databases)
	accesses, err := granter.groupAccesses(test.NewLogger(t), "prod", reads, nil)
	require.NoError(t, err)

	schemas := databaseSchemas(accesses[host])
	roleNames := schemaRoleNames(schemas)

	// All databases should be present. The duplicate from the specific entry
	// is harmless — rolesDiff deduplicates via contains().
	assert.Contains(t, roleNames, "authentication_read")
	assert.Contains(t, roleNames, "mitid_read")
	assert.Contains(t, roleNames, "fortnox_read")
}

// TestCrossNamespaceRevocation_ExpiredWriteOnlyRevokesWrite verifies that when
// a time-bound write entry expires, only the write role is removed — read
// access from allDatabases is unaffected. This is correct single-namespace
// behavior.
func TestCrossNamespaceRevocation_ExpiredWriteOnlyRevokesWrite(t *testing.T) {
	host := "db.host.example.com"
	now := time.Date(2025, 10, 26, 0, 0, 0, 0, time.UTC)

	databases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(host, "fortnox"),
		runningDatabase(host, "other_db"),
	}

	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			AllDatabases: &trueValue,
			Reason:       "I am a developer in squad Void",
		},
	}

	// This write entry has expired.
	startTime := metav1.NewTime(time.Date(2025, 10, 25, 10, 0, 0, 0, time.UTC))
	stopTime := metav1.NewTime(time.Date(2025, 10, 25, 15, 0, 0, 0, time.UTC))
	writes := []lunarwayv1alpha1.WriteAccessSpec{
		{
			AccessSpec: lunarwayv1alpha1.AccessSpec{
				Host:     lunarwayv1alpha1.ResourceVar{Value: host},
				Database: lunarwayv1alpha1.ResourceVar{Value: "fortnox"},
				Schema:   lunarwayv1alpha1.ResourceVar{Value: "fortnox"},
				Reason:   "...",
				Start:    &startTime,
				Stop:     &stopTime,
			},
		},
	}

	granter := Granter{
		Now:                     func() time.Time { return now },
		AllDatabasesReadEnabled: true,
		AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
			return databases, nil
		},
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, nil
		},
	}

	accesses, err := granter.groupAccesses(test.NewLogger(t), "prod", reads, writes)
	require.NoError(t, err)

	schemas := databaseSchemas(accesses[host])
	roleNames := schemaRoleNames(schemas)

	// The expired write should NOT be included.
	assert.NotContains(t, roleNames, "fortnox_readwrite",
		"expired write should not be in expected roles")

	// Read access should still be present for all databases.
	assert.Contains(t, roleNames, "fortnox_read")
	assert.Contains(t, roleNames, "other_db_read")
}

// --- Helpers ---

func runningDatabase(host, name string) lunarwayv1alpha1.PostgreSQLDatabase {
	return lunarwayv1alpha1.PostgreSQLDatabase{
		Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
			Name: name,
			Host: lunarwayv1alpha1.ResourceVar{Value: host},
			User: lunarwayv1alpha1.ResourceVar{}, // empty → fallback to database name as schema
		},
		Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
			Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
		},
	}
}

func newTestGranter(databases []lunarwayv1alpha1.PostgreSQLDatabase) Granter {
	return Granter{
		Now:                     time.Now,
		AllDatabasesReadEnabled: true,
		AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
			return databases, nil
		},
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, nil
		},
	}
}

func schemaRoleNames(schemas []postgres.DatabaseSchema) []string {
	var names []string
	for _, s := range schemas {
		suffix := "read"
		switch s.Privileges {
		case postgres.PrivilegeWrite:
			suffix = "readwrite"
		case postgres.PrivilegeOwningWrite:
			suffix = "readowningwrite"
		}
		schema := s.Schema
		if schema == "public" {
			schema = s.Name
		}
		names = append(names, schema+"_"+suffix)
	}
	return names
}
