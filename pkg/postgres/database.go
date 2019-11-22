package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
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
	_, err := db.Exec(fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'", credentials.Name, credentials.Password))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return fmt.Errorf("create user %s: %w", credentials.Name, err)
		}
		log.Info(fmt.Sprintf("Service user; %s already exists", credentials.Name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Service user; %s created", credentials.Name))
	}

	// Create the database
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", credentials.Name))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_database" {
			return fmt.Errorf("create database %s: %w", credentials.Name, err)
		}
		log.Info(fmt.Sprintf("Database; %s already exists", credentials.Name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Database; %s created", credentials.Name))
	}

	// Alter ownership of the database to the database user
	_, err = db.Exec(fmt.Sprintf("ALTER DATABASE %s OWNER TO %s", credentials.Name, credentials.Name))
	if err != nil {
		return fmt.Errorf("alter owner of database %s: %w", credentials.Name, err)
	}

	// Connect with the newly created role to create the schema with that role. This ensures
	// that the object is in fact owned by the service and not the creator role.
	serviceConnection, err := Connect(log, fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=disable", credentials.Name, credentials.Password, host, credentials.Name))
	if err != nil {
		return fmt.Errorf("connect with new user %s: %w", credentials.Name, err)
	}

	// Create schema in the database
	_, err = serviceConnection.Exec(fmt.Sprintf("CREATE SCHEMA %s", credentials.Name))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_schema" {
			return fmt.Errorf("create default schema %s: %w", credentials.Name, err)
		}
		log.Info(fmt.Sprintf("Schema; %s already exists in database; %s", credentials.Name, credentials.Name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Schema; %s created in database; %s", credentials.Name, credentials.Name))
	}
	// This revokation ensures that the user cannot create any objects in the
	// PUBLIC role that is assigned to all roles by default.
	log.Info(fmt.Sprintf("Revoke ALL on role PUBLIC for database '%s'", credentials.Name))
	_, err = serviceConnection.Exec(fmt.Sprintf(`REVOKE ALL ON DATABASE %s from PUBLIC;
	REVOKE ALL ON SCHEMA public from PUBLIC;
	REVOKE ALL ON ALL TABLES IN SCHEMA public from PUBLIC;`, credentials.Name))
	if err != nil {
		return fmt.Errorf("revoke all for role PUBLIC on database '%s': %w", credentials.Name, err)
	}
	return nil
}
