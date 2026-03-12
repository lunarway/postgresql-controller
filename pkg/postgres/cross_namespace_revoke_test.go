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
// Real-world scenario from krvi.yaml:
//   - krvi in "auth" namespace: allDatabases on auth-rds.hydra.svc.cluster.local.
//   - krvi in "openbanking" namespace: allDatabases on auth-rds.hydra.svc.cluster.local.
//
// When "auth" reconciles, it only knows about auth-namespace databases and
// revokes roles from openbanking-namespace databases. When "openbanking"
// reconciles next, it revokes auth-namespace roles. The user sees intermittent
// access loss.
func TestRolesDiff_CrossNamespaceRevocation(t *testing.T) {
	logger := test.NewLogger(t)

	// Simulate initial state: both namespaces have been reconciled once.
	// The PostgreSQL user has roles from both auth and openbanking namespaces.
	existingRoles := []string{
		"rds_iam",
		"hydra_read",       // from auth namespace
		"consent_read",     // from auth namespace
		"obie_connect_read", // from openbanking namespace
		"obie_payment_read", // from openbanking namespace
	}

	// --- Reconciliation from "auth" namespace ---
	// The auth namespace only knows about its own databases: hydra, consent.
	authDatabases := []DatabaseSchema{
		{Name: "hydra", Schema: "hydra", Privileges: PrivilegeRead},
		{Name: "consent", Schema: "consent", Privileges: PrivilegeRead},
	}
	staticRoles := []string{"rds_iam"}

	addable, removeable := rolesDiff(logger, existingRoles, staticRoles, authDatabases)

	// BUG: auth reconciliation revokes openbanking roles because they are not
	// in the auth namespace's expected list.
	assert.Nil(t, addable, "no roles should be added")
	assert.Equal(t, []string{"obie_connect_read", "obie_payment_read"}, removeable,
		"BUG REPRODUCED: auth namespace reconciliation revokes openbanking namespace roles")

	// --- After auth reconciliation, the user only has auth roles ---
	rolesAfterAuthReconcile := []string{
		"rds_iam",
		"hydra_read",
		"consent_read",
	}

	// --- Reconciliation from "openbanking" namespace ---
	// The openbanking namespace only knows about its own databases: obie_connect, obie_payment.
	openbankingDatabases := []DatabaseSchema{
		{Name: "obie_connect", Schema: "obie_connect", Privileges: PrivilegeRead},
		{Name: "obie_payment", Schema: "obie_payment", Privileges: PrivilegeRead},
	}

	addable2, removeable2 := rolesDiff(logger, rolesAfterAuthReconcile, staticRoles, openbankingDatabases)

	// BUG: openbanking reconciliation now revokes auth roles!
	assert.Equal(t, []string{"obie_connect_read", "obie_payment_read"}, addable2,
		"openbanking roles should be re-added")
	assert.Equal(t, []string{"hydra_read", "consent_read"}, removeable2,
		"BUG REPRODUCED: openbanking namespace reconciliation revokes auth namespace roles")
}

// TestRolesDiff_CrossNamespaceRevocationCycle demonstrates a full
// grant/revoke cycle showing how roles oscillate between namespaces across
// reconciliation loops.
func TestRolesDiff_CrossNamespaceRevocationCycle(t *testing.T) {
	logger := test.NewLogger(t)
	staticRoles := []string{"rds_iam"}

	authDatabases := []DatabaseSchema{
		{Name: "hydra", Schema: "hydra", Privileges: PrivilegeRead},
	}
	openbankingDatabases := []DatabaseSchema{
		{Name: "obie_connect", Schema: "obie_connect", Privileges: PrivilegeRead},
	}

	// Start: user has no roles.
	existingRoles := []string{}

	// Cycle 1: auth reconciles first.
	addable, removeable := rolesDiff(logger, existingRoles, staticRoles, authDatabases)
	assert.Equal(t, []string{"rds_iam", "hydra_read"}, addable)
	assert.Nil(t, removeable)

	// Apply: user now has rds_iam + hydra_read.
	existingRoles = []string{"rds_iam", "hydra_read"}

	// Cycle 1: openbanking reconciles second.
	addable, removeable = rolesDiff(logger, existingRoles, staticRoles, openbankingDatabases)
	assert.Equal(t, []string{"obie_connect_read"}, addable)
	// BUG: hydra_read is revoked!
	assert.Equal(t, []string{"hydra_read"}, removeable,
		"BUG: openbanking revokes hydra_read granted by auth namespace")

	// Apply: user now has rds_iam + obie_connect_read (hydra_read LOST).
	existingRoles = []string{"rds_iam", "obie_connect_read"}

	// Cycle 2: auth reconciles again.
	addable, removeable = rolesDiff(logger, existingRoles, staticRoles, authDatabases)
	assert.Equal(t, []string{"hydra_read"}, addable)
	// BUG: obie_connect_read is revoked!
	assert.Equal(t, []string{"obie_connect_read"}, removeable,
		"BUG: auth revokes obie_connect_read granted by openbanking namespace")

	// The user NEVER has both hydra_read AND obie_connect_read at the same
	// time after the initial reconciliation. Access oscillates depending on
	// which namespace reconciles last.
}
