package postgresqluser

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.uber.org/multierr"
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
	c.hostCredentials, err = parseHostCredentials(hosts)
	parseError(err, "host-credentials: invalid format")
	var hostNames []string
	for host := range c.hostCredentials {
		hostNames = append(hostNames, host)
	}
	c.allDatabasesReadEnabled, err = FlagSet.GetBool("all-databases-enabled-read")
	parseError(err, "all-databases-enabled-read")
	c.allDatabasesWriteEnabled, err = FlagSet.GetBool("all-databases-enabled-write")
	parseError(err, "all-databases-enabled-write")

	log.Info("Controller configured",
		"hosts", hostNames,
		"roles", c.grantRoles,
		"prefix", c.rolePrefix,
		"awsPolicyName", c.awsPolicyName,
		"awsRegion", c.awsRegion,
		"awsAccountID", c.awsAccountID,
		"allDatabasesReadEnabled", c.allDatabasesReadEnabled,
		"allDatabasesWriteEnabled", c.allDatabasesWriteEnabled,
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
		client:           mgr.GetClient(),
		resourceResolver: kube.ResourceValue,
		allDatabases:     kube.PostgreSQLDatabases,
		setAWSPolicy:     iam.SetAWSPolicy,
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
	client           client.Client
	resourceResolver func(client client.Client, resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error)
	allDatabases     func(client client.Client, namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error)
	setAWSPolicy     func(log logr.Logger, credentials *credentials.Credentials, policy iam.AWSPolicy, userID string) error

	grantRoles               []string
	rolePrefix               string
	awsPolicyName            string
	awsRegion                string
	awsAccountID             string
	awsProfile               string
	awsAccessKeyID           string
	awsSecretAccessKey       string
	allDatabasesReadEnabled  bool
	allDatabasesWriteEnabled bool

	// contains a map of credentials for hosts
	hostCredentials map[string]postgres.Credentials
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
	reqLogger.Info("Reconciling PostgreSQLUSer")
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
	reqLogger = reqLogger.WithValues("user", user.Spec.Name)

	reqLogger.Info("Reconciling PostgreSQLUser", "user", user.Spec.Name)
	accesses, err := r.groupAccesses(reqLogger, request.Namespace, user.Spec.Read, user.Spec.Write)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("group accesses: %w", err)
	}

	hosts, err := r.connectToHosts(accesses)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("connect to hosts: %w", err)
	}

	err = r.ensurePostgreSQLRoles(reqLogger, user.Spec.Name, accesses, hosts)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure postgresql roles: %w", err)
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

func (r *ReconcilePostgreSQLUser) ensurePostgreSQLRoles(log logr.Logger, name string, accesses HostAccess, hosts map[string]*sql.DB) error {
	for host, access := range accesses {
		connection, ok := hosts[host]
		if !ok {
			return fmt.Errorf("connection for host %s not found", host)
		}
		err := r.ensurePostgreSQLRole(log, connection, name, access)
		if err != nil {
			return err
		}
	}
	return nil
}

func databaseSchemas(accesses []ReadWriteAccess) []postgres.DatabaseSchema {
	var ds []postgres.DatabaseSchema
	for _, access := range accesses {
		ds = append(ds, postgres.DatabaseSchema{
			Name:       access.Database.Name,
			Schema:     access.Database.Schema,
			Privileges: access.Database.Privileges,
		})
	}
	return ds
}

func (r *ReconcilePostgreSQLUser) ensurePostgreSQLRole(log logr.Logger, db *sql.DB, name string, accesses []ReadWriteAccess) error {
	name = fmt.Sprintf("%s%s", r.rolePrefix, name)
	log = log.WithValues("operation", "ensurePostgreSQLRole", "name", name)
	err := postgres.Role(log, db, name, r.grantRoles, databaseSchemas(accesses))
	if err != nil {
		return err
	}
	return nil
}

// HostAccess represents a map of read and write access requests on host names
// including the database path.
type HostAccess map[string][]ReadWriteAccess

type ReadWriteAccess struct {
	Host     string
	Database postgres.DatabaseSchema
	Access   lunarwayv1alpha1.AccessSpec
}

func (r *ReconcilePostgreSQLUser) connectToHosts(accesses HostAccess) (map[string]*sql.DB, error) {
	hosts := make(map[string]*sql.DB)
	var errs error
	for hostDatabase := range accesses {
		// hostDatabase contains the host name and the database but we expect host
		// credentials to be without the database part
		// This will not work for hosts with multiple / characters
		hostDatabaseParts := strings.Split(hostDatabase, "/")
		host := hostDatabaseParts[0]
		database := hostDatabaseParts[1]
		credentials, ok := r.hostCredentials[host]
		if !ok {
			errs = multierr.Append(errs, fmt.Errorf("no credentials for host '%s'", host))
			continue
		}
		connectionString := postgres.ConnectionString{
			Host:     host,
			Database: database,
			User:     credentials.Name,
			Password: credentials.Password,
		}
		db, err := postgres.Connect(log, connectionString)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("connect to %s: %w", connectionString, err))
			continue
		}
		hosts[hostDatabase] = db
	}
	return hosts, errs
}

func (r *ReconcilePostgreSQLUser) groupAccesses(reqLogger logr.Logger, namespace string, reads []lunarwayv1alpha1.AccessSpec, writes []lunarwayv1alpha1.AccessSpec) (HostAccess, error) {
	if len(reads) == 0 {
		return nil, nil
	}
	hosts := make(HostAccess)
	var errs error

	err := r.groupByHosts(reqLogger, hosts, namespace, reads, postgres.PrivilegeRead, r.allDatabasesReadEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}
	err = r.groupByHosts(reqLogger, hosts, namespace, writes, postgres.PrivilegeWrite, r.allDatabasesWriteEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	if len(hosts) == 0 {
		return nil, errs
	}
	return hosts, errs
}

func (r *ReconcilePostgreSQLUser) groupByHosts(reqLogger logr.Logger, hosts HostAccess, namespace string, accesses []lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege, allDatabasesEnabled bool) error {
	var errs error
	for i, access := range accesses {
		host, err := r.resourceResolver(r.client, access.Host, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve host: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		if access.AllDatabases {
			if !allDatabasesEnabled {
				reqLogger.WithValues("spec", access, "privilege", privilege).Info("Skipping access spec: allDatabases feature not enabled")
				continue
			}
			err := r.groupAllDatabasesByHost(hosts, host, namespace, access, privilege)
			if err != nil {
				errs = multierr.Append(errs, fmt.Errorf("all databases: %w", &AccessError{
					Access: accesses[i],
					Err:    err,
				}))
			}
			continue
		}
		database, err := r.resourceResolver(r.client, access.Database, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve database: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		schema, err := r.resourceResolver(r.client, access.Schema, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve schema: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		hostDatabase := fmt.Sprintf("%s/%s", host, database)
		hosts[hostDatabase] = append(hosts[hostDatabase], ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     schema,
				Privileges: privilege,
			},
			Access: accesses[i],
		})
	}
	return errs
}

// groupAllDatabasesByHost groups read write accesses for all known databases in the hosts access map.
func (r *ReconcilePostgreSQLUser) groupAllDatabasesByHost(hosts HostAccess, host string, namespace string, access lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege) error {
	databases, err := r.allDatabases(r.client, namespace)
	if err != nil {
		return fmt.Errorf("get all databases: %w", err)
	}
	var errs error
	for _, databaseResource := range databases {
		database := databaseResource.Spec.Name
		schema := databaseResource.Spec.Name // FIXME: This will not work when schemas differ from database names
		databaseHost, err := r.resourceResolver(r.client, databaseResource.Spec.Host, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve database '%s' host name: %w", databaseResource.Spec.Name, err))
			continue
		}
		if host != databaseHost {
			continue
		}
		hostKey := fmt.Sprintf("%s/%s", host, database)
		hosts[hostKey] = append(hosts[hostKey], ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     schema,
				Privileges: privilege,
			},
			Access: access,
		})
	}
	if errs != nil {
		return errs
	}
	return nil
}

type AccessError struct {
	Access lunarwayv1alpha1.AccessSpec
	Err    error
}

var _ error = &AccessError{}

func (err *AccessError) Error() string {
	host := err.Access.Host.Value
	if host == "" && err.Access.Host.ValueFrom.SecretKeyRef != nil {
		host = fmt.Sprintf("from secret '%s' key '%s'", err.Access.Host.ValueFrom.SecretKeyRef.Name, err.Access.Host.ValueFrom.SecretKeyRef.Key)
	}
	if host == "" && err.Access.Host.ValueFrom.ConfigMapKeyRef != nil {
		host = fmt.Sprintf("from config map '%s' key '%s'", err.Access.Host.ValueFrom.ConfigMapKeyRef.Name, err.Access.Host.ValueFrom.ConfigMapKeyRef.Key)
	}
	return fmt.Sprintf("access to host %s: %v: access data: %+v", host, err.Err, err.Access)
}

func (err *AccessError) Unwrap() error {
	return err.Err
}
