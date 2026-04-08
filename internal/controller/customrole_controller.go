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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=customroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=customroles/status,verbs=get;update;patch
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

	reqLogger = reqLogger.WithValues("roleName", customRole.Spec.RoleName)
	reqLogger.V(1).Info("Reconciling CustomRole resource")

	grants := toPostgresGrants(customRole.Spec.Grants)

	for host, creds := range r.HostCredentials {
		if err := r.reconcileOnHost(reqLogger, host, creds, customRole.Spec.RoleName, customRole.Spec.GrantRoles, grants); err != nil {
			r.persistStatus(ctx, customRole, err)
			return fmt.Errorf("reconcile on host %s: %w", host, err)
		}
	}

	r.persistStatus(ctx, customRole, nil)
	return nil
}

func (r *CustomRoleReconciler) reconcileOnHost(log logr.Logger, host string, creds postgres.Credentials, roleName string, grantRoles []string, grants []postgres.CustomRoleGrant) error {
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

	// Apply per-database grants across all user databases on this host.
	if len(grants) > 0 {
		databases, err := postgres.UserDatabases(adminDB)
		if err != nil {
			return fmt.Errorf("list databases: %w", err)
		}

		for _, dbName := range databases {
			if err := r.applyGrantsOnDatabase(log, host, creds, roleName, dbName, grants); err != nil {
				return fmt.Errorf("apply grants on database %s: %w", dbName, err)
			}
		}
	}

	return nil
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

func (r *CustomRoleReconciler) applyGrantsOnDatabase(log logr.Logger, host string, adminCredentials postgres.Credentials, roleName, dbName string, grants []postgres.CustomRoleGrant) error {
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

	return postgres.ApplyDatabaseGrants(log, db, roleName, grants)
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
