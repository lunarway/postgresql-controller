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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/iam"
)

// PostgreSQLUserReconciler reconciles a PostgreSQLUser object
type PostgreSQLUserReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Granter       grants.Granter
	EnsureIAMUser func(client *iam.Client, logger logr.Logger, config iam.EnsureUserConfig, username, rolename string) error
	RemoveIAMUser func(client *iam.Client, awsLoginRoles []string, username string) error

	RolePrefix         string
	AWSPolicyName      string
	AWSRegion          string
	AWSAccountID       string
	AWSProfile         string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	IAMPolicyPrefix    string
	AWSLoginRoles      []string
}

const userFinalizer = "postgresqluser.lunar.tech/finalizer"

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlusers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list

func (r *PostgreSQLUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	reqLogger.V(1).Info("Reconciling PostgreSQLUSer")

	result, err := r.reconcile(ctx, reqLogger, req)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLUser object")
	}
	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLUser{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). //explicitly set to 1 (which is also the default) because our reconciliation process is not necessarily concurrency safe.
		Complete(r)
}

func inList(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

func (r *PostgreSQLUserReconciler) reconcile(ctx context.Context, reqLogger logr.Logger, request reconcile.Request) (ctrl.Result, error) {
	// Fetch the PostgreSQLUser instance
	user := &postgresqlv1alpha1.PostgreSQLUser{}
	err := r.Client.Get(ctx, request.NamespacedName, user)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Object not found")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// User instance created or updated
	reqLogger = reqLogger.WithValues("user", user.Spec.Name, "rolePrefix", r.RolePrefix)
	reqLogger.V(1).Info("Reconciling found PostgreSQLUser resource", "user", user.Spec.Name)

	awsConfig := &aws.Config{
		Region:      aws.String(r.AWSRegion),
		Credentials: r.getCredentials(),
	}

	// Initialize session to AWS
	session, err := session.NewSession(awsConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("session initialization for region %s: %w", r.AWSRegion, err)
	}

	client := iam.NewClient(session, reqLogger, r.AWSAccountID, r.IAMPolicyPrefix)

	markedToBeDeleted := user.GetDeletionTimestamp() != nil
	if markedToBeDeleted {
		if !inList(user.Finalizers, userFinalizer) {
			return ctrl.Result{}, nil
		}
		// Run finalization logic for userFinalizer. If the
		// finalization logic fails, don't remove the finalizer so
		// that we can retry during the next reconciliation.
		if err := r.finalizeUser(reqLogger, client, user); err != nil {
			return ctrl.Result{}, err
		}

		// Remove finalizer. Once all finalizers have been
		// removed, the object will be deleted.
		controllerutil.RemoveFinalizer(user, userFinalizer)
		err := r.Update(ctx, user)
		if err != nil {
			return ctrl.Result{}, err
		}

		return requeueResult(), nil
	}

	// Add finalizer for this CR
	if !inList(user.Finalizers, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)

		err = r.Update(ctx, user)
		if err != nil {
			return ctrl.Result{}, err
		}

		return requeueResult(), nil
	}

	// We need to sanitize the user.Spec.Name to be a valid PostgreSQL role name
	sanitizedUser := sanitizedUser(user)

	// Error check in the bottom because we want aws policy to be set no matter what.
	granterErr := r.Granter.SyncUser(reqLogger, request.Namespace, r.RolePrefix, *sanitizedUser)

	awsPolicyErr := r.EnsureIAMUser(client, reqLogger, iam.EnsureUserConfig{
		PolicyBaseName:    r.AWSPolicyName,
		Region:            r.AWSRegion,
		AccountID:         r.AWSAccountID,
		MaxUsersPerPolicy: 30,
		RolePrefix:        r.RolePrefix,
		AWSLoginRoles:     r.AWSLoginRoles,
	}, user.Spec.Name, sanitizedUser.Spec.Name)

	if granterErr != nil || awsPolicyErr != nil {
		return ctrl.Result{}, fmt.Errorf("grantErr: %v, awsPolicyErr: %v", granterErr, awsPolicyErr)
	}

	return ctrl.Result{}, nil
}

func (r *PostgreSQLUserReconciler) getCredentials() *credentials.Credentials {
	var awsCredentials *credentials.Credentials
	if len(r.AWSProfile) != 0 {
		awsCredentials = credentials.NewSharedCredentials("", r.AWSProfile)
	} else {
		awsCredentials = credentials.NewStaticCredentials(r.AWSAccessKeyID, r.AWSSecretAccessKey, "")
	}
	return awsCredentials
}

func (r *PostgreSQLUserReconciler) finalizeUser(reqLogger logr.Logger, client *iam.Client, user *postgresqlv1alpha1.PostgreSQLUser) error {

	err := r.RemoveIAMUser(client, r.AWSLoginRoles, user.Spec.Name)
	if err != nil {
		return err
	}

	reqLogger.Info("Successfully finalized PostgreSQLUser")

	return nil
}

// sanitizedUser removes characters that are not valid in PostgreSQL role names.
func sanitizedUser(raw *postgresqlv1alpha1.PostgreSQLUser) *postgresqlv1alpha1.PostgreSQLUser {
	sanitizedUser := raw.DeepCopy()

	// remove characters not allowed in PostgreSQL roles
	sanitizedUser.Spec.Name = strings.ReplaceAll(sanitizedUser.Spec.Name, ".", "_")

	return sanitizedUser
}

func requeueResult() ctrl.Result {
	return ctrl.Result{Requeue: true}
}
