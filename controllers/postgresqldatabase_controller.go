/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

// PostgreSQLDatabaseReconciler reconciles a PostgreSQLDatabase object
type PostgreSQLDatabaseReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	// contains a map of credentials for hosts
	HostCredentials map[string]postgres.Credentials
}

// +kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqldatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqldatabases/status,verbs=get;update;patch

func (r *PostgreSQLDatabaseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	status, err := r.reconcile(reqLogger, req)
	status.Persist(err, r.Log)

	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLDatabase object")
	}
	return ctrl.Result{}, stopRequeueOnInvalid(reqLogger, err)
}

func (r *PostgreSQLDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLDatabase{}).
		Complete(r)
}

func (r *PostgreSQLDatabaseReconciler) reconcile(reqLogger logr.Logger, request reconcile.Request) (status, error) {
	reqLogger.Info("Reconciling PostgreSQLDatabase")
	// Fetch the PostgreSQLDatabase instance
	database := &postgresqlv1alpha1.PostgreSQLDatabase{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, database)
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
	reqLogger = reqLogger.WithValues(
		"database", database.Spec.Name,
		"isShared", database.Spec.IsShared,
	)
	reqLogger.Info("Updating PostgreSQLDatabase resource")

	status := status{
		log:      reqLogger,
		client:   r.Client,
		now:      metav1.Now,
		database: database,
	}
	host, err := kube.ResourceValue(r.Client, database.Spec.Host, request.Namespace)
	if err != nil {
		return status, fmt.Errorf("resolve host reference: %w", err)
	}
	status.host = host
	reqLogger = reqLogger.WithValues("host", host)
	user, err := kube.ResourceValue(r.Client, database.Spec.User, request.Namespace)
	if err != nil {
		if !ctlerrors.IsInvalid(err) {
			return status, fmt.Errorf("resolve user reference: %w", err)
		}
		// backwards compatibility to support resources without a User
		reqLogger.Info("User name fallback to database name")
		user = database.Spec.Name
	}
	status.user = user
	reqLogger = reqLogger.WithValues("user", user)
	password, err := kube.ResourceValue(r.Client, database.Spec.Password, request.Namespace)
	if err != nil {
		return status, fmt.Errorf("resolve password reference: %w", err)
	}
	isShared := database.Spec.IsShared

	reqLogger.Info("Resolved all referenced values for PostgreSQLDatabase resource")

	// Ensure the database is in sync with the object
	err = r.EnsurePostgreSQLDatabase(reqLogger, host, database.Spec.Name, user, password, isShared)
	if err != nil {
		return status, fmt.Errorf("ensure database: %w", err)
	}
	return status, nil
}

type status struct {
	log    logr.Logger
	client client.Client
	now    func() metav1.Time

	database *postgresqlv1alpha1.PostgreSQLDatabase
	host     string
	user     string
}

// Persist writes the status to a PostgreSQLDatabase instance and persists it on
// client. Any errors are logged.
func (s *status) Persist(err error, log logr.Logger) {
	ok := s.update(err)
	if !ok {
		return
	}
	err = s.client.Status().Update(context.TODO(), s.database)
	if err != nil {
		log.Error(err, "failed to set status of database", "status", s)
	}
}

// update updates database reference based on its values and returns whether any
// changes were written.
func (s *status) update(err error) bool {
	var errorMessage string
	var phase postgresqlv1alpha1.PostgreSQLDatabasePhase
	switch {
	case err == nil:
		phase = postgresqlv1alpha1.PostgreSQLDatabasePhaseRunning
	case err != nil:
		errorMessage = err.Error()
		if ctlerrors.IsInvalid(err) {
			phase = postgresqlv1alpha1.PostgreSQLDatabasePhaseInvalid
		} else {
			phase = postgresqlv1alpha1.PostgreSQLDatabasePhaseFailed
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
	s.database.Status.User = s.user
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

func (r *PostgreSQLDatabaseReconciler) EnsurePostgreSQLDatabase(log logr.Logger, host, name, user, password string, isShared bool) error {
	credentials, ok := r.HostCredentials[host]
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
		Params:   credentials.Params,
	}
	db, err := postgres.Connect(log, connectionString)
	if err != nil {
		return fmt.Errorf("connect to host %s: %w", connectionString, err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Error(err, "failed to close database connection", "host", host, "database", "postgres", "user", credentials.Name)
		}
	}()
	err = postgres.Database(log, db, host, postgres.Credentials{
		Name:     name,
		User:     user,
		Password: password,
		Shared:   isShared,
	})
	if err != nil {
		return fmt.Errorf("create database %s on host %s: %w", name, connectionString, err)
	}
	return nil
}
