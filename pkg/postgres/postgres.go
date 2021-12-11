package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
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
	Params   string
}

// Raw returns a PostgreSQL connection string.
func (c ConnectionString) Raw() string {
	raw := fmt.Sprintf("postgresql://%s:%s@%s", c.User, url.QueryEscape(c.Password), c.Host)
	if c.Database != "" {
		raw += fmt.Sprintf("/%s", c.Database)
	}
	if c.Params != "" {
		raw += fmt.Sprintf("?%s", c.Params)
	} else {
		// backwards compatibility
		raw += "?sslmode=disable"
	}
	return raw
}

var _ fmt.Stringer = ConnectionString{}

func (c ConnectionString) String() string {
	raw := c.Raw()
	if c.Password == "" {
		return raw
	}
	return strings.ReplaceAll(raw, url.QueryEscape(c.Password), "********")
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
	PrivilegeRead Privilege = iota
	PrivilegeWrite
	PrivilegeOwningWrite
)

const (
	roleSuffixRead        = "read"
	roleSuffixWrite       = "readwrite"
	roleSuffixOwningWrite = "readowningwrite"
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
	case PrivilegeOwningWrite:
		return "owningwrite"
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

	// grant database access roles to created role
	existingRoles, err := persistedRoles(db, name)
	if err != nil {
		return fmt.Errorf("get existing roles: %w", err)
	}
	grantableRoles, revokeableRoles := rolesDiff(log, existingRoles, roles, databases)
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

// rolesDiff returns roles to add and remove from existingRoles slice based of
// the databases that are requested access to.
func rolesDiff(log logr.Logger, existingRoles []string, expectedRoles []string, databases []DatabaseSchema) ([]string, []string) {
	// append to expectedRoles for each database access request
	for _, database := range databases {
		var schemaPrivileges string
		switch database.Privileges {
		case PrivilegeRead:
			schemaPrivileges = roleSuffixRead
		case PrivilegeWrite:
			schemaPrivileges = roleSuffixWrite
		case PrivilegeOwningWrite:
			schemaPrivileges = roleSuffixOwningWrite
		default:
			log.Error(errors.New("priviledge unknown"), fmt.Sprintf("dropped database '%s.%s' as priviledge '%s' (%[3]d) is invalid", database.Name, database.Schema, database.Privileges), "database", database)
			continue
		}
		schema := database.Schema
		if strings.EqualFold(schema, "public") {
			schema = database.Name
		}
		schemaPrivileges = fmt.Sprintf("%s_%s", schema, schemaPrivileges)
		expectedRoles = append(expectedRoles, schemaPrivileges)
	}

	// find roles that are expected but not on the existing roles list
	var addableRoles []string
	for _, expectedRole := range expectedRoles {
		if contains(existingRoles, expectedRole) {
			continue
		}
		addableRoles = append(addableRoles, expectedRole)
	}

	// find existing roles that are not in the expected list
	var removeableRoles []string
	for _, existingRole := range existingRoles {
		if contains(expectedRoles, existingRole) {
			continue
		}
		// only remove roles that look like some we control, ie. suffixed with _read
		// or _readwrite. This is to make sure we do not change roles granted out of
		// band to specific users.
		suffixes := []string{
			roleSuffixRead,
			roleSuffixWrite,
			roleSuffixOwningWrite,
		}
		var hasSuffix bool
		for _, suffix := range suffixes {
			if strings.HasSuffix(existingRole, suffix) {
				hasSuffix = true
				break
			}
		}
		if !hasSuffix {
			continue
		}
		removeableRoles = append(removeableRoles, existingRole)
	}

	return addableRoles, removeableRoles
}

func contains(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
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
