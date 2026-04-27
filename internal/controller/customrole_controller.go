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
	"database/sql"
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

	roleName := customRole.Spec.RoleName
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
			r.persistStatus(ctx, customRole, host, err)
			return fmt.Errorf("reconcile on host %s: %w", host, err)
		}
	}

	r.persistStatus(ctx, customRole, "", nil)
	return nil
}

func (r *CustomRoleReconciler) reconcileOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string, grantRoles []string, targetDatabases []string, grants []postgres.CustomRoleGrant, functions []postgres.CustomRoleFunction) error {
	log = log.WithValues("host", host)

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

	// Resolve the effective database list and, when scoped, all user databases
	// (so each domain can run its cleanup pass without an extra query).
	databases, allUserDatabases, err := resolveTargetDatabases(log, adminDB, targetDatabases)
	if err != nil {
		return err
	}

	if err := r.reconcileRoleOnHost(log, adminDB, roleName, grantRoles); err != nil {
		return err
	}
	if err := r.reconcileGrantsOnHost(log, host, creds, roleName, databases, allUserDatabases, grants); err != nil {
		return err
	}
	if err := r.reconcileFunctionsOnHost(log, host, creds, adminDB, roleName, databases, allUserDatabases, functions); err != nil {
		return err
	}
	return nil
}

// resolveTargetDatabases returns the effective database list for this
// reconcile cycle and, when targetDatabases is non-empty, all user databases
// (used by domain reconcilers for their cleanup passes).
// Databases in postgres.ReservedSystemDatabases are filtered from the explicit list.
func resolveTargetDatabases(log logr.Logger, adminDB *sql.DB, targetDatabases []string) (databases []string, allUserDatabases []string, err error) {
	if len(targetDatabases) > 0 {
		for _, db := range targetDatabases {
			if _, ok := postgres.ReservedSystemDatabases[db]; ok {
				log.Info("Skipping reserved system database from targetDatabases", "database", db)
				continue
			}
			databases = append(databases, db)
		}
		allUserDatabases, err = postgres.UserDatabases(adminDB)
		if err != nil {
			return nil, nil, fmt.Errorf("list databases for cleanup: %w", err)
		}
		return databases, allUserDatabases, nil
	}
	databases, err = postgres.UserDatabases(adminDB)
	if err != nil {
		return nil, nil, fmt.Errorf("list databases: %w", err)
	}
	return databases, nil, nil
}

func (r *CustomRoleReconciler) persistStatus(ctx context.Context, customRole *postgresqlv1alpha1.CustomRole, failingHost string, reconcileErr error) {
	var phase postgresqlv1alpha1.CustomRolePhase
	var errorMessage string

	switch {
	case reconcileErr == nil:
		phase = postgresqlv1alpha1.CustomRolePhaseRunning
		failingHost = ""
	case ctlerrors.IsInvalid(reconcileErr):
		phase = postgresqlv1alpha1.CustomRolePhaseInvalid
		errorMessage = reconcileErr.Error()
	default:
		phase = postgresqlv1alpha1.CustomRolePhaseFailed
		errorMessage = reconcileErr.Error()
	}

	if customRole.Status.Phase == phase &&
		customRole.Status.Error == errorMessage &&
		customRole.Status.FailingHost == failingHost {
		return
	}

	customRole.Status.Phase = phase
	customRole.Status.PhaseUpdated = metav1.Now()
	customRole.Status.Error = errorMessage
	customRole.Status.FailingHost = failingHost

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
