/*
Copyright 2024.

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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	ctlerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

// CustomRoleReconciler reconciles a CustomRole object
type CustomRoleReconciler struct {
	client.Client
	Log logr.Logger

	// HostCredentials contains a map of credentials for hosts (keyed by host name)
	HostCredentials map[string]postgres.Credentials
}

const customRoleFinalizer = "customrole.postgresql.lunar.tech/finalizer"

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=customroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=customroles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=customroles/finalizers,verbs=update
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqldatabases,verbs=list;watch
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list

func (r *CustomRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())

	err = r.reconcile(ctx, reqLogger, req)
	return customRoleRequeueStrategy(reqLogger, err)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.CustomRole{}).
		Watches(
			&postgresqlv1alpha1.PostgreSQLDatabase{},
			handler.EnqueueRequestsFromMapFunc(r.mapDatabaseToCustomRoles),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(_ event.CreateEvent) bool { return true },
				// Fire when a database transitions to Running so that any grants that
				// were skipped (because the database did not yet exist on the server
				// when the CreateFunc fired) are applied as soon as it is ready.
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldDB, ok1 := e.ObjectOld.(*postgresqlv1alpha1.PostgreSQLDatabase)
					newDB, ok2 := e.ObjectNew.(*postgresqlv1alpha1.PostgreSQLDatabase)
					if !ok1 || !ok2 {
						return false
					}
					return oldDB.Status.Phase != postgresqlv1alpha1.PostgreSQLDatabasePhaseRunning &&
						newDB.Status.Phase == postgresqlv1alpha1.PostgreSQLDatabasePhaseRunning
				},
				DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
				GenericFunc: func(_ event.GenericEvent) bool { return false },
			}),
		).
		Complete(r)
}

// mapDatabaseToCustomRoles enqueues all CustomRole objects in the same namespace
// whenever a PostgreSQLDatabase resource changes.
func (r *CustomRoleReconciler) mapDatabaseToCustomRoles(ctx context.Context, obj client.Object) []reconcile.Request {
	var customRoles postgresqlv1alpha1.CustomRoleList
	if err := r.Client.List(ctx, &customRoles, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, len(customRoles.Items))
	for i, cr := range customRoles.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Name,
			},
		}
	}
	return requests
}

func (r *CustomRoleReconciler) reconcile(ctx context.Context, reqLogger logr.Logger, req ctrl.Request) error {
	reqLogger.V(1).Info("Reconciling CustomRole")

	customRole := &postgresqlv1alpha1.CustomRole{}
	err := r.Client.Get(ctx, req.NamespacedName, customRole)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	roleName := customRole.Name
	reqLogger = reqLogger.WithValues("roleName", roleName)

	// Handle deletion: clean up the PostgreSQL role and its grants before
	// allowing Kubernetes to remove the object.
	if !customRole.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(customRole, customRoleFinalizer) {
			reqLogger.V(1).Info("Cleaning up CustomRole before deletion")
			if err := r.cleanupRole(ctx, reqLogger, roleName); err != nil {
				return fmt.Errorf("cleanup role: %w", err)
			}
			controllerutil.RemoveFinalizer(customRole, customRoleFinalizer)
			if err := r.Update(ctx, customRole); err != nil {
				return fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return nil
	}

	// Ensure the finalizer is present so we can clean up on deletion.
	if !controllerutil.ContainsFinalizer(customRole, customRoleFinalizer) {
		controllerutil.AddFinalizer(customRole, customRoleFinalizer)
		if err := r.Update(ctx, customRole); err != nil {
			return fmt.Errorf("add finalizer: %w", err)
		}
		return nil
	}

	reqLogger.V(1).Info("Reconciling CustomRole resource")

	grants := toPostgresGrants(customRole.Spec.Grants)
	functions := toPostgresFunctions(customRole.Spec.Functions)

	for host, creds := range r.HostCredentials {
		if err := r.reconcileOnHost(reqLogger, host, creds, roleName, customRole.Spec.GrantRoles, customRole.Spec.Databases, grants, functions); err != nil {
			r.persistStatus(ctx, customRole, err)
			return fmt.Errorf("reconcile on host %s: %w", host, err)
		}
	}

	r.persistStatus(ctx, customRole, nil)
	return nil
}

func (r *CustomRoleReconciler) cleanupRole(_ context.Context, log logr.Logger, roleName string) error {
	for host, creds := range r.HostCredentials {
		if err := r.cleanupRoleOnHost(log, host, creds, roleName); err != nil {
			return fmt.Errorf("cleanup on host %s: %w", host, err)
		}
	}
	return nil
}

func (r *CustomRoleReconciler) cleanupRoleOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string) error {
	adminConnStr := postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     creds.User,
		Password: creds.Password,
		Params:   creds.Params,
	}
	adminDB, err := postgres.Connect(log, adminConnStr)
	if err != nil {
		return fmt.Errorf("connect to host: %w", err)
	}
	defer adminDB.Close()

	databases, err := postgres.UserDatabases(adminDB)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
	}
	for _, dbName := range databases {
		connStr := postgres.ConnectionString{
			Host:     host,
			Database: dbName,
			User:     creds.User,
			Password: creds.Password,
			Params:   creds.Params,
		}
		db, err := postgres.Connect(log, connStr)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dbName, err)
		}
		dropErr := postgres.DropManagedFunctions(log, db, roleName)
		if dropErr != nil {
			db.Close()
			return fmt.Errorf("drop functions in database %s: %w", dbName, dropErr)
		}
		revokeErr := postgres.RevokeAllDatabaseGrants(log, db, roleName)
		if closeErr := db.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close database connection", "database", dbName)
		}
		if revokeErr != nil {
			return fmt.Errorf("revoke grants in database %s: %w", dbName, revokeErr)
		}
	}

	// Drop functions scoped to the postgres database.
	if err := postgres.DropManagedFunctions(log, adminDB, roleName); err != nil {
		return fmt.Errorf("drop functions in postgres database: %w", err)
	}

	return postgres.DropCustomRole(log, adminDB, roleName)
}

func (r *CustomRoleReconciler) reconcileOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string, grantRoles []string, targetDatabases []string, grants []postgres.CustomRoleGrant, functions []postgres.CustomRoleFunction) error {
	log = log.WithValues("host", host)

	// Connect to the postgres maintenance database for server-level operations.
	adminConnStr := postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     creds.User,
		Password: creds.Password,
		Params:   creds.Params,
	}
	adminDB, err := postgres.Connect(log, adminConnStr)
	if err != nil {
		return fmt.Errorf("connect to host: %w", err)
	}
	defer adminDB.Close()

	// Create the role and apply server-level role grants.
	if err := postgres.EnsureCustomRole(log, adminDB, roleName, grantRoles); err != nil {
		return fmt.Errorf("ensure role: %w", err)
	}

	// Determine which databases to apply grants and functions to.
	// If targetDatabases is set, use exactly those (may include "postgres").
	// Otherwise, apply to all user databases.
	var databases []string
	if len(targetDatabases) > 0 {
		databases = targetDatabases
	} else {
		databases, err = postgres.UserDatabases(adminDB)
		if err != nil {
			return fmt.Errorf("list databases: %w", err)
		}
	}

	for _, dbName := range databases {
		if dbName == "postgres" {
			// Reuse the existing admin connection for the postgres database.
			if err := postgres.SyncDatabaseGrants(log, adminDB, roleName, grants); err != nil {
				return fmt.Errorf("sync grants on database postgres: %w", err)
			}
			if err := postgres.SyncDatabaseFunctions(log, adminDB, roleName, functions); err != nil {
				return fmt.Errorf("sync functions on database postgres: %w", err)
			}
			continue
		}
		if err := r.syncGrantsOnDatabase(log, host, creds, roleName, dbName, grants); err != nil {
			return fmt.Errorf("sync grants on database %s: %w", dbName, err)
		}
		if err := r.syncFunctionsOnDatabase(log, host, creds, roleName, dbName, functions); err != nil {
			return fmt.Errorf("sync functions on database %s: %w", dbName, err)
		}
	}

	return nil
}

func (r *CustomRoleReconciler) syncFunctionsOnDatabase(log logr.Logger, host string, adminCredentials postgres.Credentials, roleName, dbName string, functions []postgres.CustomRoleFunction) error {
	connStr := postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	db, err := postgres.Connect(log, connStr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", connStr, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error(err, "failed to close database connection", "database", dbName)
		}
	}()

	return postgres.SyncDatabaseFunctions(log, db, roleName, functions)
}

func toPostgresGrants(grants []postgresqlv1alpha1.CustomRoleGrant) []postgres.CustomRoleGrant {
	result := make([]postgres.CustomRoleGrant, len(grants))
	for i, g := range grants {
		result[i] = postgres.CustomRoleGrant{
			Schema:     g.Schema,
			Table:      g.Table,
			Privileges: g.Privileges,
		}
	}
	return result
}

func toPostgresFunctions(functions []postgresqlv1alpha1.CustomRoleFunction) []postgres.CustomRoleFunction {
	result := make([]postgres.CustomRoleFunction, len(functions))
	for i, f := range functions {
		result[i] = postgres.CustomRoleFunction{
			Name:    f.Name,
			Args:    f.Args,
			Returns: f.Returns,
			Body:    f.Body,
		}
	}
	return result
}

func (r *CustomRoleReconciler) syncGrantsOnDatabase(log logr.Logger, host string, adminCredentials postgres.Credentials, roleName, dbName string, grants []postgres.CustomRoleGrant) error {
	connStr := postgres.ConnectionString{
		Host:     host,
		Database: dbName,
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	db, err := postgres.Connect(log, connStr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", connStr, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error(err, "failed to close database connection", "database", dbName)
		}
	}()

	return postgres.SyncDatabaseGrants(log, db, roleName, grants)
}

func (r *CustomRoleReconciler) persistStatus(ctx context.Context, customRole *postgresqlv1alpha1.CustomRole, reconcileErr error) {
	var phase postgresqlv1alpha1.CustomRolePhase
	var errorMessage string

	switch {
	case reconcileErr == nil:
		phase = postgresqlv1alpha1.CustomRolePhaseRunning
	case ctlerrors.IsInvalid(reconcileErr):
		phase = postgresqlv1alpha1.CustomRolePhaseInvalid
		errorMessage = reconcileErr.Error()
	default:
		phase = postgresqlv1alpha1.CustomRolePhaseFailed
		errorMessage = reconcileErr.Error()
	}

	if customRole.Status.Phase == phase && customRole.Status.Error == errorMessage {
		return
	}

	customRole.Status.Phase = phase
	customRole.Status.PhaseUpdated = metav1.Now()
	customRole.Status.Error = errorMessage

	if err := r.Client.Status().Update(ctx, customRole); err != nil {
		r.Log.Error(err, "failed to update CustomRole status")
	}
}

func customRoleRequeueStrategy(log logr.Logger, err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	if ctlerrors.IsInvalid(err) {
		log.Info("Dropping CustomRole from queue as it is invalid", "error", err)
		return reconcile.Result{}, nil
	}

	if ctlerrors.IsTemporary(err) {
		log.Info("Failed to reconcile CustomRole object, attempting again shortly", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	log.Info("Failed to reconcile CustomRole object due to unknown error", "error", err)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
