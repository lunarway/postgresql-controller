package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
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

var trueValue = true

// TestReconcile_badConfigmapReference tests that reconcilation is completed
// successfully even though a an error occours during database resolvement. This
// is to ensure that a single bad PostgreSQLDatabase resource will not block the
// reconciliation of the remaining ones.
func TestReconcile_badConfigmapReference(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch         = time.Now().UnixNano()
		namespace     = "default"
		database1Name = fmt.Sprintf("database1_%d", epoch)
		database2Name = fmt.Sprintf("database2_%d", epoch)
		userName      = fmt.Sprintf("service_user_%d", epoch)

		// user requesting access to all databases on host
		userResource = &lunarwayv1alpha1.PostgreSQLUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
				Name: userName,
				Read: &[]lunarwayv1alpha1.AccessSpec{
					{
						Host: lunarwayv1alpha1.ResourceVar{
							Value: host,
						},
						AllDatabases: &trueValue,
					},
				},
			},
		}

		// valid database on host
		database1Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database1Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database1Name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database1Name,
				},
			},
		}

		// invalid database referencing a non-existing configmap
		database2Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database2Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database2Name,
				Host: lunarwayv1alpha1.ResourceVar{
					ValueFrom: &lunarwayv1alpha1.ResourceVarSource{
						ConfigMapKeyRef: &lunarwayv1alpha1.KeySelector{
							Name: "unknown",
						},
					},
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "12346",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database2Name,
				},
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, database1Resource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, userResource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		database1Resource,
		database2Resource,
		userResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLUserReconciler{
		Client: cl,
		Log:    ctrl.Log.WithName(t.Name()),
		Granter: grants.Granter{
			Now: time.Now,
			HostCredentials: map[string]postgres.Credentials{
				host: {
					Name:     "iam_creator",
					Password: "",
				},
			},
			AllDatabasesReadEnabled:  true,
			AllDatabasesWriteEnabled: true,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(cl, namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(cl, resource, namespace)
			},
		},
		AddUser: func(client *iam.Client, config iam.AddUserConfig, username, rolename string) error {
			return nil
		},
	}

	// seed database1 into the postgres host
	seededDatabase(t, host, database1Name)

	// reconcile user requesting access to all databases with a bad database
	// reference
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")
}

// TestReconcile_rolePrefix tests that reconciliations respect the rolePrefix
// setting. The PostgreSQLUser reconciler is configured with a prefix and a
// database and user are reconciled. Then a connect attempt is done with the
// prefixed user name.
func TestReconcile_rolePrefix(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch         = time.Now().UnixNano()
		namespace     = "default"
		database1Name = fmt.Sprintf("database1_%d", epoch)
		userName      = fmt.Sprintf("user_%d", epoch)
		rolePrefix    = "iam_developer_"

		// user requesting access to all databases on host
		userResource = &lunarwayv1alpha1.PostgreSQLUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
				Name: userName,
				Read: &[]lunarwayv1alpha1.AccessSpec{
					{
						Host: lunarwayv1alpha1.ResourceVar{
							Value: host,
						},
						AllDatabases: &trueValue,
					},
				},
			},
		}

		// valid database on host
		database1Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database1Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database1Name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database1Name,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, database1Resource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, userResource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		database1Resource,
		userResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLUserReconciler{
		Client:     cl,
		Log:        ctrl.Log.WithName(t.Name()),
		RolePrefix: rolePrefix,
		Granter: grants.Granter{
			Now: time.Now,
			HostCredentials: map[string]postgres.Credentials{
				host: {
					Name:     "iam_creator",
					Password: "",
				},
			},
			AllDatabasesReadEnabled:  true,
			AllDatabasesWriteEnabled: true,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(cl, namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(cl, resource, namespace)
			},
		},
		AddUser: func(client *iam.Client, config iam.AddUserConfig, username, rolename string) error {
			return nil
		},
	}

	// seed database1 into the postgres host
	seededDatabase(t, host, database1Name)

	// reconcile user requesting access to all databases with a bad database
	// reference
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")

	// assert that the user can connect with a prefixed role
	assertAccess(t, host, database1Name, fmt.Sprintf("%s%s", rolePrefix, userName)) // simulates what users will sign in with through AWS
}

// TestReconcile_dotInName tests that we can handle PostgeSQLUser resources with
// a spec.name field that contains a '.' character, eg. my.name. This is needed
// as the name is used for both the PostgreSQL role and the email in AWS policy.
func TestReconcile_dotInName(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch             = time.Now().UnixNano()
		namespace         = "default"
		database1Name     = fmt.Sprintf("database1_%d", epoch)
		userName          = fmt.Sprintf("user.%d", epoch)
		userNameSanitized = fmt.Sprintf("user_%d", epoch)

		// user requesting access to all databases on host
		userResource = &lunarwayv1alpha1.PostgreSQLUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
				Name: userName,
				Read: &[]lunarwayv1alpha1.AccessSpec{
					{
						Host: lunarwayv1alpha1.ResourceVar{
							Value: host,
						},
						AllDatabases: &trueValue,
					},
				},
			},
		}

		// valid database on host
		database1Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database1Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database1Name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database1Name,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, database1Resource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, userResource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		database1Resource,
		userResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLUserReconciler{
		Client:     cl,
		Log:        ctrl.Log.WithName(t.Name()),
		RolePrefix: "",
		Granter: grants.Granter{
			Now: time.Now,
			HostCredentials: map[string]postgres.Credentials{
				host: {
					Name:     "iam_creator",
					Password: "",
				},
			},
			AllDatabasesReadEnabled:  true,
			AllDatabasesWriteEnabled: true,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(cl, namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(cl, resource, namespace)
			},
		},
		AddUser: func(client *iam.Client, config iam.AddUserConfig, username, rolename string) error {
			assert.Equal(t, userName, username, "iam username must be the original")
			assert.Equal(t, rolename, userNameSanitized, "iam rolename must be the sanitized")
			return nil
		},
	}

	// seed database1 into the postgres host
	seededDatabase(t, host, database1Name)

	// reconcile user requesting access to all databases with a bad database
	// reference
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")

	// assert that the user can connect with a prefixed role
	assertAccess(t, host, database1Name, userNameSanitized) // simulates what users will sign in with through AWS
}

// TestReconcile_multipleDatabaseResources tests that access granted by
// allDatabases works as expected. Two databases on the same host are seeded
// with a table. After reconciliation of a user requesting access to all
// database a query on each table is made.
//
// The test confirms a regression in the role mechanism introduced in
// 46e333a80e8dd6ea7829ccd53c3d9578ef0c0575 resulting in only a single database
// role being active at any time.
func TestReconcile_multipleDatabaseResources(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch         = time.Now().UnixNano()
		namespace     = "default"
		database1Name = fmt.Sprintf("database1_%d", epoch)
		database2Name = fmt.Sprintf("database2_%d", epoch)
		userName      = fmt.Sprintf("user_%d", epoch)

		// user requesting access to all databases on host
		userResource = &lunarwayv1alpha1.PostgreSQLUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
				Name: userName,
				Read: &[]lunarwayv1alpha1.AccessSpec{
					{
						Host: lunarwayv1alpha1.ResourceVar{
							Value: host,
						},
						AllDatabases: &trueValue,
					},
				},
			},
		}

		// valid database on host
		database1Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database1Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database1Name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database1Name,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
		database2Resource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      database2Name,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: database2Name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: database2Name,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, database1Resource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, userResource)
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		database1Resource,
		database2Resource,
		userResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLUserReconciler{
		Client: cl,
		Log:    ctrl.Log.WithName(t.Name()),
		Granter: grants.Granter{
			Now: time.Now,
			HostCredentials: map[string]postgres.Credentials{
				host: {
					Name:     "iam_creator",
					Password: "",
				},
			},
			AllDatabasesReadEnabled:  true,
			AllDatabasesWriteEnabled: true,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(cl, namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(cl, resource, namespace)
			},
		},
		AddUser: func(client *iam.Client, config iam.AddUserConfig, username, rolename string) error {
			return nil
		},
	}

	// seed database1 into the postgres host
	seededDatabase(t, host, database1Name)
	seededDatabase(t, host, database2Name)

	// reconcile user requesting access to all databases with a bad database
	// reference
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")

	// assert that the user can connect to both databases
	assertAccess(t, host, database1Name, userName)
	assertAccess(t, host, database2Name, userName)
}

// seededDatabase creates a database with name along with a 'movies' table owned
// by the database role.
func seededDatabase(t *testing.T, host, name string) {
	t.Helper()

	dbConn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: "postgres",
		Host:     host,
		Password: "",
		User:     "iam_creator",
	})
	if !assert.NoErrorf(t, err, "failed to connect to database host to seed database '%s'", name) {
		return
	}
	err = postgres.Database(logf.Log, dbConn, host, postgres.Credentials{
		Name:     name,
		Password: "123456",
		User:     name,
	})
	if !assert.NoErrorf(t, err, "failed to created seeded database '%s'", name) {
		return
	}
	db1Conn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: name,
		Host:     host,
		Password: "123456",
		User:     name,
	})
	if !assert.NoErrorf(t, err, "failed to connect to database '%s' to create a table", name) {
		return
	}
	_, err = db1Conn.Exec(`CREATE TABLE movies(title varchar(50));`)
	if !assert.NoErrorf(t, err, "failed to create table in database '%s'", name) {
		return
	}
}

func assertAccess(t *testing.T, host, databaseName, userName string) {
	userConn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: databaseName,
		Host:     host,
		Password: "",
		User:     userName,
	})
	if !assert.NoErrorf(t, err, "failed to connect to database '%s' with user '%s'", databaseName, userName) {
		return
	}
	defer userConn.Close()
	_, err = userConn.Query(fmt.Sprintf("SELECT * from %s.movies", databaseName))
	assert.NoErrorf(t, err, "failed to query table in database '%s'", databaseName)
}
