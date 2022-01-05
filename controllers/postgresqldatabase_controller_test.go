package controllers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
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

func TestStatus_update(t *testing.T) {
	before := metav1.Time{
		Time: time.Date(2019, time.December, 18, 17, 7, 3, 0, time.UTC),
	}
	now := metav1.Time{
		Time: time.Date(2019, time.December, 18, 18, 7, 3, 0, time.UTC),
	}
	tt := []struct {
		name    string
		status  status
		err     error
		changes bool
		after   *lunarwayv1alpha1.PostgreSQLDatabase
	}{
		{
			name: "new status",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase: "",
					},
				},
			},
			err: &ctlerrors.Invalid{
				Err: errors.New("some validation error"),
			},
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: now,
					Error:        "some validation error",
				},
			},
		},
		{
			name: "same status and error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err:     errors.New("some validation error"),
			changes: false,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
					PhaseUpdated: before,
					Error:        "some validation error",
				},
			},
		},
		{
			name: "same status and different error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err: &ctlerrors.Invalid{
				Err: errors.New("some new validation error"),
			},
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
					PhaseUpdated: now,
					Error:        "some new validation error",
				},
			},
		},
		{
			name: "new status and no error",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid,
						PhaseUpdated: before,
						Error:        "some validation error",
					},
				},
			},
			err:     nil,
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
					PhaseUpdated: now,
					Error:        "",
				},
			},
		},
		{
			name: "host name changed",
			status: status{
				database: &lunarwayv1alpha1.PostgreSQLDatabase{
					Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
						Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed,
						PhaseUpdated: before,
						Error:        "unknown host",
						Host:         "localhost:1234",
					},
				},
				host: "localhost:5432",
			},
			err:     nil,
			changes: true,
			after: &lunarwayv1alpha1.PostgreSQLDatabase{
				Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
					Phase:        lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
					PhaseUpdated: now,
					Error:        "",
					Host:         "localhost:5432",
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tc.status.now = func() metav1.Time {
				return now
			}
			changes := tc.status.update(tc.err)
			assert.Equal(t, changes, tc.changes, "change indication not as expected")
			assert.Equal(t, tc.after, tc.status.database, "database status not as expected")
		})
	}
}

// TestPostgreSQLDatabase_Reconcile_hostCredentialsResourceReference tests that
// a PostgreSQLDatabase resource can reference a PostgreSQLHostCredentials
// resource.
func TestPostgreSQLDatabase_Reconcile_hostCredentialsResourceReference(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch               = time.Now().UnixNano()
		namespace           = "default"
		databaseName        = fmt.Sprintf("database_%d", epoch)
		hostCredentialsName = fmt.Sprintf("hostcredentials_%d", epoch)

		credentialsResource = &lunarwayv1alpha1.PostgreSQLHostCredentials{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hostCredentialsName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLHostCredentialsSpec{
				Host: lunarwayv1alpha1.ResourceVar{
					Value: "localhost",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: "admin",
				},
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "password",
				},
			},
		}

		databaseResource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      databaseName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name:            databaseName,
				HostCredentials: hostCredentialsName,
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: databaseName,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, databaseResource, credentialsResource, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		databaseResource,
		credentialsResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLDatabaseReconciler{
		Client:          cl,
		Log:             ctrl.Log.WithName(t.Name()),
		HostCredentials: nil,
	}

	// seed database into the postgres host
	seededDatabase(t, host, databaseName)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      databaseName,
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

// TestPostgreSQLDatabase_Reconcile_unknownHostCredentialsResourceReference
// tests that references to an unknown host credentials resource will results in
// a reconciliation error.
func TestPostgreSQLDatabase_Reconcile_unknownHostCredentialsResourceReference(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	host := test.Integration(t)
	var (
		epoch        = time.Now().UnixNano()
		namespace    = "default"
		databaseName = fmt.Sprintf("database_%d", epoch)

		databaseResource = &lunarwayv1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      databaseName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name:            databaseName,
				HostCredentials: "unknown",
				Password: lunarwayv1alpha1.ResourceVar{
					Value: "123456",
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: databaseName,
				},
			},
			Status: lunarwayv1alpha1.PostgreSQLDatabaseStatus{
				Phase: lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning,
			},
		}
	)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(lunarwayv1alpha1.GroupVersion, databaseResource, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		databaseResource,
	}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		Build()

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &PostgreSQLDatabaseReconciler{
		Client:          cl,
		Log:             ctrl.Log.WithName(t.Name()),
		HostCredentials: nil,
	}

	seededDatabase(t, host, databaseName)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      databaseName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(context.Background(), req)
	assert.Error(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")
}
