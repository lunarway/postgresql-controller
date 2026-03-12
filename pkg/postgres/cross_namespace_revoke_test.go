package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/test"
)

// TestRolesDiff_CrossNamespaceRevocation demonstrates the bug where two
// PostgreSQLUser resources in different namespaces pointing to the same
// PostgreSQL host cause each reconciliation to revoke roles granted by the
// other namespace.
//
// Scenario:
//   - alice in "team-a" namespace: allDatabases on shared-db.example.local.
//   - alice in "team-b" namespace: allDatabases on shared-db.example.local.
//
// When "team-a" reconciles, it only knows about team-a databases and revokes
// roles from team-b databases. When "team-b" reconciles next, it revokes
// team-a roles. The user sees intermittent access loss.
func TestRolesDiff_CrossNamespaceRevocation(t *testing.T) {
	logger := test.NewLogger(t)

	// Simulate initial state: both namespaces have been reconciled once.
	// The PostgreSQL user has roles from both team-a and team-b namespaces.
	existingRoles := []string{
		"app_role",
		"orders_read",   // from team-a namespace
		"invoices_read", // from team-a namespace
		"reports_read",  // from team-b namespace
		"billing_read",  // from team-b namespace
	}

	// --- Reconciliation from "team-a" namespace ---
	// team-a only knows about its own databases: orders, invoices.
	teamADatabases := []DatabaseSchema{
		{Name: "orders", Schema: "orders", Privileges: PrivilegeRead},
		{Name: "invoices", Schema: "invoices", Privileges: PrivilegeRead},
	}
	staticRoles := []string{"app_role"}

	addable, removeable := rolesDiff(logger, existingRoles, staticRoles, teamADatabases)

	// BUG: team-a reconciliation revokes team-b roles because they are not
	// in team-a's expected list.
	assert.Nil(t, addable, "no roles should be added")
	assert.Equal(t, []string{"reports_read", "billing_read"}, removeable,
		"BUG REPRODUCED: team-a namespace reconciliation revokes team-b namespace roles")

	// --- After team-a reconciliation, the user only has team-a roles ---
	rolesAfterTeamAReconcile := []string{
		"app_role",
		"orders_read",
		"invoices_read",
	}

	// --- Reconciliation from "team-b" namespace ---
	// team-b only knows about its own databases: reports, billing.
	teamBDatabases := []DatabaseSchema{
		{Name: "reports", Schema: "reports", Privileges: PrivilegeRead},
		{Name: "billing", Schema: "billing", Privileges: PrivilegeRead},
	}

	addable2, removeable2 := rolesDiff(logger, rolesAfterTeamAReconcile, staticRoles, teamBDatabases)

	// BUG: team-b reconciliation now revokes team-a roles!
	assert.Equal(t, []string{"reports_read", "billing_read"}, addable2,
		"team-b roles should be re-added")
	assert.Equal(t, []string{"orders_read", "invoices_read"}, removeable2,
		"BUG REPRODUCED: team-b namespace reconciliation revokes team-a namespace roles")
}

// TestRolesDiff_CrossNamespaceRevocationCycle demonstrates a full
// grant/revoke cycle showing how roles oscillate between namespaces across
// reconciliation loops.
func TestRolesDiff_CrossNamespaceRevocationCycle(t *testing.T) {
	logger := test.NewLogger(t)
	staticRoles := []string{"app_role"}

	teamADatabases := []DatabaseSchema{
		{Name: "orders", Schema: "orders", Privileges: PrivilegeRead},
	}
	teamBDatabases := []DatabaseSchema{
		{Name: "reports", Schema: "reports", Privileges: PrivilegeRead},
	}

	// Start: user has no roles.
	existingRoles := []string{}

	// Cycle 1: team-a reconciles first.
	addable, removeable := rolesDiff(logger, existingRoles, staticRoles, teamADatabases)
	assert.Equal(t, []string{"app_role", "orders_read"}, addable)
	assert.Nil(t, removeable)

	// Apply: user now has app_role + orders_read.
	existingRoles = []string{"app_role", "orders_read"}

	// Cycle 1: team-b reconciles second.
	addable, removeable = rolesDiff(logger, existingRoles, staticRoles, teamBDatabases)
	assert.Equal(t, []string{"reports_read"}, addable)
	// BUG: orders_read is revoked!
	assert.Equal(t, []string{"orders_read"}, removeable,
		"BUG: team-b revokes orders_read granted by team-a namespace")

	// Apply: user now has app_role + reports_read (orders_read LOST).
	existingRoles = []string{"app_role", "reports_read"}

	// Cycle 2: team-a reconciles again.
	addable, removeable = rolesDiff(logger, existingRoles, staticRoles, teamADatabases)
	assert.Equal(t, []string{"orders_read"}, addable)
	// BUG: reports_read is revoked!
	assert.Equal(t, []string{"reports_read"}, removeable,
		"BUG: team-a revokes reports_read granted by team-b namespace")

	// The user NEVER has both orders_read AND reports_read at the same time
	// after the initial reconciliation. Access oscillates depending on which
	// namespace reconciles last.
}
