/*
Copyright 2021.

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

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

// PostgreSQLDatabaseReconciler reconciles a PostgreSQLDatabase object
type PostgreSQLDatabaseReconciler struct {
	client.Client
	Log logr.Logger

	ManagerRoleName string
	// contains a map of credentials for hosts
	HostCredentials map[string]postgres.Credentials
}

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqldatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqldatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list

func (r *PostgreSQLDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	status, err := r.reconcile(ctx, reqLogger, req)
	status.Persist(ctx, err, r.Log)

	return requeueStrategy(reqLogger, err)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLDatabase{}).
		Complete(r)
}

func (r *PostgreSQLDatabaseReconciler) reconcile(ctx context.Context, reqLogger logr.Logger, request reconcile.Request) (status, error) {
	reqLogger.Info("Reconciling PostgreSQLDatabase")
	// Fetch the PostgreSQLDatabase instance
	database := &postgresqlv1alpha1.PostgreSQLDatabase{}
	err := r.Client.Get(ctx, request.NamespacedName, database)
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
	host, adminCredentials, err := r.adminCredentials(ctx, reqLogger, &adminCredentialsParams{
		namespace:       request.NamespacedName.Namespace,
		host:            database.Spec.Host,
		hostCredentials: database.Spec.HostCredentials,
	})
	if err != nil {
		return status, fmt.Errorf("determining host credentials: %w", err)
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
	password := ""
	if database.Spec.Password != nil {
		password, err = kube.ResourceValue(r.Client, *database.Spec.Password, request.Namespace)
		if err != nil {
			return status, fmt.Errorf("resolve password reference: %w", err)
		}
	}
	isShared := database.Spec.IsShared
	extensions := database.Spec.Extensions

	reqLogger.Info("Resolved all referenced values for PostgreSQLDatabase resource")

	// Ensure the database is in sync with the object
	err = r.EnsurePostgreSQLDatabase(
		ctx,
		reqLogger,
		&EnsureParams{
			Host:        host,
			Admin:       *adminCredentials,
			ManagerRole: r.ManagerRoleName,
			Extensions:  fromApiExtensions(extensions),
			Target: postgres.Credentials{
				Name:     database.Spec.Name,
				User:     user,
				Password: password,
				Shared:   isShared,
			},
		},
	)
	if err != nil {
		return status, fmt.Errorf("ensure database: %w", err)
	}

	return status, nil
}

func fromApiExtensions(extensions []postgresqlv1alpha1.PostgreSQLDatabaseExtension) postgres.Extensions {
	postgresExtensions := make([]postgres.Extension, 0, len(extensions))

	for _, e := range extensions {
		postgresExtensions = append(postgresExtensions, postgres.NewExtension(e.ExtensionName))
	}

	return postgresExtensions
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
func (s *status) Persist(ctx context.Context, err error, log logr.Logger) {
	ok := s.update(err)
	if !ok {
		return
	}

	err = s.client.Status().Update(ctx, s.database)
	if err != nil {
		log.Error(err, "failed to set status of database", "status", s)
	}
}

// update updates database reference based on its values and returns whether any
// changes were written.
func (s *status) update(err error) bool {
	if s.database == nil {
		return false
	}

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

// requeueStrategy returns the right strategy for handling any errors from the
// reconciliation loop.
func requeueStrategy(log logr.Logger, err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	// if resource is invalid there is nothing we can do about it and we rely on
	// the client to update the resource before we touch it again
	if ctlerrors.IsInvalid(err) {
		log.Info("Dropping resources from queue as it is invalid", "error", err)
		return reconcile.Result{}, nil
	}

	// if it is a temporary error, eg. secret not found, we try again shortly as
	// the secret might have appeared
	if ctlerrors.IsTemporary(err) {
		log.Info("Failed to reconcile PostgreSQLDatabase object, attempting again shortly", "error", err)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	// if it is an unknown error we try again later as it might be an unidentified
	// temporary issue, eg. no access to Kubernetes API.
	log.Info("Failed to reconcile PostgreSQLDatabase object due unknown error", "error", err)
	return ctrl.Result{
		RequeueAfter: 10 * time.Second,
	}, nil
}

// EnsureParams contains the required parameters for
// `PostgreSQLDatabaseReconciler.EnsurePostgreSQLDatabase()`.
type EnsureParams struct {
	// Host is the host name for the database instance.
	Host string

	// Admin holds the administrator credentials for the database instance.
	Admin postgres.Credentials

	ManagerRole string

	// Extensions is a list of extensions expected to be enabled. Extensions outside of this list won't be disabled
	Extensions []postgres.Extension

	// Target contains the credentials for the Postgres database that we intend
	// to create.
	Target postgres.Credentials
}

func (r *PostgreSQLDatabaseReconciler) EnsurePostgreSQLDatabase(ctx context.Context, log logr.Logger, params *EnsureParams) error {
	err := postgres.Database(log, params.Host, params.Admin, params.Target, params.ManagerRole, params.Extensions)
	if err != nil {
		return fmt.Errorf("create database %s on host %s: %w", params.Target.Name, params.Host, err)
	}

	return nil
}

type adminCredentialsParams struct {
	// namespace is the namespace containing the target PostgreSQLDatabase
	// resource.
	namespace string

	// host is the host name of the host. If it is provided, its value must be
	// a key in the `PostgreSQLDatabaseReconciler`'s `HostCredentials` map
	// field. It must not be provided if `HostCredentials` is provided.
	host postgresqlv1alpha1.ResourceVar

	// hostCredentials is the name of the `PostgreSQLHostCredentials` resource
	// in the same namespace. It must not be provided if `Host` is provided.
	hostCredentials string
}

// adminCredentials returns the correct `postgres.Credentials` based on the
// values of `params.Host` and `params.HostCredentials` fields of `params`.
// Exactly one of these fields must be set, and if `Host` is set, then this
// method will return the adminCredentials from `r.HostCredentials` map,
// otherwise it will search the Kubernetes namespace for a
// `PostgreSQLHostCredentials` with the name specified in
// `params.HostCredentials`.
func (r *PostgreSQLDatabaseReconciler) adminCredentials(ctx context.Context, reqLogger logr.Logger, params *adminCredentialsParams) (string, *postgres.Credentials, error) {
	host, err := kube.ResourceValue(r.Client, params.host, params.namespace)
	if err != nil {
		// if the `host` value is missing, we want to keep going because it
		// should mean that the `hostCredentials` is provided.
		if !errors.Is(err, kube.ErrNoValue) {
			return "", nil, fmt.Errorf("resolve host reference: %w", err)
		}
	}

	// If the `HostCredentials` field is populated and the `Host` field is
	// empty, then return credentials stored in the corresponding
	// `PostgreSQLHostCredentials` resource.
	if host == "" && params.hostCredentials != "" {
		reqLogger.Info(fmt.Sprintf("Using remote host credential from PostgreSQLHostCredentials resource %s/%s", params.namespace, params.hostCredentials))
		host, credentials, err := r.remoteCredentials(ctx, params.namespace, params.hostCredentials)
		if err != nil {
			return "", nil, fmt.Errorf("get remote credentials %s/%s: %w", params.namespace, params.hostCredentials, err)
		}
		return host, credentials, nil
	}

	// If the `Host` field is populated but no the `HostCredentials` field,
	// then return the credentials from the `r.HostCredentials` map.
	if params.hostCredentials == "" && host != "" {
		reqLogger.Info("Using local host credential from controller arguments")
		cs, ok := r.HostCredentials[host]
		if !ok {
			return "", nil, ctlerrors.NewInvalid(fmt.Errorf("unknown credentials for host"))
		}
		return host, &cs, nil
	}

	// If we got here, neither or both of the fields are populated. Return an
	// error.
	return "", nil, ctlerrors.NewInvalid(fmt.Errorf("must specify exactly one of `host` and `hostCredentials`"))
}

// remoteCredentials resolves the credentials from a `PostgreSQLHostCredentials`
// resource.
func (r *PostgreSQLDatabaseReconciler) remoteCredentials(ctx context.Context, namespace, name string) (string, *postgres.Credentials, error) {
	// Fetch the `PostgreSQLHostCredentials` from the API.
	var hostCreds postgresqlv1alpha1.PostgreSQLHostCredentials
	err := r.Client.Get(
		ctx,
		types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
		&hostCreds,
	)
	if err != nil {
		return "", nil, fmt.Errorf("get PostgreSQLHostCredentials resource: %w", err)
	}

	// Resolve the `user` field.
	user, err := kube.ResourceValue(r.Client, hostCreds.Spec.User, hostCreds.Namespace)
	if err != nil {
		return "", nil, fmt.Errorf("resolve user resource var: %w", err)
	}

	// Resolve the `password` field.
	password, err := kube.ResourceValue(r.Client, hostCreds.Spec.Password, hostCreds.Namespace)
	if err != nil {
		return "", nil, fmt.Errorf("resolve password resource var: %w", err)
	}

	// Resolve the `host` field.
	host, err := kube.ResourceValue(r.Client, hostCreds.Spec.Host, hostCreds.Namespace)
	if err != nil {
		return "", nil, fmt.Errorf("resolve host resource var: %w", err)
	}

	// Return the resulting host and credentials
	return host, &postgres.Credentials{
		Name:     user,
		User:     user,
		Password: password,
		Params:   hostCreds.Spec.Params,
	}, nil
}
