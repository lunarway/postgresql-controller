package grants

import (
	"fmt"

	"github.com/go-logr/logr"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.uber.org/multierr"
)

type Granter struct {
	AllDatabasesReadEnabled  bool
	AllDatabasesWriteEnabled bool
	AllDatabases             func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error)
	ResourceResolver         func(resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error)
}

// HostAccess represents a map of read and write access requests on host names
// including the database path.
type HostAccess map[string][]ReadWriteAccess

type ReadWriteAccess struct {
	Host     string
	Database postgres.DatabaseSchema
	Access   lunarwayv1alpha1.AccessSpec
}

func (g *Granter) GroupAccesses(reqLogger logr.Logger, namespace string, reads []lunarwayv1alpha1.AccessSpec, writes []lunarwayv1alpha1.AccessSpec) (HostAccess, error) {
	if len(reads) == 0 {
		return nil, nil
	}
	hosts := make(HostAccess)
	var errs error

	err := g.groupByHosts(reqLogger, hosts, namespace, reads, postgres.PrivilegeRead, g.AllDatabasesReadEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}
	err = g.groupByHosts(reqLogger, hosts, namespace, writes, postgres.PrivilegeWrite, g.AllDatabasesWriteEnabled)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	if len(hosts) == 0 {
		return nil, errs
	}
	return hosts, errs
}

func (g *Granter) groupByHosts(reqLogger logr.Logger, hosts HostAccess, namespace string, accesses []lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege, allDatabasesEnabled bool) error {
	var errs error
	for i, access := range accesses {
		host, err := g.ResourceResolver(access.Host, namespace)
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
			err := g.groupAllDatabasesByHost(hosts, host, namespace, access, privilege)
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
func (g *Granter) groupAllDatabasesByHost(hosts HostAccess, host string, namespace string, access lunarwayv1alpha1.AccessSpec, privilege postgres.Privilege) error {
	databases, err := g.AllDatabases(namespace)
	if err != nil {
		return fmt.Errorf("get all databases: %w", err)
	}
	var errs error
	for _, databaseResource := range databases {
		database := databaseResource.Spec.Name
		// this limits the `allDatabases` field to only work grant access in a
		// schema named after the database
		schema := databaseResource.Spec.Name
		databaseHost, err := g.ResourceResolver(databaseResource.Spec.Host, namespace)
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
	return fmt.Sprintf("access to host %s: %v", host, err.Err)
}

func (err *AccessError) Unwrap() error {
	return err.Err
}
