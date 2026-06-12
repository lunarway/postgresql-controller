package controller

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/iam"
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

// TestExternalServiceUser_reconcile_createsRole verifies that reconciling a
// PostgreSQLExternalServiceUser creates the Postgres LOGIN role with rds_iam
// granted and calls EnsureIAMExternalServiceUser with the correct arguments.
func TestExternalServiceUser_reconcile_createsRole(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)

	var (
		epoch        = time.Now().UnixNano()
		namespace    = "default"
		dbUsername   = fmt.Sprintf("ext_svc_%d", epoch)
		principalArn = "arn:aws:iam::478824949770:user/VVCTenantUser"

		resource = &lunarwayv1alpha1.PostgreSQLExternalServiceUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dbUsername,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLExternalServiceUserSpec{
				PrincipalArn: principalArn,
				Host:         lunarwayv1alpha1.ResourceVar{Value: host},
				DBUsername:   dbUsername,
			},
		}
	)

	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, resource, &lunarwayv1alpha1.PostgreSQLExternalServiceUserList{})

	cl := fake.NewClientBuilder().
		WithRuntimeObjects([]runtime.Object{resource}...).
		WithStatusSubresource(resource).
		Build()

	var (
		capturedPrincipalArn string
		capturedDBUsername   string
	)

	r := &PostgreSQLExternalServiceUserReconciler{
		Client: cl,
		Scheme: s,
		EnsureIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, principalArn, dbUsername string) error {
			capturedPrincipalArn = principalArn
			capturedDBUsername = dbUsername
			return nil
		},
		RemoveIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _ string) error {
			return nil
		},
		AWSPolicyName:      "test-policy",
		AWSRegion:          "eu-west-1",
		AWSAccountID:       "000000000000",
		AWSAccessKeyID:     testAWSKeyID,
		AWSSecretAccessKey: testAWSSecretKey,
		IAMPolicyPrefix:    "/test/",
		HostCredentials: map[string]postgres.Credentials{
			host: {Name: "postgres", User: testPostgresUser, Password: testPostgresPassword},
		},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: dbUsername, Namespace: namespace}}
	doReconcileExternalServiceUser(t, r, req)

	// IAM function received correct arguments.
	assert.Equal(t, principalArn, capturedPrincipalArn)
	assert.Equal(t, dbUsername, capturedDBUsername)

	// Postgres role should exist and have rds_iam granted.
	db, err := postgres.Connect(postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     testPostgresUser,
		Password: testPostgresPassword,
	})
	require.NoError(t, err)
	defer db.Close()

	assertRoleExists(t, db, dbUsername)
	assertRoleGranted(t, db, dbUsername, "rds_iam")
}

// TestExternalServiceUser_reconcile_withSpecRoles verifies that spec.Roles are
// granted to the DB user in addition to the mandatory rds_iam role.
func TestExternalServiceUser_reconcile_withSpecRoles(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)

	var (
		epoch      = time.Now().UnixNano()
		namespace  = "default"
		dbUsername = fmt.Sprintf("ext_svc_roles_%d", epoch)
		extraRole  = fmt.Sprintf("extra_role_%d", epoch)
	)

	// Pre-create the extra role so it can be granted.
	adminDB, err := postgres.Connect(postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     testPostgresUser,
		Password: testPostgresPassword,
	})
	require.NoError(t, err)
	_, err = adminDB.Exec(fmt.Sprintf("CREATE ROLE %s NOLOGIN", extraRole))
	require.NoError(t, err)
	adminDB.Close()

	resource := &lunarwayv1alpha1.PostgreSQLExternalServiceUser{
		ObjectMeta: metav1.ObjectMeta{Name: dbUsername, Namespace: namespace},
		Spec: lunarwayv1alpha1.PostgreSQLExternalServiceUserSpec{
			PrincipalArn: "arn:aws:iam::478824949770:user/VVCTenantUser",
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			DBUsername:   dbUsername,
			Roles: []lunarwayv1alpha1.PostgreSQLExternalServiceUserRole{
				{RoleName: extraRole},
			},
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, resource, &lunarwayv1alpha1.PostgreSQLExternalServiceUserList{})

	cl := fake.NewClientBuilder().
		WithRuntimeObjects([]runtime.Object{resource}...).
		WithStatusSubresource(resource).
		Build()

	r := &PostgreSQLExternalServiceUserReconciler{
		Client: cl,
		Scheme: s,
		EnsureIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _, _ string) error {
			return nil
		},
		RemoveIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _ string) error {
			return nil
		},
		AWSRegion:          "eu-west-1",
		AWSAccountID:       "000000000000",
		AWSAccessKeyID:     testAWSKeyID,
		AWSSecretAccessKey: testAWSSecretKey,
		IAMPolicyPrefix:    "/test/",
		AWSPolicyName:      "test-policy",
		HostCredentials: map[string]postgres.Credentials{
			host: {Name: "postgres", User: testPostgresUser, Password: testPostgresPassword},
		},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: dbUsername, Namespace: namespace}}
	doReconcileExternalServiceUser(t, r, req)

	db, err := postgres.Connect(postgres.ConnectionString{
		Host: host, Database: "postgres", User: testPostgresUser, Password: testPostgresPassword,
	})
	require.NoError(t, err)
	defer db.Close()

	assertRoleExists(t, db, dbUsername)
	assertRoleGranted(t, db, dbUsername, "rds_iam")
	assertRoleGranted(t, db, dbUsername, extraRole)
}

// TestExternalServiceUser_reconcile_unknownHost verifies that reconciliation
// does not panic and reports an error when the host is not in HostCredentials.
func TestExternalServiceUser_reconcile_unknownHost(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// Does not require a real Postgres — the error fires before connecting.
	namespace := "default"
	dbUsername := "svc_user"

	resource := &lunarwayv1alpha1.PostgreSQLExternalServiceUser{
		ObjectMeta: metav1.ObjectMeta{Name: dbUsername, Namespace: namespace},
		Spec: lunarwayv1alpha1.PostgreSQLExternalServiceUserSpec{
			PrincipalArn: "arn:aws:iam::478824949770:user/VVCTenantUser",
			Host:         lunarwayv1alpha1.ResourceVar{Value: "unknown-host:5432"},
			DBUsername:   dbUsername,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, resource, &lunarwayv1alpha1.PostgreSQLExternalServiceUserList{})

	cl := fake.NewClientBuilder().
		WithRuntimeObjects([]runtime.Object{resource}...).
		WithStatusSubresource(resource).
		Build()

	r := &PostgreSQLExternalServiceUserReconciler{
		Client: cl,
		Scheme: s,
		EnsureIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _, _ string) error {
			t.Fatal("EnsureIAMExternalServiceUser should not be called when host is unknown")
			return nil
		},
		RemoveIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _ string) error {
			return nil
		},
		AWSRegion:          "eu-west-1",
		AWSAccountID:       "000000000000",
		AWSAccessKeyID:     testAWSKeyID,
		AWSSecretAccessKey: testAWSSecretKey,
		IAMPolicyPrefix:    "/test/",
		AWSPolicyName:      "test-policy",
		HostCredentials:    map[string]postgres.Credentials{}, // empty — host not found
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: dbUsername, Namespace: namespace}}

	// Add finalizer first (two reconcile passes needed).
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err) // first pass just adds finalizer

	_, err = r.Reconcile(context.Background(), req)
	assert.Error(t, err, "expected error when host is not in HostCredentials")
}

// TestExternalServiceUser_reconcile_iamError verifies that an IAM error causes
// the reconcile to return an error and the status to reflect failure.
func TestExternalServiceUser_reconcile_iamError(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)

	var (
		epoch      = time.Now().UnixNano()
		namespace  = "default"
		dbUsername = fmt.Sprintf("ext_svc_iamerr_%d", epoch)
	)

	resource := &lunarwayv1alpha1.PostgreSQLExternalServiceUser{
		ObjectMeta: metav1.ObjectMeta{Name: dbUsername, Namespace: namespace},
		Spec: lunarwayv1alpha1.PostgreSQLExternalServiceUserSpec{
			PrincipalArn: "arn:aws:iam::478824949770:user/VVCTenantUser",
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			DBUsername:   dbUsername,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, resource, &lunarwayv1alpha1.PostgreSQLExternalServiceUserList{})

	cl := fake.NewClientBuilder().
		WithRuntimeObjects([]runtime.Object{resource}...).
		WithStatusSubresource(resource).
		Build()

	iamErr := fmt.Errorf("simulated IAM error")

	r := &PostgreSQLExternalServiceUserReconciler{
		Client: cl,
		Scheme: s,
		EnsureIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _, _ string) error {
			return iamErr
		},
		RemoveIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _ string) error {
			return nil
		},
		AWSRegion:          "eu-west-1",
		AWSAccountID:       "000000000000",
		AWSAccessKeyID:     testAWSKeyID,
		AWSSecretAccessKey: testAWSSecretKey,
		IAMPolicyPrefix:    "/test/",
		AWSPolicyName:      "test-policy",
		HostCredentials: map[string]postgres.Credentials{
			host: {Name: "postgres", User: testPostgresUser, Password: testPostgresPassword},
		},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: dbUsername, Namespace: namespace}}

	// First pass: add finalizer.
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Second pass: hits the IAM error.
	_, err = r.Reconcile(context.Background(), req)
	assert.ErrorIs(t, err, iamErr)
}

// TestExternalServiceUser_reconcile_deletion verifies that finalizer cleanup
// calls RemoveIAMExternalServiceUser and drops the Postgres role.
func TestExternalServiceUser_reconcile_deletion(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)

	var (
		epoch      = time.Now().UnixNano()
		namespace  = "default"
		dbUsername = fmt.Sprintf("ext_svc_del_%d", epoch)
	)

	now := metav1.Now()
	resource := &lunarwayv1alpha1.PostgreSQLExternalServiceUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:              dbUsername,
			Namespace:         namespace,
			DeletionTimestamp: &now,
			Finalizers:        []string{externalServiceUserFinalizer},
		},
		Spec: lunarwayv1alpha1.PostgreSQLExternalServiceUserSpec{
			PrincipalArn: "arn:aws:iam::478824949770:user/VVCTenantUser",
			Host:         lunarwayv1alpha1.ResourceVar{Value: host},
			DBUsername:   dbUsername,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, resource, &lunarwayv1alpha1.PostgreSQLExternalServiceUserList{})

	cl := fake.NewClientBuilder().
		WithRuntimeObjects([]runtime.Object{resource}...).
		WithStatusSubresource(resource).
		Build()

	// Pre-create the role so deletion has something to drop.
	adminDB, err := postgres.Connect(postgres.ConnectionString{
		Host: host, Database: "postgres", User: testPostgresUser, Password: testPostgresPassword,
	})
	require.NoError(t, err)
	_, err = adminDB.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN", dbUsername))
	require.NoError(t, err)
	adminDB.Close()

	var removeCalled bool
	r := &PostgreSQLExternalServiceUserReconciler{
		Client: cl,
		Scheme: s,
		EnsureIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, _, _ string) error {
			return nil
		},
		RemoveIAMExternalServiceUser: func(_ *iam.Client, _ logr.Logger, _ iam.EnsureExternalServiceUserConfig, username string) error {
			removeCalled = true
			assert.Equal(t, dbUsername, username)
			return nil
		},
		AWSRegion:          "eu-west-1",
		AWSAccountID:       "000000000000",
		AWSAccessKeyID:     testAWSKeyID,
		AWSSecretAccessKey: testAWSSecretKey,
		IAMPolicyPrefix:    "/test/",
		AWSPolicyName:      "test-policy",
		HostCredentials: map[string]postgres.Credentials{
			host: {Name: "postgres", User: testPostgresUser, Password: testPostgresPassword},
		},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: dbUsername, Namespace: namespace}}
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	assert.True(t, removeCalled, "RemoveIAMExternalServiceUser should have been called")

	// Role should be dropped.
	db, err := postgres.Connect(postgres.ConnectionString{
		Host: host, Database: "postgres", User: testPostgresUser, Password: testPostgresPassword,
	})
	require.NoError(t, err)
	defer db.Close()
	assertRoleNotExists(t, db, dbUsername)
}

// doReconcileExternalServiceUser drives the reconciler to completion,
// tolerating requeues (e.g. finalizer registration pass).
func doReconcileExternalServiceUser(t *testing.T, r *PostgreSQLExternalServiceUserReconciler, req reconcile.Request) {
	t.Helper()
	const limit = 10
	for i := 0; i < limit; i++ {
		res, err := r.Reconcile(context.Background(), req)
		if !assert.NoError(t, err, "reconciliation failed on attempt %d", i+1) {
			return
		}
		if !res.Requeue {
			return
		}
	}
	t.Errorf("reconciler did not converge after %d attempts", limit)
}

func assertRoleExists(t *testing.T, db *sql.DB, roleName string) {
	t.Helper()
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", roleName).Scan(&exists)
	require.NoError(t, err, "failed to query pg_roles for %s", roleName)
	assert.True(t, exists, "expected role %s to exist", roleName)
}

func assertRoleNotExists(t *testing.T, db *sql.DB, roleName string) {
	t.Helper()
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", roleName).Scan(&exists)
	require.NoError(t, err, "failed to query pg_roles for %s", roleName)
	assert.False(t, exists, "expected role %s to not exist", roleName)
}

func assertRoleGranted(t *testing.T, db *sql.DB, roleName, grantedRole string) {
	t.Helper()
	var granted bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pg_auth_members m
			JOIN pg_roles r ON r.oid = m.roleid
			JOIN pg_roles u ON u.oid = m.member
			WHERE u.rolname = $1 AND r.rolname = $2
		)`, roleName, grantedRole).Scan(&granted)
	require.NoError(t, err, "failed to query pg_auth_members for %s → %s", roleName, grantedRole)
	assert.True(t, granted, "expected %s to be granted to %s", grantedRole, roleName)
}

// Ensure ctrl.Log is initialised for tests that don't use logf.SetLogger.
// testPostgresUser and testPostgresPassword are the credentials for the
// integration test Postgres instance (local Docker container, not production).
const (
	testPostgresUser     = "iam_creator"
	testPostgresPassword = testPostgresUser //nolint:gosec — local Docker test container only
	testAWSKeyID         = "foo"
	testAWSSecretKey     = "bar" //nolint:gosec
)

// Ensure ctrl.Log is initialised for tests that don't use logf.SetLogger.
var _ = ctrl.Log
