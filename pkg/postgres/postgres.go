package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

func Connect(log logr.Logger, connectionString string) (*sql.DB, error) {
	log.Info("Connecting to database", "url", connectionString)
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}

type Privilege int

const (
	PrivilegeRead  Privilege = iota
	PrivilegeWrite Privilege = iota
)

type DatabaseSchema struct {
	Name       string
	Schema     string
	Privileges Privilege
}

func Role(log logr.Logger, db *sql.DB, name string, roles []string, databases []DatabaseSchema) error {
	log.Info(fmt.Sprintf("Creating role %s", name))
	query := fmt.Sprintf("CREATE ROLE %s WITH LOGIN", name)
	if len(roles) != 0 {
		query += fmt.Sprintf(" IN ROLE %s", strings.Join(roles, ", "))
	}
	_, err := db.Exec(query)
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return err
		}
		log.Info(fmt.Sprintf("Role %s already exists", name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Role %s created", name))
	}
	if len(roles) != 0 {
		_, err = db.Exec(fmt.Sprintf("GRANT %s TO %s", strings.Join(roles, ", "), name))
		if err != nil {
			return err
		}
	}

	for _, database := range databases {
		// Only needed for testing without rds_iam role that otherwise grants this right
		log.Info(fmt.Sprintf("Granting CONNECT to database '%s'", database.Name))
		_, err = db.Exec(fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", database.Name, name))
		if err != nil {
			return fmt.Errorf("grant connect on database '%s': %w", database.Name, err)
		}
		log.Info(fmt.Sprintf("Granting USAGE of schema '%s'", database.Name))
		_, err = db.Exec(fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", database.Name, name))
		if err != nil {
			return fmt.Errorf("grant usage on schema '%s': %w", database.Name, err)
		}
		var schemaPrivileges string
		if database.Privileges == PrivilegeRead {
			schemaPrivileges = "SELECT"
		}
		if database.Privileges == PrivilegeWrite {
			schemaPrivileges = "SELECT, INSERT, UPDATE, DELETE"
		}
		if len(schemaPrivileges) == 0 {
			continue
		}
		log.Info(fmt.Sprintf("Granting %s to tables in schema '%s'", schemaPrivileges, database.Name))
		_, err = db.Exec(fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", schemaPrivileges, database.Name, name))
		if err != nil {
			return fmt.Errorf("grant access privileges '%s' on schema '%s': %w", schemaPrivileges, database.Name, err)
		}
	}
	return nil
}
