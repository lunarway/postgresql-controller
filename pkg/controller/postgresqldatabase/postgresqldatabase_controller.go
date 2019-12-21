package postgresqldatabase

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_postgresqldatabase").WithValues("controller", "postgresqldatabase-controller")

var FlagSet *pflag.FlagSet

func init() {
	FlagSet = pflag.NewFlagSet("controller_postgresqldatabase", pflag.ExitOnError)
	FlagSet.StringToString("host-credentials-database", nil, "Host and credential pairs in the form hostname=user:password. Use comma separated pairs for multiple hosts")
}

func parseFlags(c *ReconcilePostgreSQLDatabase) {
	hosts, err := FlagSet.GetStringToString("host-credentials-database")
	parseError(err, "host-credentials-database")
	c.hostCredentials, err = parseHostCredentials(hosts)
	parseError(err, "host-credentials: invalid format")
	var hostNames []string
	for host := range c.hostCredentials {
		hostNames = append(hostNames, host)
	}
	log.Info("Controller configured",
		"hosts", hostNames,
	)
}

func parseError(err error, flag string) {
	if err != nil {
		log.Error(err, fmt.Sprintf("error parsing flag %s", flag))
		os.Exit(1)
	}
}

func parseHostCredentials(hosts map[string]string) (map[string]postgres.Credentials, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	hostCredentials := make(map[string]postgres.Credentials)
	for host, credentials := range hosts {
		var err error
		hostCredentials[host], err = postgres.ParseUsernamePassword(credentials)
		if err != nil {
			return nil, err
		}
	}
	return hostCredentials, nil
}

// Add creates a new PostgreSQLDatabase Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	c := &ReconcilePostgreSQLDatabase{
		client: mgr.GetClient(),
	}
	parseFlags(c)
	return c
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

	// contains a map of credentials for hosts
	hostCredentials map[string]postgres.Credentials
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
	status, err := r.reconcile(reqLogger, request)
	status.Persist(err)
	return reconcile.Result{}, stopRequeueOnInvalid(reqLogger, err)
}

func (r *ReconcilePostgreSQLDatabase) reconcile(reqLogger logr.Logger, request reconcile.Request) (status, error) {
	// Fetch the PostgreSQLDatabase instance
	database := &lunarwayv1alpha1.PostgreSQLDatabase{}
	err := r.client.Get(context.TODO(), request.NamespacedName, database)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return status{}, nil
		}
		// Error reading the object - requeue the request.
		return status{}, err
	}
	reqLogger = reqLogger.WithValues("database", database.Spec.Name)
	reqLogger.Info("Reconciling PostgreSQLDatabase")

	status := status{
		log:      reqLogger,
		client:   r.client,
		now:      metav1.Now,
		database: database,
	}
	host, err := kube.ResourceValue(r.client, database.Spec.Host, request.Namespace)
	if err != nil {
		return status, fmt.Errorf("resolve host reference: %w", err)
	}
	status.host = host
	password, err := kube.ResourceValue(r.client, database.Spec.Password, request.Namespace)
	if err != nil {
		return status, fmt.Errorf("resolve password reference: %w", err)
	}

	// Ensure the database is in sync with the object
	err = r.EnsurePostgreSQLDatabase(reqLogger, host, database.Spec.Name, password)
	if err != nil {
		return status, fmt.Errorf("ensure database: %w", err)
	}
	return status, nil
}

type status struct {
	log    logr.Logger
	client client.Client
	now    func() metav1.Time

	database *lunarwayv1alpha1.PostgreSQLDatabase
	host     string
}

// Persist writes the status to a PostgreSQLDatabase instance and persists it on
// client. Any errors are logged.
func (s *status) Persist(err error) {
	ok := s.update(err)
	if !ok {
		return
	}
	err = s.client.Status().Update(context.TODO(), s.database)
	if err != nil {
		log.Error(err, "failed to set status of database", "status", s)
	}
	return
}

// update updates database reference based on its values and returns whether any
// changes were written.
func (s *status) update(err error) bool {
	var errorMessage string
	var phase lunarwayv1alpha1.PostgreSQLDatabasePhase
	switch {
	case err == nil:
		phase = lunarwayv1alpha1.PostgreSQLDatabasePhaseRunning
	case err != nil:
		errorMessage = err.Error()
		if ctlerrors.IsInvalid(err) {
			phase = lunarwayv1alpha1.PostgreSQLDatabasePhaseInvalid
		} else {
			phase = lunarwayv1alpha1.PostgreSQLDatabasePhaseFailed
		}
	}
	phaseEqual := s.database.Status.Phase == phase
	errorEqual := s.database.Status.Error == errorMessage
	hostEqual := s.database.Status.Host == s.host
	if phaseEqual && errorEqual && hostEqual {
		return false
	}
	s.database.Status.PhaseUpdated = s.now()
	s.database.Status.Phase = phase
	s.database.Status.Host = s.host
	s.database.Status.Error = errorMessage
	return true
}

// stopRequeueOnInvalid detects if err should stop requeing of a request.
func stopRequeueOnInvalid(log logr.Logger, err error) error {
	if !ctlerrors.IsInvalid(err) {
		return err
	}
	if ctlerrors.IsTemporary(err) {
		return err
	}
	log.Error(err, "Dropping resources from queue as it is invalid")
	return nil
}

func (r *ReconcilePostgreSQLDatabase) EnsurePostgreSQLDatabase(log logr.Logger, host, name, password string) error {
	credentials, ok := r.hostCredentials[host]
	if !ok {
		return &ctlerrors.Invalid{
			Err: fmt.Errorf("unknown credentials for host"),
		}
	}
	connectionString := postgres.ConnectionString{
		Host:     host,
		Database: "postgres", // default database
		User:     credentials.Name,
		Password: credentials.Password,
	}
	db, err := postgres.Connect(log, connectionString)
	if err != nil {
		return fmt.Errorf("connect to host %s: %w", connectionString, err)
	}
	err = postgres.Database(log, db, host, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("create database %s on host %s: %w", name, connectionString, err)
	}
	return nil
}
