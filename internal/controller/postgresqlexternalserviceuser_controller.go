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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"

	"github.com/go-logr/logr"
)

const externalServiceUserFinalizer = "postgresqlexternalserviceuser.lunar.tech/finalizer"

// PostgreSQLExternalServiceUserReconciler reconciles a PostgreSQLExternalServiceUser object.
type PostgreSQLExternalServiceUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	EnsureIAMExternalServiceUser func(client *iam.Client, log logr.Logger, config iam.EnsureExternalServiceUserConfig, principalArn, dbUsername string) error
	RemoveIAMExternalServiceUser func(client *iam.Client, log logr.Logger, config iam.EnsureExternalServiceUserConfig, dbUsername string) error

	AWSPolicyName      string
	AWSRegion          string
	AWSAccountID       string
	AWSProfile         string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	IAMPolicyPrefix    string
	AWSLoginRoles      []string
	HostCredentials    map[string]postgres.Credentials
}

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlexternalserviceusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlexternalserviceusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlexternalserviceusers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list

func (r *PostgreSQLExternalServiceUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("Reconciling PostgreSQLExternalServiceUser")

	result, err := r.reconcile(ctx, reqLogger, req)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLExternalServiceUser")
	}
	return result, err
}

func (r *PostgreSQLExternalServiceUserReconciler) reconcile(ctx context.Context, reqLogger logr.Logger, req reconcile.Request) (ctrl.Result, error) {
	obj := &postgresqlv1alpha1.PostgreSQLExternalServiceUser{}
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	reqLogger = reqLogger.WithValues("principalArn", obj.Spec.PrincipalArn, "dbUsername", obj.Spec.DBUsername)

	// Build the AWS IAM client used for policy management.
	awsCfg := &aws.Config{
		Region:      aws.String(r.AWSRegion),
		Credentials: r.getCredentials(),
	}
	awsSession, err := session.NewSession(awsCfg)
	if err != nil {
		return ctrl.Result{}, r.persistFailed(ctx, obj, fmt.Errorf("initialise AWS session for region %s: %w", r.AWSRegion, err))
	}
	iamClient := iam.NewClient(awsSession, reqLogger, r.AWSAccountID, r.IAMPolicyPrefix)

	iamConfig := iam.EnsureExternalServiceUserConfig{
		Region:         r.AWSRegion,
		AccountID:      r.AWSAccountID,
		PolicyBaseName: r.AWSPolicyName,
		AWSLoginRoles:  r.AWSLoginRoles,
	}

	// Handle deletion via finalizer.
	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, externalServiceUserFinalizer) {
			reqLogger.V(1).Info("Cleaning up PostgreSQLExternalServiceUser before deletion")

			if err := r.RemoveIAMExternalServiceUser(iamClient, reqLogger, iamConfig, obj.Spec.DBUsername); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove IAM policy for %s: %w", obj.Spec.DBUsername, err)
			}

			// Drop the Postgres role. Logged but non-blocking: if the host is
			// unavailable the IAM policy has already been removed and the role
			// will be inert.
			if err := r.dropPostgresRole(ctx, reqLogger, req.Namespace, obj.Spec.Host, obj.Spec.DBUsername); err != nil {
				reqLogger.Error(err, "Failed to drop Postgres role during deletion; continuing")
			}

			controllerutil.RemoveFinalizer(obj, externalServiceUserFinalizer)
			if err := r.Update(ctx, obj); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is registered before doing any work.
	if !controllerutil.ContainsFinalizer(obj, externalServiceUserFinalizer) {
		controllerutil.AddFinalizer(obj, externalServiceUserFinalizer)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// If spec.dbUsername was renamed, clean up the stale IAM policy and Postgres
	// role before creating resources under the new name.
	if obj.Status != nil && obj.Status.DBUsername != "" && obj.Status.DBUsername != obj.Spec.DBUsername {
		previousUsername := obj.Status.DBUsername
		reqLogger.Info("dbUsername changed, cleaning up previous resources", "previous", previousUsername, "new", obj.Spec.DBUsername)

		if err := r.RemoveIAMExternalServiceUser(iamClient, reqLogger, iamConfig, previousUsername); err != nil {
			return ctrl.Result{}, r.persistFailed(ctx, obj, fmt.Errorf("remove old IAM policy for %s: %w", previousUsername, err))
		}
		if err := r.dropPostgresRole(ctx, reqLogger, req.Namespace, obj.Spec.Host, previousUsername); err != nil {
			reqLogger.Error(err, "Failed to drop previous Postgres role after rename; continuing", "role", previousUsername)
		}
	}

	// Connect to the target Postgres instance.
	db, err := r.connectPostgres(ctx, req.Namespace, obj)
	if err != nil {
		return ctrl.Result{}, r.persistFailed(ctx, obj, fmt.Errorf("connect to postgres: %w", err))
	}
	defer db.Close()

	// Build the desired Postgres role list: rds_iam is always required for
	// IAM-based RDS authentication, plus any additional roles from the spec.
	desiredRoles := []string{"rds_iam"}
	for _, role := range obj.Spec.Roles {
		desiredRoles = append(desiredRoles, role.RoleName)
	}

	// Ensure the Postgres LOGIN role exists and has the correct grants.
	if err := postgres.EnsureIAMLoginRole(reqLogger, db, obj.Spec.DBUsername, desiredRoles); err != nil {
		return ctrl.Result{}, r.persistFailed(ctx, obj, fmt.Errorf("ensure postgres role %s: %w", obj.Spec.DBUsername, err))
	}

	// Ensure the IAM policy exists and is attached to the login roles.
	if err := r.EnsureIAMExternalServiceUser(iamClient, reqLogger, iamConfig, obj.Spec.PrincipalArn, obj.Spec.DBUsername); err != nil {
		return ctrl.Result{}, r.persistFailed(ctx, obj, fmt.Errorf("ensure IAM policy for %s: %w", obj.Spec.DBUsername, err))
	}

	return ctrl.Result{}, r.persistRunning(ctx, obj)
}

// connectPostgres resolves the host ResourceVar and opens a connection using the
// matching entry in HostCredentials.
func (r *PostgreSQLExternalServiceUserReconciler) connectPostgres(ctx context.Context, namespace string, obj *postgresqlv1alpha1.PostgreSQLExternalServiceUser) (*sql.DB, error) {
	host, err := kube.ResourceValue(r.Client, obj.Spec.Host, namespace)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}

	creds, ok := r.HostCredentials[host]
	if !ok {
		return nil, fmt.Errorf("no credentials configured for host %q", host)
	}

	connStr := postgres.ConnectionString{
		Host:     host,
		Database: creds.Name,
		User:     creds.User,
		Password: creds.Password,
		Params:   creds.Params,
	}
	db, err := postgres.Connect(connStr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", connStr, err)
	}
	return db, nil
}

// dropPostgresRole connects to Postgres via the given host ResourceVar and drops
// the named role. Accepting host and roleName separately allows it to be called
// with a previous username during a rename, or with spec values during deletion.
func (r *PostgreSQLExternalServiceUserReconciler) dropPostgresRole(ctx context.Context, log logr.Logger, namespace string, host postgresqlv1alpha1.ResourceVar, roleName string) error {
	resolvedHost, err := kube.ResourceValue(r.Client, host, namespace)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	creds, ok := r.HostCredentials[resolvedHost]
	if !ok {
		return fmt.Errorf("no credentials configured for host %q", resolvedHost)
	}
	db, err := postgres.Connect(postgres.ConnectionString{
		Host:     resolvedHost,
		Database: creds.Name,
		User:     creds.User,
		Password: creds.Password,
		Params:   creds.Params,
	})
	if err != nil {
		return fmt.Errorf("connect to %s: %w", resolvedHost, err)
	}
	defer db.Close()
	return postgres.DropCustomRole(log, db, roleName)
}

// getCredentials returns AWS credentials from either a shared profile or
// static key/secret, mirroring the PostgreSQLUserReconciler approach.
func (r *PostgreSQLExternalServiceUserReconciler) getCredentials() *credentials.Credentials {
	if r.AWSProfile != "" {
		return credentials.NewSharedCredentials("", r.AWSProfile)
	}
	return credentials.NewStaticCredentials(r.AWSAccessKeyID, r.AWSSecretAccessKey, "")
}

// persistRunning updates the resource status to Running after a successful reconcile.
// DBUsername is recorded so the controller can detect renames on the next reconcile.
func (r *PostgreSQLExternalServiceUserReconciler) persistRunning(ctx context.Context, obj *postgresqlv1alpha1.PostgreSQLExternalServiceUser) error {
	now := metav1.Now()
	obj.Status = &postgresqlv1alpha1.PostgreSQLExternalServiceUserStatus{
		ObservedGeneration: obj.Generation,
		DBUsername:         obj.Spec.DBUsername,
		Conditions: []postgresqlv1alpha1.PostgreSQLExternalServiceUserCondition{
			{
				Type:               postgresqlv1alpha1.PostgreSQLExternalServiceUserPhaseRunning,
				Status:             apiv1.ConditionTrue,
				LastUpdateTime:     now,
				LastTransitionTime: now,
				Message:            "Reconciled successfully",
			},
		},
	}
	if err := r.Client.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("update status to Running: %w", err)
	}
	return nil
}

// persistFailed updates the resource status to Failed and returns the original error
// so the reconcile loop requeues.
func (r *PostgreSQLExternalServiceUserReconciler) persistFailed(ctx context.Context, obj *postgresqlv1alpha1.PostgreSQLExternalServiceUser, reconcileErr error) error {
	now := metav1.Now()
	obj.Status = &postgresqlv1alpha1.PostgreSQLExternalServiceUserStatus{
		ObservedGeneration: obj.Generation,
		Conditions: []postgresqlv1alpha1.PostgreSQLExternalServiceUserCondition{
			{
				Type:               postgresqlv1alpha1.PostgreSQLExternalServiceUserPhaseFailed,
				Status:             apiv1.ConditionTrue,
				LastUpdateTime:     now,
				LastTransitionTime: now,
				Message:            reconcileErr.Error(),
			},
		},
	}
	if err := r.Client.Status().Update(ctx, obj); err != nil {
		// Log but don't mask the original error.
		log.Log.Error(err, "Failed to update status to Failed")
	}
	return reconcileErr
}

// SetupWithManager registers the controller with the Manager.
func (r *PostgreSQLExternalServiceUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLExternalServiceUser{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
