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
	return GrantRoles(log, db, name, roles, databases)
}

// GrantRoles grants role name with all roles from the roles slice along with
// extrated roles from databases slice.
func GrantRoles(log logr.Logger, db *sql.DB, name string, roles []string, databases []DatabaseSchema) error {
	grantableRoles, revokeableRoles, err := rolesDiff(db, name, roles, databases)
	if err != nil {
		return fmt.Errorf("resolve roles diff: %w", err)
	}
	log.Info(fmt.Sprintf("Found %d grantable and %d revokable roles for %s", len(grantableRoles), len(revokeableRoles), name), "grantable", grantableRoles, "revokeable", revokeableRoles)
	if len(grantableRoles) != 0 {
		joinedRoles := strings.Join(grantableRoles, ",")
		_, err = db.Exec(fmt.Sprintf("GRANT %s TO %s", joinedRoles, name))
		if err != nil {
			return fmt.Errorf("grant access privileges '%s' to '%s': %w", joinedRoles, name, err)
		}
	}
	if len(revokeableRoles) != 0 {
		joinedRoles := strings.Join(revokeableRoles, ",")
		_, err = db.Exec(fmt.Sprintf("REVOKE %s FROM %s", joinedRoles, name))
		if err != nil {
			return fmt.Errorf("revoke access privileges '%s' to '%s': %w", joinedRoles, name, err)
		}
	}
	return nil
}

func rolesDiff(db *sql.DB, name string, staticRoles []string, databases []DatabaseSchema) ([]string, []string, error) {
	var grantableRoles []string
	if len(staticRoles) != 0 {
		grantableRoles = append(grantableRoles, staticRoles...)
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
		grantableRoles = append(grantableRoles, fmt.Sprintf("%s_%s", database.Name, schemaPrivileges))
	}
	existingRoles, err := persistedRoles(db, name)
	if err != nil {
		return nil, nil, fmt.Errorf("get existing roles: %w", err)
	}
	var addableRoles, removeableRoles []string
	for _, grantableRole := range grantableRoles {
		for _, existingRole := range existingRoles {
			if existingRole == grantableRole {
				// FIXME: implement this filtering
			}
		}
	}
	return addableRoles, removeableRoles, nil
}

func persistedRoles(db *sql.DB, name string) ([]string, error) {
	rows, err := db.Query("SELECT rolname FROM pg_user JOIN pg_auth_members ON (pg_user.usesysid=pg_auth_members.member) JOIN pg_roles ON (pg_roles.oid=pg_auth_members.roleid) WHERE pg_user.usename=$1", name)
	if err != nil {
		return nil, fmt.Errorf("select roles: %w", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var rolName string
		err = rows.Scan(&rolName)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		roles = append(roles, rolName)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("scanning rows: %w", err)
	}
	return roles, nil
}
