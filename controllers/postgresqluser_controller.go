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

	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	Granter      grants.Granter
	SetAWSPolicy func(log logr.Logger, credentials *credentials.Credentials, policy iam.AddUserConfig, userID string) error

	RolePrefix         string
	AWSPolicyName      string
	AWSRegion          string
	AWSAccountID       string
	AWSProfile         string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
}

// +kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.lunar.tech,resources=postgresqlusers/status,verbs=get;update;patch

func (r *PostgreSQLUserReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	reqLogger.Info("Reconciling PostgreSQLUSer")

	result, err := r.reconcile(reqLogger, req)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLUser object")
	}
	return result, err
}

func (r *PostgreSQLUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresqlv1alpha1.PostgreSQLUser{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). //explicitly set to 1 (which is also the default) because our reconciliation process is not necessarily concurrency safe.
		Complete(r)
}

func (r *PostgreSQLUserReconciler) reconcile(reqLogger logr.Logger, request reconcile.Request) (ctrl.Result, error) {
	// Fetch the PostgreSQLUser instance
	user := &postgresqlv1alpha1.PostgreSQLUser{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, user)
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
	reqLogger.Info("Reconciling found PostgreSQLUser resource", "user", user.Spec.Name)

	// Error check in the bottom because we want aws policy to be set no matter what.
	granterErr := r.Granter.SyncUser(reqLogger, request.Namespace, r.RolePrefix, *user)

	var awsCredentials *credentials.Credentials
	if len(r.AWSProfile) != 0 {
		awsCredentials = credentials.NewSharedCredentials("", r.AWSProfile)
	} else {
		awsCredentials = credentials.NewStaticCredentials(r.AWSAccessKeyID, r.AWSSecretAccessKey, "")
	}

	awsPolicyErr := r.SetAWSPolicy(reqLogger, awsCredentials, iam.AddUserConfig{
		PolicyBaseName:    r.AWSPolicyName,
		Region:            r.AWSRegion,
		AccountID:         r.AWSAccountID,
		MaxUsersPerPolicy: 30,
		IamPrefix:         "/lunar-postgresql-user/",
		RolePrefix:        r.RolePrefix,
	}, user.Spec.Name)

	if granterErr != nil || awsPolicyErr != nil {
		return ctrl.Result{}, fmt.Errorf("grantErr: %v, awsPolicyErr: %v", granterErr, awsPolicyErr)
	}

	return ctrl.Result{}, nil
}
