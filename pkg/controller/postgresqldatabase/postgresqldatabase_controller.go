package postgresqldatabase

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_postgresqldatabase")

// Add creates a new PostgreSQLDatabase Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	return &ReconcilePostgreSQLDatabase{
		client: mgr.GetClient(),
		db:     db,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("postgresqldatabase-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PostgreSQLDatabase
	err = c.Watch(&source.Kind{Type: &lunarwayv1alpha1.PostgreSQLDatabase{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePostgreSQLDatabase implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePostgreSQLDatabase{}

// ReconcilePostgreSQLDatabase reconciles a PostgreSQLDatabase object
type ReconcilePostgreSQLDatabase struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client

	db *sql.DB
}

// Reconcile reads that state of the cluster for a PostgreSQLDatabase object and makes changes based on the state read
// and what is in the PostgreSQLDatabase.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePostgreSQLDatabase) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PostgreSQLDatabase")

	// Fetch the PostgreSQLDatabase instance
	database := &lunarwayv1alpha1.PostgreSQLDatabase{}
	err := r.client.Get(context.TODO(), request.NamespacedName, database)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	reqLogger = reqLogger.WithValues("database", database.Spec.Name)
	reqLogger.Info("Reconciling PostgreSQLDatabase")

	// Resolve the password, is the value in a configMap or Secret or just a plain value
	password, err := kube.ResourceValue(r.client, database.Spec.Password, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ensure the database is in sync with the object
	err = r.ensurePostgreSQLDatabase(reqLogger, database.Spec.Name, password)
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

func (r *ReconcilePostgreSQLDatabase) ensurePostgreSQLDatabase(log logr.Logger, name, password string) error {
	// Create the service user
	_, err := r.db.Exec(fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'", name, password))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return err
		}
		log.Info(fmt.Sprintf("Service user; %s already exists", name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Service user; %s created", name))
	}

	// Create the database
	_, err = r.db.Exec(fmt.Sprintf("CREATE DATABASE %s", name))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_database" {
			return err
		}
		log.Info(fmt.Sprintf("Database; %s already exists", name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Database; %s created", name))
	}

	// Alter ownership of the database to the database user
	_, err = r.db.Exec(fmt.Sprintf("ALTER DATABASE %s OWNER TO %s", name, name))
	if err != nil {
		return err
	}

	serviceConnection, err := postgresqlConnection(fmt.Sprintf("postgresql://%s:%s@localhost:5432/%s?sslmode=disable", name, password, name))
	if err != nil {
		return err
	}

	// Create schema in the database
	_, err = serviceConnection.Exec(fmt.Sprintf("CREATE SCHEMA %s", name))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_schema" {
			return err
		}
		log.Info(fmt.Sprintf("Schema; %s already exists in database; %s", name, name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Schema; %s created in database; %s", name, name))
	}
	return nil
}
