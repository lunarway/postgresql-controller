package grants

import (
	"database/sql"
	"fmt"
	"strings"

	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.uber.org/multierr"
)

// Sync syncronizes PostgreSQL role grants for current users and their
// capability mappings.
//
// It collects all users and resolves the required grants for each user. The
// updates the stored grants in the database instance if there are any changes
// required.
// func (g *Granter) Sync(namespace string) error {
// 	// Get all users
// 	users, err := g.AllUsers(namespace)
// 	if err != nil {
// 		return err
// 	}

// 	// get all grants for users in database controlled by the controller

// 	var errs error
// 	for _, user := range users {
// 		err := g.SyncUser(namespace, user)
// 		if err != nil {
// 			errs = multierr.Append(errs, fmt.Errorf("user %s: %w", user.Name, err))
// 		}
// 	}
// 	if errs != nil {
// 		return errs
// 	}
// 	return nil
// }

func (g *Granter) SyncUser(namespace string, user lunarwayv1alpha1.PostgreSQLUser) error {
	//   resolve required grants taking expiration into account
	//   diff against existing
	//   revoke/grant what is needed
	accesses, err := g.groupAccesses(namespace, user.Spec.Read, user.Spec.Write)
	if err != nil {
		if len(accesses) == 0 {
			return fmt.Errorf("group accesses: %w", err)
		}
		g.Log.Error(err, fmt.Sprintf("Some access requests could not be resolved. Continuating with the resolved ones"))
	}
	g.Log.Info(fmt.Sprintf("Found access requests for %d hosts", len(accesses)))

	hosts, err := g.connectToHosts(accesses)
	if err != nil {
		return fmt.Errorf("connect to hosts: %w", err)
	}
	defer func() {
		err := closeConnectionToHosts(hosts)
		if err != nil {
			g.Log.Error(err, "failed to close connection to hosts")
		}
	}()

	err = g.setRolesOnHosts(user.Name, accesses, hosts)
	if err != nil {
		return fmt.Errorf("grant access on host: %w", err)
	}

	return nil
}

func (g *Granter) connectToHosts(accesses HostAccess) (map[string]*sql.DB, error) {
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
		db, err := postgres.Connect(g.Log, connectionString)
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

func (g *Granter) setRolesOnHosts(name string, accesses HostAccess, hosts map[string]*sql.DB) error {
	var errs error
	for host, access := range accesses {
		connection, ok := hosts[host]
		if !ok {
			return fmt.Errorf("connection for host %s not found", host)
		}
		err := postgres.GrantRoles(g.Log, connection, name, g.StaticRoles, databaseSchemas(access))
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
