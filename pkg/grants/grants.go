package grants

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.uber.org/multierr"
)

type Granter struct {
	AllDatabasesReadEnabled  bool
	AllDatabasesWriteEnabled bool
	AllDatabases             func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error)
	AllUsers                 func(namespace string) ([]lunarwayv1alpha1.PostgreSQLUser, error)
	ResourceResolver         func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error)

	StaticRoles     []string
	HostCredentials map[string]postgres.Credentials
	Now             func() time.Time
}

// HostAccess represents a map of read and write access requests on host names
// including the database path.
type HostAccess map[string][]ReadWriteAccess

type ReadWriteAccess struct {
	Host     string
	Database postgres.DatabaseSchema
	Access   lunarwayv1alpha1.AccessSpec
}

func (g *Granter) groupAccesses(log logr.Logger, namespace string, reads []lunarwayv1alpha1.AccessSpec, writes []lunarwayv1alpha1.AccessSpec) (HostAccess, error) {
	if len(reads) == 0 {
		return nil, nil
	}
	hosts := make(HostAccess)
	var errs error

	err := g.groupByHosts(log, hosts, namespace, reads, postgres.PrivilegeRead, g.AllDatabasesReadEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}
	err = g.groupByHosts(log, hosts, namespace, writes, postgres.PrivilegeWrite, g.AllDatabasesWriteEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	if len(hosts) == 0 {
		return nil, errs
	}
	return hosts, errs
}

func (g *Granter) groupByHosts(log logr.Logger, hosts HostAccess, namespace string, accesses []lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege, allDatabasesEnabled bool) error {
	var errs error
	for i, access := range accesses {
		reqLogger := log.WithValues("spec", access, "privilege", privilege)
		// access it not requested to be granted yet
		if g.Now().Before(access.Start.Time) {
			reqLogger.Info("Skipping access spec: start time is in the future")
			continue
		}
		// access request has expired
		if !access.Stop.IsZero() && g.Now().After(access.Stop.Time) {
			reqLogger.Info("Skipping access spec: stop time is in the past")
			continue
		}
		host, err := g.ResourceResolver(access.Host, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve host: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		reqLogger = reqLogger.WithValues("host", host)
		if access.AllDatabases {
			if !allDatabasesEnabled {
				reqLogger.Info("Skipping access spec: allDatabases feature not enabled")
				continue
			}
			reqLogger.Info("Grouping access for all databases on host")
			err := g.groupAllDatabasesByHost(reqLogger, hosts, host, namespace, access, privilege)
			if err != nil {
				errs = multierr.Append(errs, fmt.Errorf("all databases: %w", &AccessError{
					Access: accesses[i],
					Err:    err,
				}))
			}
			continue
		}
		database, err := g.ResourceResolver(access.Database, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve database: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		schema, err := g.ResourceResolver(access.Schema, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve schema: %w", &AccessError{
				Access: accesses[i],
				Err:    err,
			}))
			continue
		}
		hosts[host] = append(hosts[host], ReadWriteAccess{
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
func (g *Granter) groupAllDatabasesByHost(reqLogger logr.Logger, hosts HostAccess, host string, namespace string, access lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege) error {
	databases, err := g.AllDatabases(namespace)
	if err != nil {
		return fmt.Errorf("get all databases: %w", err)
	}
	if len(databases) == 0 {
		reqLogger.WithValues("spec", access, "privilege", privilege, "host", host, "namespace", namespace).Info(fmt.Sprintf("Flag allDatabases results in no privileges granted: no PostgreSQLDatabase resources found on host '%s'", host))
		return nil
	}
	reqLogger.Info(fmt.Sprintf("Found %d PostgreSQLDatabase resources in namespace '%s'", len(databases), namespace))
	var errs error
	for _, databaseResource := range databases {
		database := databaseResource.Spec.Name
		databaseHost, err := g.ResourceResolver(databaseResource.Spec.Host, namespace)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("resolve database '%s' host name: %w", databaseResource.Spec.Name, err))
			continue
		}
		if host != databaseHost {
			reqLogger.Info(fmt.Sprintf("Skipping resource '%s' as it is on another host (%s)", database, databaseHost))
			continue
		}
		schema, err := g.ResourceResolver(databaseResource.Spec.User, namespace)
		if err != nil && !errors.Is(err, kube.ErrNoValue) {
			errs = multierr.Append(errs, fmt.Errorf("resolve database '%s' user name: %w", databaseResource.Spec.Name, err))
			continue
		}
		if schema == "" {
			schema = database
		}
		reqLogger.Info(fmt.Sprintf("Resolved database '%s' with schema '%s'", database, schema))
		hosts[host] = append(hosts[host], ReadWriteAccess{
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
	return fmt.Sprintf("access to host %s: %v", host, err.Err)
}

func (err *AccessError) Unwrap() error {
	return err.Err
}
