package postgres_test

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestParseHostCredentials(t *testing.T) {
	tt := []struct {
		name   string
		input  string
		output postgres.Credentials
		err    error
	}{
		{
			name:   "empty string",
			input:  "",
			output: postgres.Credentials{},
			err:    errors.New("username empty"),
		},
		{
			name:  "complete",
			input: "user:password",
			output: postgres.Credentials{
				Name:     "user",
				Password: "password",
			},
			err: nil,
		},
		{
			name:  "no password",
			input: "user",
			output: postgres.Credentials{
				Name:     "user",
				Password: "",
			},
			err: nil,
		},
		{
			name:  "empty password",
			input: "user:",
			output: postgres.Credentials{
				Name:     "user",
				Password: "",
			},
			err: nil,
		},
		{
			name:   "empty username and password",
			input:  ":",
			output: postgres.Credentials{},
			err:    errors.New("username empty"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output, err := postgres.ParseUsernamePassword(tc.input)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as expected")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}

func TestDatabase_sunshine(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(logf.Log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	newDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	// Validate Schema
	schemas := storedSchema(t, newDB, name)
	assert.Equal(t, []string{name}, schemas, "schema not as expected")

	// Validate iam_creator not able to see schema
	schemas = storedSchema(t, db, name)
	assert.Equal(t, []string(nil), schemas, "schema not as expected")

	// Validate owner of database
	owners := validateOwner(t, db, name)
	t.Logf("Owners of database: %v", owners)
	assert.Equal(t, []string{name}, owners, "owner not as expected")
}

// TestDatabase_existingResourcePrivilegesForReadWriteRoles tests that we can
// gain access to resources created prior to the read and readwrite roles by a
// service role.
//
// This validates that users can adopt use of this controller with existing
// databases and resources without any manual intervention.
func TestDatabase_existingResourcePrivilegesForReadWriteRoles(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	log.Info("TC: Connection as iam_creator")
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	developerName := fmt.Sprintf("%s_developer", name)
	password := "test"

	// create a database and resources with plain SQL is if it already exists when
	// the controller tries to reconcile.
	log.Info("TC: Creating user and database and changes owner")
	dbExec(t, db, fmt.Sprintf(`CREATE USER %s WITH PASSWORD '%s' NOCREATEROLE VALID UNTIL 'infinity'`, name, password))
	dbExec(t, db, fmt.Sprintf(`CREATE DATABASE %s`, name))
	dbExec(t, db, fmt.Sprintf(`
	GRANT %[1]s TO CURRENT_USER;
	ALTER DATABASE %[1]s OWNER TO %[1]s;
	REVOKE %[1]s FROM CURRENT_USER;
	`, name))

	log.Info("TC: Connect as service user")
	serviceDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("Connect as existing service user failed: %v", err)
	}
	log.Info("TC: Create schema, table and insert a row")
	dbExec(t, serviceDB, fmt.Sprintf(`
	CREATE SCHEMA %[1]s;
	CREATE TABLE %[1]s.%[1]s (title varchar(40));
	INSERT INTO %[1]s.%[1]s VALUES('a product');
	`, name))

	log.Info("TC: Run controller database creation")
	err = postgres.Database(log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("Create service database failed: %v", err)
	}

	// reconnect to get newly granted rights
	log.Info("TC: Reconnect as iam_creator")
	db, err = postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	log.Info("TC: Run controller user creation")
	err = postgres.Role(log, db, developerName, nil, []postgres.DatabaseSchema{{
		Name:       name,
		Schema:     name,
		Privileges: postgres.PrivilegeRead,
	}})
	if err != nil {
		t.Fatalf("Create new developer role failed: %v", err)
	}

	log.Info("TC: Connect as developer")
	developerDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     developerName,
		Password: "",
	})
	if err != nil {
		t.Fatalf("Connect as developer user failed: %v", err)
	}
	// This should not result in an error as the controller should have made sure
	// that the product schema and table have been made available to the read and
	// readwrite roles
	log.Info("TC: Select from table")
	dbExec(t, developerDB, fmt.Sprintf(`SELECT * FROM %[1]s.%[1]s`, name))
}

func TestDatabase_idempotency(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	// Invoke again with same name
	err = postgres.Database(log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Logf("The error: %#v", err)
		t.Fatalf("Second EnsurePostgreSQLDatabase failed: %v", err)
	}
}

func validateOwner(t *testing.T, db *sql.DB, owner string) []string {
	t.Helper()
	rows, err := db.Query("SELECT pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname = $1", owner)
	if err != nil {
		t.Fatalf("get owner failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

// storedRoles returns roles for a specific user name sorted by name.
func storedSchema(t *testing.T, db *sql.DB, schemaName string) []string {
	t.Helper()
	rows, err := db.Query("select schema_name from information_schema.schemata where schema_name = $1", schemaName)
	if err != nil {
		t.Fatalf("get schema for schema query failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

func stringsResult(t *testing.T, rows *sql.Rows) []string {
	var results []string
	for rows.Next() {
		var result string
		err := rows.Scan(&result)
		if err != nil {
			t.Fatalf("scan row failed: %v", err)
		}
		results = append(results, result)
	}
	err := rows.Err()
	if err != nil {
		t.Fatalf("scanning rows failed: %v", err)
	}
	sort.Strings(results)
	return results
}
