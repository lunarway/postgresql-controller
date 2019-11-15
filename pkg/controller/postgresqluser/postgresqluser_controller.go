package postgresqluser

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_postgresqluser")

// Add creates a new PostgreSQLUser Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	connectionString := "postgresql://iam_creator:@localhost:5432?sslmode=disable"
	db, err := postgresqlConnection(connectionString)
	if err != nil {
		return err
	}
	return add(mgr, newReconciler(mgr, db))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, db *sql.DB) reconcile.Reconciler {
	return &ReconcilePostgreSQLUser{
		client:        mgr.GetClient(),
		db:            db,
		grantRoles:    []string{"rds_iam", "iam_developer"},
		setAWSPolicy:  iam.SetAWSPolicy,
		rolePrefix:    "iam_developer_",
		awsPolicyName: "postgresql-controller-users",
		awsRegion:     "eu-west-1",
		awsAccountID:  "660013655494",
		awsProfile:    os.Getenv("AWS_PROFILE"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("postgresqluser-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PostgreSQLUser
	err = c.Watch(&source.Kind{Type: &lunarwayv1alpha1.PostgreSQLUser{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePostgreSQLUser implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePostgreSQLUser{}

// ReconcilePostgreSQLUser reconciles a PostgreSQLUser object
type ReconcilePostgreSQLUser struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client

	db           *sql.DB
	setAWSPolicy func(log logr.Logger, policy iam.AWSPolicy, userID string) error

	grantRoles []string
	rolePrefix string

	awsPolicyName string
	awsRegion     string
	awsAccountID  string
	awsProfile    string
}

// Reconcile reads that state of the cluster for a PostgreSQLUser object and makes changes based on the state read
// and what is in the PostgreSQLUser.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePostgreSQLUser) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PostgreSQLUSer")
	// Fetch the PostgreSQLUser instance
	user := &lunarwayv1alpha1.PostgreSQLUser{}
	err := r.client.Get(context.TODO(), request.NamespacedName, user)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Object not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// User instance created or updated
	reqLogger = reqLogger.WithValues("user", user.Spec.Name)
	reqLogger.Info("Reconciling PostgreSQLUser", "user", user.Spec.Name)

	err = r.ensurePostgreSQLRole(reqLogger, user.Spec.Name)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.setAWSPolicy(reqLogger, iam.AWSPolicy{Name: r.awsPolicyName, Region: r.awsRegion, Profile: r.awsProfile, AccountID: r.awsAccountID}, user.Spec.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func postgresqlConnection(connectionString string) (*sql.DB, error) {
	log.Info("Connecting to database", "url", connectionString)
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (r *ReconcilePostgreSQLUser) ensurePostgreSQLRole(log logr.Logger, name string) error {
	name = fmt.Sprintf("%s%s", r.rolePrefix, name)
	roles := strings.Join(r.grantRoles, ", ")
	_, err := r.db.Exec(fmt.Sprintf("CREATE ROLE %s WITH LOGIN IN ROLE %s", name, roles))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return err
		}
		log.Info(fmt.Sprintf("Role %s already exists", name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Role %s created", name))
	}

	_, err = r.db.Exec(fmt.Sprintf("GRANT %s TO %s", roles, name))
	if err != nil {
		return err
	}
	return nil
}
