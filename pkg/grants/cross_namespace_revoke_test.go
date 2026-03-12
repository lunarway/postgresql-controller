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
// where the same user has PostgreSQLUser resources in multiple namespaces, and
// two of them (team-a and team-b) point to the same PostgreSQL host
// (shared-db.example.local.).
//
// Each namespace's reconciliation independently computes desired roles from
// only its own databases. When rolesDiff runs, it revokes any existing roles
// not in the expected list — including roles granted by the other namespace.
func TestCrossNamespaceRevocation_KrviScenario(t *testing.T) {
	sharedHost := "shared-db.example.local."

	// Databases known in the "team-a" namespace on the shared host.
	teamADatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "orders"),
		runningDatabase(sharedHost, "invoices"),
	}

	// Databases known in the "team-b" namespace on the shared host.
	teamBDatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "reports"),
		runningDatabase(sharedHost, "billing"),
	}

	// --- Step 1: groupAccesses for team-a namespace ---
	teamAGranter := newTestGranter(teamADatabases)
	teamAAccesses, err := teamAGranter.groupAccesses(
		test.NewLogger(t), "team-a",
		[]lunarwayv1alpha1.AccessSpec{
			{
				Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
				AllDatabases: &trueValue,
				Reason:       "team-a service account needs read access",
			},
		},
		nil,
	)
	require.NoError(t, err)

	teamASchemas := databaseSchemas(teamAAccesses[sharedHost])
	teamARoleNames := schemaRoleNames(teamASchemas)
	assert.ElementsMatch(t, []string{"orders_read", "invoices_read"}, teamARoleNames)

	// --- Step 2: groupAccesses for team-b namespace ---
	teamBGranter := newTestGranter(teamBDatabases)
	teamBAccesses, err := teamBGranter.groupAccesses(
		test.NewLogger(t), "team-b",
		[]lunarwayv1alpha1.AccessSpec{
			{
				Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
				AllDatabases: &trueValue,
				Reason:       "team-b service account needs read access",
			},
		},
		nil,
	)
	require.NoError(t, err)

	teamBSchemas := databaseSchemas(teamBAccesses[sharedHost])
	teamBRoleNames := schemaRoleNames(teamBSchemas)
	assert.ElementsMatch(t, []string{"reports_read", "billing_read"}, teamBRoleNames)

	// --- Step 3: Demonstrate the conflict ---
	// The two namespaces produce completely disjoint role sets for the same
	// host and the same user. When reconciled independently, each namespace's
	// rolesDiff call will revoke the other's roles because they are not in its
	// expected list.
	//
	// This is the root cause: SyncUser processes one PostgreSQLUser at a time
	// and has no awareness of other PostgreSQLUser resources for the same
	// username on the same host in different namespaces.
	for _, teamARole := range teamARoleNames {
		assert.NotContains(t, teamBRoleNames, teamARole,
			"BUG CONFIRMED: team-a role %q is not in team-b's expected list and WILL be revoked when team-b reconciles", teamARole)
	}
	for _, teamBRole := range teamBRoleNames {
		assert.NotContains(t, teamARoleNames, teamBRole,
			"BUG CONFIRMED: team-b role %q is not in team-a's expected list and WILL be revoked when team-a reconciles", teamBRole)
	}
}

// TestCrossNamespaceRevocation_SameNamespaceSafe verifies that within a single
// namespace, multiple entries on the same host do NOT cause revocations because
// all entries are grouped together into one databaseSchemas slice.
func TestCrossNamespaceRevocation_SameNamespaceSafe(t *testing.T) {
	host := "db.host.example.com"

	databases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(host, "users"),
		runningDatabase(host, "products"),
		runningDatabase(host, "analytics"),
	}

	// User has allDatabases AND a specific database entry on the same host.
	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			AllDatabases: &trueValue,
			Reason:       "service account needs read access to all databases",
		},
		{
			Host:     lunarwayv1alpha1.ResourceVar{Value: host},
			Database: lunarwayv1alpha1.ResourceVar{Value: "users"},
			Schema:   lunarwayv1alpha1.ResourceVar{Value: "users"},
			Reason:   "explicit entry for users database",
		},
	}

	granter := newTestGranter(databases)
	accesses, err := granter.groupAccesses(test.NewLogger(t), "team-a", reads, nil)
	require.NoError(t, err)

	schemas := databaseSchemas(accesses[host])
	roleNames := schemaRoleNames(schemas)

	// All databases should be present. The duplicate from the specific entry
	// is harmless — rolesDiff deduplicates via contains().
	assert.Contains(t, roleNames, "users_read")
	assert.Contains(t, roleNames, "products_read")
	assert.Contains(t, roleNames, "analytics_read")
}

// TestCrossNamespaceRevocation_ExpiredWriteOnlyRevokesWrite verifies that when
// a time-bound write entry expires, only the write role is removed — read
// access from allDatabases is unaffected. This is correct single-namespace
// behavior.
func TestCrossNamespaceRevocation_ExpiredWriteOnlyRevokesWrite(t *testing.T) {
	host := "db.host.example.com"
	now := time.Date(2025, 10, 26, 0, 0, 0, 0, time.UTC)

	databases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(host, "orders"),
		runningDatabase(host, "products"),
	}

	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			AllDatabases: &trueValue,
			Reason:       "service account needs read access",
		},
	}

	// This write entry has expired.
	startTime := metav1.NewTime(time.Date(2025, 10, 25, 10, 0, 0, 0, time.UTC))
	stopTime := metav1.NewTime(time.Date(2025, 10, 25, 15, 0, 0, 0, time.UTC))
	writes := []lunarwayv1alpha1.WriteAccessSpec{
		{
			AccessSpec: lunarwayv1alpha1.AccessSpec{
				Host:     lunarwayv1alpha1.ResourceVar{Value: host},
				Database: lunarwayv1alpha1.ResourceVar{Value: "orders"},
				Schema:   lunarwayv1alpha1.ResourceVar{Value: "orders"},
				Reason:   "temporary write access",
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

	accesses, err := granter.groupAccesses(test.NewLogger(t), "team-a", reads, writes)
	require.NoError(t, err)

	schemas := databaseSchemas(accesses[host])
	roleNames := schemaRoleNames(schemas)

	// The expired write should NOT be included.
	assert.NotContains(t, roleNames, "orders_readwrite",
		"expired write should not be in expected roles")

	// Read access should still be present for all databases.
	assert.Contains(t, roleNames, "orders_read")
	assert.Contains(t, roleNames, "products_read")
}

// TestCrossNamespaceRevocation_MergeFixPreventsRevocation verifies that
// mergeSiblingAccesses expands the expected role set to include roles from
// sibling namespaces, preventing cross-namespace revocations.
func TestCrossNamespaceRevocation_MergeFixPreventsRevocation(t *testing.T) {
	sharedHost := "shared-db.example.local."

	teamADatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "orders"),
		runningDatabase(sharedHost, "invoices"),
	}
	teamBDatabases := []lunarwayv1alpha1.PostgreSQLDatabase{
		runningDatabase(sharedHost, "reports"),
		runningDatabase(sharedHost, "billing"),
	}

	teamAUser := lunarwayv1alpha1.PostgreSQLUser{
		ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "team-a"},
		Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
			Name: "alice",
			Read: &[]lunarwayv1alpha1.AccessSpec{
				{
					Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
					AllDatabases: &trueValue,
					Reason:       "team-a service account needs read access",
				},
			},
		},
	}
	teamBUser := lunarwayv1alpha1.PostgreSQLUser{
		ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "team-b"},
		Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
			Name: "alice",
			Read: &[]lunarwayv1alpha1.AccessSpec{
				{
					Host:         lunarwayv1alpha1.ResourceVar{Value: sharedHost},
					AllDatabases: &trueValue,
					Reason:       "team-b service account needs read access",
				},
			},
		},
	}

	allUsers := []lunarwayv1alpha1.PostgreSQLUser{teamAUser, teamBUser}

	// Build a granter that knows about all databases across namespaces.
	granter := Granter{
		Now:                     time.Now,
		AllDatabasesReadEnabled: true,
		AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
			switch namespace {
			case "team-a":
				return teamADatabases, nil
			case "team-b":
				return teamBDatabases, nil
			default:
				return append(teamADatabases, teamBDatabases...), nil
			}
		},
		AllUsers: func() ([]lunarwayv1alpha1.PostgreSQLUser, error) {
			return allUsers, nil
		},
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, nil
		},
	}

	// Compute accesses for the team-a namespace.
	teamAAccesses, err := granter.groupAccesses(
		test.NewLogger(t), "team-a",
		*teamAUser.Spec.Read,
		nil,
	)
	require.NoError(t, err)

	// Apply the merge fix.
	granter.mergeSiblingAccesses(test.NewLogger(t), "team-a", "alice", teamAAccesses)

	mergedSchemas := databaseSchemas(teamAAccesses[sharedHost])
	mergedRoleNames := schemaRoleNames(mergedSchemas)

	// After merging, all 4 databases must be present so rolesDiff won't revoke any of them.
	assert.ElementsMatch(t,
		[]string{"orders_read", "invoices_read", "reports_read", "billing_read"},
		mergedRoleNames,
		"merged accesses must contain roles from both namespaces to prevent revocation",
	)
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
