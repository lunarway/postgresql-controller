package postgresqluser

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/spf13/pflag"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_postgresqluser").WithValues("controller", "postgresqluser-controller")

var FlagSet *pflag.FlagSet

func init() {
	FlagSet = pflag.NewFlagSet("controller_postgresqluser", pflag.ExitOnError)
	FlagSet.StringSlice("user-roles", []string{"rds_iam"}, "Roles granted to all users")
	FlagSet.String("user-role-prefix", "iam_developer_", "Prefix of roles created in PostgreSQL for users")
	FlagSet.String("aws-policy-name", "postgres-controller-users", "AWS Policy name to update IAM statements on")
	FlagSet.String("aws-region", "eu-west-1", "AWS Region where IAM policies are located")
	FlagSet.String("aws-account-id", "660013655494", "AWS Account id where IAM policies are located")
	FlagSet.String("aws-profile", "", "AWS Profile to use for credentials")
	FlagSet.String("aws-access-key-id", "", "AWS access key id to use for credentials")
	FlagSet.String("aws-secret-access-key", "", "AWS secret access key to use for credentials")
	FlagSet.StringToString("host-credentials-user", nil, "Host and credential pairs in the form hostname=user:password. Use comma separated pairs for multiple hosts")
	FlagSet.Bool("all-databases-enabled-read", false, "Enable usage of allDatabases field in read access requests")
	FlagSet.Bool("all-databases-enabled-write", false, "Enable usage of allDatabases field in write access requests")
}

func parseFlags(c *ReconcilePostgreSQLUser) {
	var err error
	c.grantRoles, err = FlagSet.GetStringSlice("user-roles")
	parseError(err, "user-roles")
	c.rolePrefix, err = FlagSet.GetString("user-role-prefix")
	parseError(err, "user-role-prefix")
	c.awsPolicyName, err = FlagSet.GetString("aws-policy-name")
	parseError(err, "aws-policy-name")
	c.awsRegion, err = FlagSet.GetString("aws-region")
	parseError(err, "aws-region")
	c.awsAccountID, err = FlagSet.GetString("aws-account-id")
	parseError(err, "aws-account")
	c.awsProfile, err = FlagSet.GetString("aws-profile")
	parseError(err, "aws-profile")
	c.awsAccessKeyID, err = FlagSet.GetString("aws-access-key-id")
	parseError(err, "aws-access-key-id")
	c.awsSecretAccessKey, err = FlagSet.GetString("aws-secret-access-key")
	parseError(err, "aws-secret-access-key")
	hosts, err := FlagSet.GetStringToString("host-credentials-user")
	parseError(err, "host-credentials-user")
	c.granter.HostCredentials, err = parseHostCredentials(hosts)
	parseError(err, "host-credentials: invalid format")
	var hostNames []string
	for host := range c.granter.HostCredentials {
		hostNames = append(hostNames, host)
	}
	c.granter.AllDatabasesReadEnabled, err = FlagSet.GetBool("all-databases-enabled-read")
	parseError(err, "all-databases-enabled-read")
	c.granter.AllDatabasesWriteEnabled, err = FlagSet.GetBool("all-databases-enabled-write")
	parseError(err, "all-databases-enabled-write")

	log.Info("Controller configured",
		"hosts", hostNames,
		"roles", c.grantRoles,
		"prefix", c.rolePrefix,
		"awsPolicyName", c.awsPolicyName,
		"awsRegion", c.awsRegion,
		"awsAccountID", c.awsAccountID,
		"allDatabasesReadEnabled", c.granter.AllDatabasesReadEnabled,
		"allDatabasesWriteEnabled", c.granter.AllDatabasesWriteEnabled,
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

// Add creates a new PostgreSQLUser Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	c := &ReconcilePostgreSQLUser{
		client: mgr.GetClient(),
		granter: grants.Granter{
			Now: time.Now,
			AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(mgr.GetClient(), namespace)
			},
			ResourceResolver: func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(mgr.GetClient(), resource, namespace)
			},
		},
		setAWSPolicy: iam.SetAWSPolicy,
	}
	parseFlags(c)
	return c
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("postgresqluser-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PostgreSQLUser
	err = c.Watch(&source.Kind{Type: &lunarwayv1alpha1.PostgreSQLUser{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePostgreSQLUser implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePostgreSQLUser{}

// ReconcilePostgreSQLUser reconciles a PostgreSQLUser object
type ReconcilePostgreSQLUser struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client       client.Client
	granter      grants.Granter
	setAWSPolicy func(log logr.Logger, credentials *credentials.Credentials, policy iam.AWSPolicy, userID string) error

	grantRoles         []string
	rolePrefix         string
	awsPolicyName      string
	awsRegion          string
	awsAccountID       string
	awsProfile         string
	awsAccessKeyID     string
	awsSecretAccessKey string
}

// Reconcile reads that state of the cluster for a PostgreSQLUser object and makes changes based on the state read
// and what is in the PostgreSQLUser.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePostgreSQLUser) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	requestID, err := uuid.NewRandom()
	if err != nil {
		reqLogger.Error(err, "Failed to pick a request ID. Continuing without")
	}
	reqLogger = reqLogger.WithValues("requestId", requestID.String())
	reqLogger.Info("Reconciling PostgreSQLUSer")

	result, err := r.reconcile(reqLogger, request)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile PostgreSQLUser object")
	}
	return result, err
}

func (r *ReconcilePostgreSQLUser) reconcile(reqLogger logr.Logger, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the PostgreSQLUser instance
	user := &lunarwayv1alpha1.PostgreSQLUser{}
	err := r.client.Get(context.TODO(), request.NamespacedName, user)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Object not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// User instance created or updated

	// prefix name with configured rolePrefix
	user.Spec.Name = fmt.Sprintf("%s%s", r.rolePrefix, user.Spec.Name)

	reqLogger = reqLogger.WithValues("user", user.Spec.Name, "rolePrefix", r.rolePrefix)
	reqLogger.Info("Reconciling found PostgreSQLUser resource", "user", user.Spec.Name)

	err = r.granter.SyncUser(reqLogger, request.Namespace, *user)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("sync user grants: %w", err)
	}

	var awsCredentials *credentials.Credentials
	if len(r.awsProfile) != 0 {
		awsCredentials = credentials.NewSharedCredentials("", r.awsProfile)
	} else {
		awsCredentials = credentials.NewStaticCredentials(r.awsAccessKeyID, r.awsSecretAccessKey, "")
	}
	err = r.setAWSPolicy(reqLogger, awsCredentials, iam.AWSPolicy{
		Name:      r.awsPolicyName,
		Region:    r.awsRegion,
		AccountID: r.awsAccountID,
	}, user.Spec.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("set aws policy: %w", err)
	}

	return reconcile.Result{}, nil
}
