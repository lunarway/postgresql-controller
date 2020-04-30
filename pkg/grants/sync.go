package grants

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.uber.org/multierr"
)

// SyncUser syncronizes a PostgreSQL user's access requests against the roles
// defined in the host instances. Any excessive roles are removed and missing
// ones are added.
func (g *Granter) SyncUser(log logr.Logger, namespace string, user lunarwayv1alpha1.PostgreSQLUser) error {
	//   resolve required grants taking expiration into account
	//   diff against existing
	//   revoke/grant what is needed
	accesses, err := g.groupAccesses(log, namespace, user.Spec.Read, user.Spec.Write)
	if err != nil {
		if len(accesses) == 0 {
			return fmt.Errorf("group accesses: %w", err)
		}
		log.Error(err, fmt.Sprintf("Some access requests could not be resolved. Continuating with the resolved ones"))
	}
	log.Info(fmt.Sprintf("Found access requests for %d hosts", len(accesses)))

	hosts, err := g.connectToHosts(log, accesses)
	if err != nil {
		return fmt.Errorf("connect to hosts: %w", err)
	}
	defer func() {
		err := closeConnectionToHosts(hosts)
		if err != nil {
			log.Error(err, "failed to close connection to hosts")
		}
	}()

	err = g.setRolesOnHosts(log, user.Name, accesses, hosts)
	if err != nil {
		return fmt.Errorf("grant access on host: %w", err)
	}

	return nil
}

func (g *Granter) connectToHosts(log logr.Logger, accesses HostAccess) (map[string]*sql.DB, error) {
	hosts := make(map[string]*sql.DB)
	var errs error
	for hostDatabase := range accesses {
		// hostDatabase contains the host name and the database but we expect host
		// credentials to be without the database part
		// This will not work for hosts with multiple / characters
		hostDatabaseParts := strings.Split(hostDatabase, "/")
		host := hostDatabaseParts[0]
		database := hostDatabaseParts[1]
		credentials, ok := g.HostCredentials[host]
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

func closeConnectionToHosts(hosts map[string]*sql.DB) error {
	var errs error
	for name, conn := range hosts {
		err := conn.Close()
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("host %s: %w", name, err))
		}
	}
	return errs
}

func (g *Granter) setRolesOnHosts(log logr.Logger, name string, accesses HostAccess, hosts map[string]*sql.DB) error {
	var errs error
	for host, access := range accesses {
		connection, ok := hosts[host]
		if !ok {
			return fmt.Errorf("connection for host %s not found", host)
		}
		err := postgres.Role(log, connection, name, g.StaticRoles, databaseSchemas(access))
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("grant roles: %w", err))
		}
	}
	if errs != nil {
		return errs
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
