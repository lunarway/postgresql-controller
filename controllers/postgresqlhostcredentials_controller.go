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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
)

// PostgreSQLHostCredentialsReconciler reconciles a PostgreSQLHostCredentials object
type PostgreSQLHostCredentialsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlhostcredentials,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlhostcredentials/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlhostcredentials/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PostgreSQLHostCredentials object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *PostgreSQLHostCredentialsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to picka  request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	reqLogger.Info("Reconciling PostgreSQLHostCredentials")

	result, err := r.reconcile(ctx, reqLogger, req)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLHostCredentials object")
	}

	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLHostCredentialsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLHostCredentials{}).
		Complete(r)
}

func (r *PostgreSQLHostCredentialsReconciler) reconcile(ctx context.Context, reqLogger logr.Logger, request reconcile.Request) (ctrl.Result, error) {
	creds := postgresqlv1alpha1.PostgreSQLHostCredentials{}
	if err := r.Client.Get(ctx, request.NamespacedName, &creds); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("PostgreSQLHostCredentials not found")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		return ctrl.Result{}, err
	}

	// HostCredentials instance created or updated
	return ctrl.Result{}, nil
}
