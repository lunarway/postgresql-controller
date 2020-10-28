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
	if c.Password == "" {
		return fmt.Errorf("password is empty")
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
		Name: pair[0],
	}
	if len(pair) == 2 {
		c.Password = pair[1]
	}
	return c, nil
}

// Database ensures that a user with provided password exists on the host and
// that read and readwrite roles are created with default priviledges on a
// schema named after the database name.
func Database(log logr.Logger, db *sql.DB, host string, credentials Credentials) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	err := credentials.Validate()
	if err != nil {
		return fmt.Errorf("credentials not valid: %w", err)
	}

	// Create the service user
	err = createUser(log, db, credentials.User, credentials.Password)
	if err != nil {
		return fmt.Errorf("create service user: %w", err)
	}
	var (
		readRole            = fmt.Sprintf("%s_%s", credentials.User, roleSuffixRead)
		readWriteRole       = fmt.Sprintf("%s_%s", credentials.User, roleSuffixWrite)
		readOwningWriteRole = fmt.Sprintf("%s_%s", credentials.User, roleSuffixOwningWrite)
	)

	// if the database is shared we need to grant the existing database role to
	// the user to allow it to create schemas etc. this is a terrible hack to
	// support services using a shared database with mixed owners of the resources.
	if credentials.Shared {
		// ensures access to existing schemas and tables
		err = execf(db, fmt.Sprintf("GRANT %s TO %s", credentials.Name, credentials.User))
		if err != nil {
			return fmt.Errorf("grant %s to service user %s: %w", credentials.Name, credentials.User, err)
		}
	}

	// Create read and readwrite roles that can be used to grant users access to
	// the objects in this database.
	err = createRoles(log, db, readRole, readWriteRole, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("create service read, readwrite and readowningwrite roles: %w", err)
	}

	// Create the database
	err = createDatabase(log, db, credentials.Name)
	if err != nil {
		return fmt.Errorf("create service database '%s': %w", credentials.Name, err)
	}

	// if the database is shared we cannot grant the service user ownership of the
	// database as that would break the actual owners rights.
	if !credentials.Shared {
		// Alter ownership of the database to the database user. The current user
		// needs to belong to the new role before owner ship can be changed.
		err = execf(db, "GRANT %s TO CURRENT_USER", credentials.User)
		if err != nil {
			return fmt.Errorf("grant new role '%s' to creator role: %w", credentials.User, err)
		}
		err = execf(db, "ALTER DATABASE %s OWNER TO %s", credentials.Name, credentials.User)
		if err != nil {
			return fmt.Errorf("alter owner of database %s to %s: %w", credentials.Name, credentials.User, err)
		}
		err = execf(db, "REVOKE %s FROM CURRENT_USER", credentials.User)
		if err != nil {
			return fmt.Errorf("revoke new role '%s' to creator role: %w", credentials.User, err)
		}
	}

	// Connect with the newly created role to create the schema with that role.
	// This ensures that the object is in fact owned by the service and not the
	// creator role.
	serviceConnection, err := Connect(log, ConnectionString{
		Host:     host,
		Database: credentials.Name,
		User:     credentials.User,
		Password: credentials.Password,
	})
	if err != nil {
		return fmt.Errorf("connect with new user %s: %w", credentials.Name, err)
	}
	defer func() {
		err := serviceConnection.Close()
		if err != nil {
			log.Error(err, "failed to close service database connection", "host", host, "database", credentials.Name, "user", credentials.Name)
		}
	}()

	// Create schema in the database
	err = createSchema(log, serviceConnection, credentials.User)
	if err != nil {
		return fmt.Errorf("create schema '%s' as service user '%[1]s': %w", credentials.User, err)
	}

	// set default read and write priviledges on the read and readwrite roles as
	// to ensure the roles' priviledges apply to all objects created later on.
	err = setReadPriviledges(serviceConnection, credentials.User, readRole)
	if err != nil {
		return fmt.Errorf("set default read priviledges for role %s: %w", readRole, err)
	}
	err = setReadWritePriviledges(serviceConnection, credentials.User, readWriteRole)
	if err != nil {
		return fmt.Errorf("set default readwrite priviledges for role %s: %w", readWriteRole, err)
	}

	// an owning write request makes it possible to do everything a read and
	// readwrite role can along with being granted the owner role to allow DROP
	// and ALTER as well
	err = setReadWritePriviledges(serviceConnection, credentials.User, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("set default readowningwrite priviledges for role %s: %w", readOwningWriteRole, err)
	}
	err = execf(serviceConnection, "GRANT %s TO %s", credentials.User, readOwningWriteRole)
	if err != nil {
		return fmt.Errorf("grant owner %s to readowningwrite for role %s: %w", credentials.User, readOwningWriteRole, err)
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
	log.Info(fmt.Sprintf("Grant usage on schema '%s' to PUBLIC", credentials.User))
	err = execf(serviceConnection, "GRANT USAGE ON SCHEMA %s TO PUBLIC", credentials.User)
	if err != nil {
		return fmt.Errorf("grant usage on schema '%s' to PUBLIC: %w", credentials.User, err)
	}
	return nil
}

func createUser(log logr.Logger, db *sql.DB, user, password string) error {
	log = log.WithValues("user", user)
	return idempotentExec(log, db, idempotentExecReq{
		objectType: "service user",
		errorCode:  "duplicate_object",
		query:      fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'", user, password),
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
