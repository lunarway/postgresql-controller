package postgresqluser

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestParseHostCredentials(t *testing.T) {
	tt := []struct {
		name   string
		input  map[string]string
		output map[string]postgres.Credentials
		err    error
	}{
		{
			name:   "nil map",
			input:  nil,
			output: nil,
		},
		{
			name: "single host",
			input: map[string]string{
				"host:5432": "user:password",
			},
			output: map[string]postgres.Credentials{
				"host:5432": {
					Name:     "user",
					Password: "password",
				},
			},
			err: nil,
		},
		{
			name: "multiple hosts",
			input: map[string]string{
				"host1:5432": "user1:password1",
				"host2:5432": "user2:password2",
			},
			output: map[string]postgres.Credentials{
				"host1:5432": {
					Name:     "user1",
					Password: "password1",
				},
				"host2:5432": {
					Name:     "user2",
					Password: "password2",
				},
			},
			err: nil,
		},
		{
			name: "single host without user or password",
			input: map[string]string{
				"host:5432": "",
			},
			output: nil,
			err:    errors.New("username empty"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output, err := parseHostCredentials(tc.input)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as expected")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}

// TestReconcile_badConfigmapReference tests that reconcilation is completed
// successfully even though a an error occours during database resolvement. This
// is to ensure that a single bad PostgreSQLDatabase resource will not block the
// reconciliation of the remaining ones.
func TestReconcile_badConfigmapReference(t *testing.T) {
	// Set the logger to development mode for verbose logs.
	logf.SetLogger(logf.ZapLogger(true))
	logger := logf.Log
	host := test.Integration(t)
	var (
		namespace     = "default"
		database1Name = "database1"
		database2Name = "database2"
		userName      = "service_user"

		// user requesting access to all databases on host
		userResource = &lunarwayv1alpha1.PostgreSQLUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: lunarwayv1alpha1.PostgreSQLUserSpec{
				Name: userName,
				Read: []lunarwayv1alpha1.AccessSpec{
					{
						Host: lunarwayv1alpha1.ResourceVar{
							Value: host,
						},
						AllDatabases: true,
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
	s.AddKnownTypes(lunarwayv1alpha1.SchemeGroupVersion, database1Resource)
	s.AddKnownTypes(lunarwayv1alpha1.SchemeGroupVersion, userResource)
	s.AddKnownTypes(lunarwayv1alpha1.SchemeGroupVersion, &lunarwayv1alpha1.PostgreSQLDatabaseList{})

	// Add tracked objects to the fake client simulating their existence in a k8s
	// cluster
	objs := []runtime.Object{
		database1Resource,
		database2Resource,
		userResource,
	}
	cl := fake.NewFakeClient(objs...)

	// Create a controller object with the fake client but otherwise "live" setup
	// with database interaction
	r := &ReconcilePostgreSQLUser{
		client: cl,
		granter: grants.Granter{
			HostCredentials: map[string]postgres.Credentials{
				host: {
					Name:     "iam_creator",
					Password: "",
				},
			},
			Log:                      logger,
			AllDatabasesReadEnabled:  true,
			AllDatabasesWriteEnabled: true,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(cl, namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(cl, resource, namespace)
			},
		},
		setAWSPolicy: func(log logr.Logger, credentials *credentials.Credentials, policy iam.AWSPolicy, userID string) error {
			return nil
		},
	}

	// seed database1 into the postgres host
	dbConn, err := postgres.Connect(logf.Log, postgres.ConnectionString{
		Database: "postgres",
		Host:     host,
		Password: "",
		User:     "iam_creator",
	})
	if !assert.NoError(t, err, "failed to connect to database to seed database") {
		return
	}
	err = postgres.Database(logf.Log, dbConn, host, postgres.Credentials{
		Name:     database1Name,
		Password: "123456",
		User:     database1Name,
	})
	if !assert.NoError(t, err, "failed to seed database") {
		return
	}

	// reconcile user requesting access to all databases with a bad database
	// reference
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(req)
	assert.NoError(t, err, "reconciliation failed")
	assert.Equal(t, reconcile.Result{
		Requeue:      false,
		RequeueAfter: 0,
	}, res, "result not as expected")
}
