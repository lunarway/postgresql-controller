package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

// ConnectionString represents a PostgreSQL connection string.
//
// Use method Raw to get the raw connection string for sql.Open.
//
// The type masks the password when used with fmt.Stringer e.g. fmt.Sprintf.
type ConnectionString struct {
	Host     string
	Database string
	User     string
	Password string
}

// Raw returns a PostgreSQL connection string.
func (c ConnectionString) Raw() string {
	raw := fmt.Sprintf("postgresql://%s:%s@%s", c.User, c.Password, c.Host)
	if c.Database != "" {
		raw += fmt.Sprintf("/%s", c.Database)
	}
	raw += "?sslmode=disable"
	return raw
}

var _ fmt.Stringer = ConnectionString{}

func (c ConnectionString) String() string {
	raw := c.Raw()
	if c.Password == "" {
		return raw
	}
	return strings.ReplaceAll(raw, c.Password, "********")
}

func Connect(log logr.Logger, connectionString ConnectionString) (*sql.DB, error) {
	log.Info("Connecting to database", "url", connectionString)
	db, err := sql.Open("postgres", connectionString.Raw())
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(0)
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

func (p Privilege) String() string {
	switch p {
	case PrivilegeRead:
		return "read"
	case PrivilegeWrite:
		return "write"
	default:
		return "unknown"
	}
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
			return fmt.Errorf("create role %s: %w", name, err)
		}
		log.Info(fmt.Sprintf("Role %s already exists", name), "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info(fmt.Sprintf("Role %s created", name))
	}
	if len(roles) != 0 {
		_, err = db.Exec(fmt.Sprintf("GRANT %s TO %s", strings.Join(roles, ", "), name))
		if err != nil {
			return fmt.Errorf("grant static roles '%v': %w", roles, err)
		}
	}

	for _, database := range databases {
		var schemaPrivileges string
		if database.Privileges == PrivilegeRead {
			schemaPrivileges = "read"
		}
		if database.Privileges == PrivilegeWrite {
			schemaPrivileges = "readwrite"
		}
		if len(schemaPrivileges) == 0 {
			continue
		}
		log.Info(fmt.Sprintf("Granting %s to %s", schemaPrivileges, name))
		_, err = db.Exec(fmt.Sprintf("GRANT %s_%s TO %s", database.Name, schemaPrivileges, name))
		if err != nil {
			return fmt.Errorf("grant access privileges '%s' on schema '%s': %w", schemaPrivileges, database.Schema, err)
		}
	}
	return nil
}
