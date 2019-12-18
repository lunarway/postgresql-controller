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
	Password string
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
		Name: pair[0],
	}
	if len(pair) == 2 {
		c.Password = pair[1]
	}
	return c, nil
}

func Database(log logr.Logger, db *sql.DB, host string, credentials Credentials) error {
	// Create the service user
	err := createUser(log, db, credentials.Name, credentials.Password)
	if err != nil {
		return fmt.Errorf("create service user: %w", err)
	}
	var (
		readRole      = fmt.Sprintf("%s_read", credentials.Name)
		readWriteRole = fmt.Sprintf("%s_readwrite", credentials.Name)
	)

	// Create read and readwrite roles that can be used to grant users access to
	// the objects in this database.
	err = createRoles(log, db, readRole, readWriteRole)
	if err != nil {
		return fmt.Errorf("create service read and readwrite roles: %w", err)
	}

	// Create the database
	err = createDatabase(log, db, credentials.Name)
	if err != nil {
		return fmt.Errorf("create service database '%s': %w", credentials.Name, err)
	}

	// Alter ownership of the database to the database user. The current user
	// needs to belong to the new role before owner ship can be changed.
	err = execf(db, "GRANT %s TO CURRENT_USER", credentials.Name)
	if err != nil {
		return fmt.Errorf("grant new role '%s' to creator role: %w", credentials.Name, err)
	}
	err = execf(db, "ALTER DATABASE %s OWNER TO %s", credentials.Name, credentials.Name)
	if err != nil {
		return fmt.Errorf("alter owner of database %s: %w", credentials.Name, err)
	}
	err = execf(db, "REVOKE %s FROM CURRENT_USER", credentials.Name)
	if err != nil {
		return fmt.Errorf("revoke new role '%s' to creator role: %w", credentials.Name, err)
	}

	// Connect with the newly created role to create the schema with that role.
	// This ensures that the object is in fact owned by the service and not the
	// creator role.
	serviceConnection, err := Connect(log, ConnectionString{
		Host:     host,
		Database: credentials.Name,
		User:     credentials.Name,
		Password: credentials.Password,
	})
	if err != nil {
		return fmt.Errorf("connect with new user %s: %w", credentials.Name, err)
	}

	// Create schema in the database
	err = createSchema(log, serviceConnection, credentials.Name)
	if err != nil {
		return fmt.Errorf("create schema '%s' as service user '%[1]s': %w", credentials.Name, err)
	}

	// set default read and write priviledges on the read and readwrite roles as
	// to ensure the roles' priviledges apply to all objects created later on.
	err = setReadPriviledges(serviceConnection, credentials.Name, readRole)
	if err != nil {
		return fmt.Errorf("set default read priviledges for role %s: %w", readRole, err)
	}
	err = setReadWritePriviledges(serviceConnection, credentials.Name, readWriteRole)
	if err != nil {
		return fmt.Errorf("set default readwrite priviledges for role %s: %w", readWriteRole, err)
	}

	// This revokation ensures that the user cannot create any objects in the
	// PUBLIC role that is assigned to all roles by default.
	log.Info(fmt.Sprintf("Revoke ALL on role PUBLIC for database '%s'", credentials.Name))
	err = execf(serviceConnection, `
		REVOKE ALL ON DATABASE %s from PUBLIC;
		REVOKE ALL ON SCHEMA public from PUBLIC;
		REVOKE ALL ON ALL TABLES IN SCHEMA public from PUBLIC;`, credentials.Name)
	if err != nil {
		return fmt.Errorf("revoke all for role PUBLIC on database '%s': %w", credentials.Name, err)
	}
	// Grant CONNECT privileges to PUBLIC again to ensure new roles are allowed to connect.
	log.Info("Grant CONNECT to PUBLIC")
	err = execf(serviceConnection, "GRANT CONNECT ON DATABASE %s TO PUBLIC", credentials.Name)
	if err != nil {
		return fmt.Errorf("grant connect to database '%s' to PUBLIC: %w", credentials.Name, err)
	}
	log.Info("Grant usage on schema to PUBLIC")
	err = execf(serviceConnection, "GRANT USAGE ON SCHEMA %s TO PUBLIC", credentials.Name)
	if err != nil {
		return fmt.Errorf("grant connect to database '%s' to PUBLIC: %w", credentials.Name, err)
	}
	return nil
}

func createUser(log logr.Logger, db *sql.DB, name, password string) error {
	log = log.WithValues("database", name)
	return idempotentExec(log, db, idempotentExecReq{
		objectType: "service user",
		errorCode:  "duplicate_object",
		query:      fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'", name, password),
	})
}

func createDatabase(log logr.Logger, db *sql.DB, name string) error {
	log = log.WithValues("database", name)
	return idempotentExec(log, db, idempotentExecReq{
		objectType: "database",
		errorCode:  "duplicate_database",
		query:      fmt.Sprintf("CREATE DATABASE %s", name),
	})
}

func createSchema(log logr.Logger, db *sql.DB, name string) error {
	log = log.WithValues("schema", name)
	return idempotentExec(log, db, idempotentExecReq{
		objectType: "schema",
		errorCode:  "duplicate_schema",
		query:      fmt.Sprintf("CREATE SCHEMA %s", name),
	})
}

func createRoles(log logr.Logger, db *sql.DB, roles ...string) error {
	var errs error
	for _, role := range roles {
		log := log.WithValues("role", role)
		err := idempotentExec(log, db, idempotentExecReq{
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

type idempotentExecReq struct {
	objectType string
	errorCode  string
	query      string
}

func idempotentExec(log logr.Logger, db *sql.DB, args idempotentExecReq) error {
	_, err := db.Exec(args.query)
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != args.errorCode {
			return err
		}
		log.Info(fmt.Sprintf("%s already exists", args.objectType), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("%s created", args.objectType))
	}
	return nil
}

func setReadPriviledges(db *sql.DB, schema string, role string) error {
	return setDefaultPriviledges(db, schema, role, "SELECT")
}

func setReadWritePriviledges(db *sql.DB, schema string, role string) error {
	return setDefaultPriviledges(db, schema, role, "SELECT, INSERT, UPDATE, DELETE")
}

func setDefaultPriviledges(db *sql.DB, schema, role, priviledges string) error {
	// ensures access to future schemas and tables
	err := execf(db, "ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT %s ON TABLES TO %s;", schema, priviledges, role)
	if err != nil {
		return fmt.Errorf("alter default privileges of schema: %w", err)
	}
	err = execf(db, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT %s ON TABLES TO %s;", priviledges, role)
	if err != nil {
		return fmt.Errorf("alter default privileges of public schema: %w", err)
	}
	// ensures access to existing schemas and tables
	err = execf(db, fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", schema, role))
	if err != nil {
		return fmt.Errorf("grant %s privileges on existing schema: %w", priviledges, err)
	}
	err = execf(db, fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", priviledges, schema, role))
	if err != nil {
		return fmt.Errorf("grant %s privileges on existing tables: %w", priviledges, err)
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
