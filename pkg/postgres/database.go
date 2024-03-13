package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
	"go.uber.org/multierr"
)

// Credentials represents connection credentials for a user on a
// PostgreSQL instance.
type Credentials struct {
	Name     string
	User     string
	Password string
	Shared   bool
	Params   string
}

func (c Credentials) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is empty")
	}
	if c.User == "" {
		return fmt.Errorf("user is empty")
	}
	return nil
}

// ParseUsernamePassword parses string s as a PostgreSQL user name and password
// pair. If the user name is determined to be empty an error is returned.
func ParseUsernamePassword(s string) (Credentials, error) {
	if len(s) == 0 {
		return Credentials{}, fmt.Errorf("username empty")
	}
	pair := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(pair[0]) == 0 {
		return Credentials{}, fmt.Errorf("username empty")
	}
	c := Credentials{
		User: pair[0],
	}
	if len(pair) == 2 {
		c.Password = pair[1]
	}
	return c, nil
}

// Database ensures that a user with provided password exists on the host and
// that read and readwrite roles are created with default privileges on a
// schema named after the database name.
func Database(log logr.Logger, host string, adminCredentials, serviceCredentials Credentials, managerRole string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if managerRole == "" {
		return fmt.Errorf("managerRole required")
	}
	err := serviceCredentials.Validate()
	if err != nil {
		return fmt.Errorf("serviceCredentials not valid: %w", err)
	}

	// Create the database
	err = createDatabase(log, host, adminCredentials, serviceCredentials.Name)
	if err != nil {
		return fmt.Errorf("create service database '%s': %w", serviceCredentials.Name, err)
	}

	// Connect to the service database
	serviceConnectionString := ConnectionString{
		Host:     host,
		Database: serviceCredentials.Name,
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	serviceConnection, err := Connect(log, serviceConnectionString)
	if err != nil {
		return fmt.Errorf("connect to host %s: %w", serviceConnectionString, err)
	}
	defer func() {
		err := serviceConnection.Close()
		if err != nil {
			log.Error(err, "failed to close database connection", "host", serviceConnectionString.Host, "database", "postgres", "user", serviceConnectionString.User)
		}
	}()

	// Create the service user
	err = createServiceRole(log, serviceConnection, serviceCredentials.User, serviceCredentials.Password)
	if err != nil {
		return fmt.Errorf("create service user: %w", err)
	}

	// if the database is shared we need to grant the existing database role to
	// the user to allow it to create schemas etc. this is a terrible hack to
	// support services using a shared database with mixed owners of the resources.
	if serviceCredentials.Shared {
		// ensures access to existing schemas and tables
		err = execf(serviceConnection, fmt.Sprintf("GRANT %s TO %s", serviceCredentials.Name, serviceCredentials.User))
		if err != nil {
			return fmt.Errorf("grant %s to service user %s: %w", serviceCredentials.Name, serviceCredentials.User, err)
		}
	}

	// Grant the service user role to the managerRole WITH ADMIN OPTION
	// This allows the managerRole to act on behalf of the service user
	err = grantAdminOption(log, serviceConnection, serviceCredentials.Name, managerRole)
	if err != nil {
		return fmt.Errorf("grant %s to management role %s: %w", serviceCredentials.Name, managerRole, err)
	}

	// Create read and readwrite roles that can be used to grant users access to
	// the objects in this database.
	var (
		readRole            = fmt.Sprintf("%s_%s", serviceCredentials.User, roleSuffixRead)
		readWriteRole       = fmt.Sprintf("%s_%s", serviceCredentials.User, roleSuffixWrite)
		readOwningWriteRole = fmt.Sprintf("%s_%s", serviceCredentials.User, roleSuffixOwningWrite)
	)
	err = createRoles(log, serviceConnection, readRole, readWriteRole, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("create service read, readwrite and readowningwrite roles: %w", err)
	}

	// Alter ownership of the database to the database user. The current user
	// needs to belong to the new role before owner ship can be changed.
	err = execf(serviceConnection, "GRANT %s TO CURRENT_USER", serviceCredentials.User)
	if err != nil {
		return fmt.Errorf("grant new role '%s' to creator role: %w", serviceCredentials.User, err)
	}
	defer func() {
		err = execf(serviceConnection, "REVOKE %s FROM CURRENT_USER", serviceCredentials.User)
		if err != nil {
			log.Error(err, fmt.Sprintf("revoke new role '%s' to creator role", serviceCredentials.User))
		}
	}()

	// if the database is shared we cannot grant the service user ownership of the
	// database as that would break the actual owners rights.
	if !serviceCredentials.Shared {
		err = execf(serviceConnection, "ALTER DATABASE %s OWNER TO %s", serviceCredentials.Name, serviceCredentials.User)
		if err != nil {
			return fmt.Errorf("alter owner of database %s to %s: %w", serviceCredentials.Name, serviceCredentials.User, err)
		}
	}

	// Grant the service role (which is owner) to the readowningwrite role
	err = execf(serviceConnection, "GRANT %s TO %s", serviceCredentials.User, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("grant owner %s to readowningwrite for role %s: %w", serviceCredentials.User, readOwningWriteRole, err)
	}

	// Execute the rest of the queries as the service user on the service database

	// Create schema in the database
	err = createSchemaAs(log, serviceConnection, serviceCredentials.User, serviceCredentials.User)
	if err != nil {
		return fmt.Errorf("create schema '%s' as service user '%[1]s': %w", serviceCredentials.User, err)
	}

	// Set default privileges for the service user
	err = setDefaultPrivileges(serviceConnection, serviceCredentials.User, readRole, readWriteRole, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("set default privileges for service user '%s': %w", serviceCredentials.User, err)
	}

	// This revokation ensures that the user cannot create any objects in the
	// PUBLIC role that is assigned to all roles by default.
	err = revokeAllOnPublic(log, serviceConnection, serviceCredentials)
	if err != nil {
		return fmt.Errorf("revoke all on public for service user '%s': %w", serviceCredentials.User, err)
	}

	// Grant CONNECT and USAGE to PUBLIC again to ensure new roles are allowed to connect.
	err = grantConnectAndUsage(log, serviceConnection, serviceCredentials)
	if err != nil {
		return fmt.Errorf("grant connect and usage to public for service user '%s': %w", serviceCredentials.User, err)
	}

	return nil
}

func createServiceRole(log logr.Logger, db *sql.DB, user, password string) error {
	log = log.WithValues("user", user)
	var query string
	if password != "" {
		query = fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'", user, password)
	} else {
		query = fmt.Sprintf("CREATE ROLE %s NOCREATEROLE", user)
	}
	return tryExec(log, db, tryExecReq{
		objectType: "service user",
		errorCode:  "duplicate_object",
		query:      query,
	})
}

func createDatabase(log logr.Logger, host string, adminCredentials Credentials, name string) error {
	log = log.WithValues("database", name)

	connectionString := ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     adminCredentials.User,
		Password: adminCredentials.Password,
		Params:   adminCredentials.Params,
	}
	db, err := Connect(log, connectionString)
	if err != nil {
		return fmt.Errorf("connect to host %s: %w", connectionString, err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Error(err, "failed to close database connection", "host", connectionString.Host, "database", "postgres", "user", connectionString.User)
		}
	}()

	return tryExec(log, db, tryExecReq{
		objectType: "database",
		errorCode:  "duplicate_database",
		query:      fmt.Sprintf("CREATE DATABASE %s", name),
	})
}

func createSchemaAs(log logr.Logger, db *sql.DB, schema, actor string) error {
	log = log.WithValues("schema", schema)
	return tryExec(log, db, tryExecReq{
		objectType: "schema",
		errorCode:  "duplicate_schema",
		query:      prependSetRole(fmt.Sprintf("CREATE SCHEMA %s", schema), actor),
	})
}

func createRoles(log logr.Logger, db *sql.DB, roles ...string) error {
	var errs error
	for _, role := range roles {
		log := log.WithValues("role", role)
		err := tryExec(log, db, tryExecReq{
			objectType: "service role",
			errorCode:  "duplicate_object",
			query:      fmt.Sprintf("CREATE ROLE %s", role),
		})
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("create role %s: %w", role, err))
		}
	}
	if errs != nil {
		return errs
	}
	return nil
}

func grantAdminOption(log logr.Logger, db *sql.DB, serviceRole string, managerRole string) error {
	return tryExec(log, db, tryExecReq{
		objectType: "service role",
		errorCode:  "undefined_object",
		query:      fmt.Sprintf("GRANT %s TO %s WITH ADMIN OPTION", serviceRole, managerRole),
	})
}

func setDefaultPrivileges(serviceConnection *sql.DB, serviceRole, readRole, readWriteRole, readOwningWriteRole string) error {
	err := setReadPrivilegesAs(serviceConnection, serviceRole, readRole, serviceRole)
	if err != nil {
		return fmt.Errorf("set default read privileges for role %s: %w, as %s", readRole, err, serviceRole)
	}

	err = setReadWritePrivilegesAs(serviceConnection, serviceRole, readWriteRole, serviceRole)
	if err != nil {
		return fmt.Errorf("set default readwrite privileges for role %s: %w, as %s", readWriteRole, err, serviceRole)
	}

	err = setReadWritePrivilegesAs(serviceConnection, serviceRole, readOwningWriteRole, serviceRole)
	if err != nil {
		return fmt.Errorf("set default readowningwrite privileges for role %s: %w, as %s", readOwningWriteRole, err, serviceRole)
	}
	return nil
}

func revokeAllOnPublic(log logr.Logger, serviceConnection *sql.DB, serviceCredentials Credentials) error {
	log.Info(fmt.Sprintf("Revoke ALL on role PUBLIC for database '%s'", serviceCredentials.Name))
	err := execAsf(serviceConnection, serviceCredentials.User, `
		REVOKE ALL ON DATABASE %s from PUBLIC;
		REVOKE ALL ON SCHEMA public from PUBLIC;
		REVOKE ALL ON ALL TABLES IN SCHEMA public from PUBLIC;`, serviceCredentials.Name)
	if err != nil {
		return fmt.Errorf("revoke all for role PUBLIC on database '%s': %w, as %s", serviceCredentials.Name, err, serviceCredentials.User)
	}
	return nil
}

func grantConnectAndUsage(log logr.Logger, serviceConnection *sql.DB, serviceCredentials Credentials) error {
	// Grant CONNECT privileges to PUBLIC again to ensure new roles are allowed to connect.
	log.Info("Grant CONNECT to PUBLIC")
	err := execAsf(serviceConnection, serviceCredentials.User, "GRANT CONNECT ON DATABASE %s TO PUBLIC", serviceCredentials.Name)
	if err != nil {
		return fmt.Errorf("grant connect to database '%s' to PUBLIC: %w as %s", serviceCredentials.Name, err, serviceCredentials.User)
	}

	log.Info(fmt.Sprintf("Grant usage on schema '%s' to PUBLIC", serviceCredentials.User))
	err = execAsf(serviceConnection, serviceCredentials.User, "GRANT USAGE ON SCHEMA %s TO PUBLIC", serviceCredentials.User)
	if err != nil {
		return fmt.Errorf("grant usage on schema '%s' to PUBLIC: %w as %s", serviceCredentials.User, err, serviceCredentials.User)
	}

	return nil
}

type tryExecReq struct {
	objectType string
	errorCode  string
	query      string
}

func tryExec(log logr.Logger, db *sql.DB, args tryExecReq) error {
	_, err := db.Exec(args.query)
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != args.errorCode {
			return err
		}
		log.Info(fmt.Sprintf("expected err '%s' occured. Ignoring for objectType '%s'", args.errorCode, args.objectType), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("%s created", args.objectType))
	}
	return nil
}

func setReadPrivilegesAs(db *sql.DB, schema, role, actor string) error {
	return setDefaultPrivilegesAs(db, schema, role, "SELECT", actor)
}

func setReadWritePrivilegesAs(db *sql.DB, schema, role, actor string) error {
	return setDefaultPrivilegesAs(db, schema, role, "SELECT, INSERT, UPDATE, DELETE", actor)
}

func setDefaultPrivilegesAs(db *sql.DB, schema, role, privileges, actor string) error {
	// ensures access to future schemas and tables
	err := execAsf(db, actor, "ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT %s ON TABLES TO %s;", schema, privileges, role)
	if err != nil {
		return fmt.Errorf("alter default privileges of schema: %w, as %s", err, actor)
	}
	err = execAsf(db, actor, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT %s ON TABLES TO %s;", privileges, role)
	if err != nil {
		return fmt.Errorf("alter default privileges of public schema: %w, as %s", err, actor)
	}
	// ensures access to existing schemas and tables
	err = execAsf(db, actor, fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", schema, role))
	if err != nil {
		return fmt.Errorf("grant %s privileges on existing schema: %w, as %s", privileges, err, actor)
	}
	err = execAsf(db, actor, fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", privileges, schema, role))
	if err != nil {
		return fmt.Errorf("grant %s privileges on existing tables: %w, as %s", privileges, err, actor)
	}
	return nil
}

// execf executes a formatted query on db.
func execf(db *sql.DB, query string, args ...interface{}) error {
	_, err := db.Exec(fmt.Sprintf(query, args...))
	if err != nil {
		return err
	}
	return nil
}

// execf executes a formatted query on db as given role.
func execAsf(db *sql.DB, role string, query string, args ...interface{}) error {
	err := execf(db, prependSetRole(query, role), args...)
	if err != nil {
		return fmt.Errorf("unable to execute query '%s', with args %v. %w", prependSetRole(query, role), args, err)
	}
	return nil
}

func prependSetRole(query, role string) string {
	return fmt.Sprintf(`
		SET ROLE %s;
		%s`, role, query)
}
