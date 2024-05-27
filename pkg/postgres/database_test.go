package postgres_test

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestParseUsernamePassword(t *testing.T) {
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
				User:     "user",
				Password: "password",
			},
			err: nil,
		},
		{
			name:  "no password",
			input: "user",
			output: postgres.Credentials{
				User:     "user",
				Password: "",
			},
			err: nil,
		},
		{
			name:  "empty password",
			input: "user:",
			output: postgres.Credentials{
				User:     "user",
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
	managerRole := "postgres_role_name"
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create manager role failed: %v", err)
	}
	defer db.Close()

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(logf.Log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name:     name,
			User:     name,
			Password: password,
		}, managerRole)
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	assert.True(t, roleCanLogin(t, db, name))
	assert.True(t, hasPassword(t, log, postgresqlHost, name))

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

func TestDatabase_noPassword(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	managerRole := "postgres_role_name"
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create manager role failed: %v", err)
	}
	defer db.Close()

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())

	err = postgres.Database(logf.Log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name: name,
			User: name,
		}, managerRole)
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	assert.False(t, roleCanLogin(t, db, name))

	newDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     "iam_creator",
		Password: "iam_creator",
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

func TestDatabase_switchFromLoginToNoLoginAndBack(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	managerRole := "postgres_role_name"

	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create managerRole: %v", err)
	}
	defer db.Close()

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(log, postgresqlHost, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name:     name,
		User:     name,
		Password: password,
	}, managerRole)
	if err != nil {
		t.Fatalf("Database failed: %v", err)
	}

	assert.True(t, roleCanLogin(t, db, name))
	assert.True(t, hasPassword(t, log, postgresqlHost, name))

	// Invoke again with same name but no password
	err = postgres.Database(log, postgresqlHost, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name: name,
		User: name,
	}, managerRole)
	if err != nil {
		t.Logf("The error: %#v", err)
		t.Fatalf("Second Database failed: %v", err)
	}
	assert.False(t, roleCanLogin(t, db, name))
	assert.False(t, hasPassword(t, log, postgresqlHost, name))

	// Invoke again with same name with password
	err = postgres.Database(log, postgresqlHost, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name:     name,
		User:     name,
		Password: password,
	}, managerRole)
	if err != nil {
		t.Logf("The error: %#v", err)
		t.Fatalf("Second Database failed: %v", err)
	}
	assert.True(t, roleCanLogin(t, db, name))
	assert.True(t, hasPassword(t, log, postgresqlHost, name))

	newDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     "iam_creator",
		Password: "iam_creator",
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
	managerRole := "postgres_role_name"
	log.Info("TC: Connection as iam_creator")
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer db.Close()

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
	defer serviceDB.Close()
	log.Info("TC: Create schema, table and insert a row")
	dbExec(t, serviceDB, fmt.Sprintf(`
	CREATE SCHEMA %[1]s;
	CREATE TABLE %[1]s.%[1]s (title varchar(40));
	INSERT INTO %[1]s.%[1]s VALUES('a product');
	`, name))

	log.Info("TC: Run controller database creation")
	err = postgres.Database(log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name:     name,
			User:     name,
			Password: password,
		}, managerRole)
	if err != nil {
		t.Fatalf("Create service database failed: %v", err)
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
		Password: password,
	})
	if err != nil {
		t.Fatalf("Connect as developer user failed: %v", err)
	}
	defer developerDB.Close()
	// This should not result in an error as the controller should have made sure
	// that the schema and table have been made available to the read and
	// readwrite roles
	log.Info("TC: Select from table")
	dbExec(t, developerDB, fmt.Sprintf(`SELECT * FROM %[1]s.%[1]s`, name))
}

// TestDatabase_defaultDatabaseName tests that we can handle database resources
// referencing the default name of the database instance.
func TestDatabase_defaultDatabaseName(t *testing.T) {
	postgresqlHost := test.Integration(t)
	managerRole := "postgres_role_name"
	log := test.SetLogger(t)
	log.Info("TC: Connecting as iam_creator")
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer db.Close()

	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create manager role failed: %v", err)
	}

	// setup a database that will be shared
	log.Info("TC: Create a legacy database that will be shared with other services")
	err = postgres.Database(log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name:     "legacy",
			User:     "legacy",
			Password: "legacy_pass",
			Shared:   false,
		}, managerRole)
	if err != nil {
		t.Fatalf("create legacy database failed: %v", err)
	}

	// setup a new schema on the shared database
	log.Info("TC: Request new database using default postgres database (postgres)")
	err = postgres.Database(log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name:     "legacy",
			User:     "service",
			Password: "service_pass",
			Shared:   true,
		}, managerRole)
	if err != nil {
		t.Fatalf("Create service database failed: %v", err)
	}
}

// TestDatabase_mixedOwnershipOnSharedDatabase tests that we can handle shared
// databases where ownership is mixed. It will setup a shared database and
// create a role for it. It then creates a table with the shared user and a
// table with a service user. Lastly it verifies that both the service role and
// a developer requesting access to it can access data on both the non-owned and
// owned tables.
func TestDatabase_mixedOwnershipOnSharedDatabase(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.NewLogger(t)
	log.Info("TC: Connecting as iam_creator on defaul database")
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to default database failed: %v", err)
	}
	defer db.Close()

	epoch := time.Now().UnixNano()
	sharedDatabaseName := fmt.Sprintf("shared_%d", epoch)
	newUser := fmt.Sprintf("new_user_%d", epoch)
	developer := fmt.Sprintf("developer_%d", epoch)
	managerRole := "postgres_role_name"

	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create manager role failed: %v", err)
	}

	// create the shared database with a role of the same name and owned by the
	// shared role
	dbExec(t, db, `CREATE ROLE %s WITH login`, sharedDatabaseName)
	dbExec(t, db, `CREATE ROLE %s_read`, sharedDatabaseName)
	dbExec(t, db, `CREATE ROLE %s_readwrite`, sharedDatabaseName)
	dbExec(t, db, `CREATE DATABASE %s`, sharedDatabaseName)
	dbExec(t, db, `GRANT %s TO CURRENT_USER`, sharedDatabaseName)
	dbExec(t, db, `ALTER DATABASE %s OWNER TO %s`, sharedDatabaseName, sharedDatabaseName)
	dbExec(t, db, `REVOKE %s FROM CURRENT_USER`, sharedDatabaseName)

	// connect to shared database with created role
	sharedConn, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: sharedDatabaseName,
		User:     sharedDatabaseName,
		Password: sharedDatabaseName,
	})
	if err != nil {
		t.Fatalf("connect to sahred database failed: %v", err)
	}
	defer sharedConn.Close()

	// create schema and table that is to be used by a new role but owned by the
	// existing shared user
	dbExec(t, sharedConn, `CREATE SCHEMA %s`, newUser)
	dbExec(t, sharedConn, `CREATE TABLE %s.not_owned (title varchar(40) NOT NULL)`, newUser)
	dbExec(t, sharedConn, `INSERT INTO %s.not_owned VALUES('value-from-shared-user')`, newUser)

	// create schema and table that should not be possible to access with the new
	// shared user
	dbExec(t, sharedConn, `CREATE SCHEMA another`)
	dbExec(t, sharedConn, `CREATE TABLE another.another (title varchar(40) NOT NULL)`)
	dbExec(t, sharedConn, `INSERT INTO another.another VALUES('another')`)

	// this is the functionality we want to ensure works. It requests access to a
	// shared database with a new user where the schema exists created by the
	// shared user
	log.Info("TC: Create new_user database on shared database")
	err = postgres.Database(log, postgresqlHost,
		postgres.Credentials{
			User:     "iam_creator",
			Password: "iam_creator",
		}, postgres.Credentials{
			Name:     sharedDatabaseName,
			User:     newUser,
			Password: newUser,
			Shared:   true,
		}, managerRole)
	if err != nil {
		t.Fatalf("create new_user schema on shared database failed: %v", err)
	}

	// connect as the new user and do some queries to ensure permissions are
	// correct
	newUserConn, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: sharedDatabaseName,
		User:     newUser,
		Password: newUser,
	})
	if err != nil {
		t.Fatalf("connect to shared database failed: %v", err)
	}
	defer newUserConn.Close()

	// validate that we can create tables and insert data into them
	dbExec(t, newUserConn, `CREATE TABLE %s.owned (title varchar(40) NOT NULL)`, newUser)
	dbExec(t, newUserConn, `INSERT INTO %s.owned VALUES('owned-row')`, newUser)

	// validate that we can insert rows into the existing non-owned table
	dbExec(t, newUserConn, `INSERT INTO %s.not_owned VALUES('value-from-new-user')`, newUser)

	// validate that we can query data from the owned table
	ownedRows := dbQuery(t, newUserConn, `SELECT * FROM %s.owned`, newUser)
	assert.Equal(t, []string{"owned-row"}, ownedRows, "owned rows not as expected")

	// validate that we can query data from the existing non-owned table
	nonOwned := dbQuery(t, newUserConn, `SELECT * FROM %s.not_owned`, newUser)
	assert.Equal(t, []string{"value-from-new-user", "value-from-shared-user"}, nonOwned, "nonowned rows not as expected")

	// request access to the new user schema of the shared database
	err = postgres.Role(log, db, developer, nil, []postgres.DatabaseSchema{
		{
			Name:       sharedDatabaseName,
			Privileges: postgres.PrivilegeRead,
			Schema:     newUser,
		},
	})
	if err != nil {
		t.Fatalf("create developer role to new user database failed: %v", err)
	}

	// connect as the developer on the shared database
	developerConn, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: sharedDatabaseName,
		User:     developer,
		Password: developer,
	})
	if err != nil {
		t.Fatalf("connect to newUser with developer failed: %v", err)
	}
	defer developerConn.Close()

	// validate that the developer can query data from the owned table
	developerOwnedRows := dbQuery(t, developerConn, `SELECT * FROM %s.owned`, newUser)
	assert.Equal(t, []string{"owned-row"}, developerOwnedRows, "owned rows not as expected")

	// validate that we can query data from the existing non-owned table
	developerNonOwnedRows := dbQuery(t, developerConn, `SELECT * FROM %s.not_owned`, newUser)
	assert.Equal(t, []string{"value-from-new-user", "value-from-shared-user"}, developerNonOwnedRows, "nonowned rows not as expected")
}

func TestDatabase_idempotency(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	managerRole := "postgres_role_name"
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	err = createManagerRole(log, db, managerRole)
	if err != nil {
		t.Fatalf("create managerRole: %v", err)
	}
	defer db.Close()

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(log, postgresqlHost, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name:     name,
		User:     name,
		Password: password,
	}, managerRole)
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	// Invoke again with same name
	err = postgres.Database(log, postgresqlHost, postgres.Credentials{
		User:     "iam_creator",
		Password: "iam_creator",
	}, postgres.Credentials{
		Name:     name,
		User:     name,
		Password: password,
	}, managerRole)
	if err != nil {
		t.Logf("The error: %#v", err)
		t.Fatalf("Second EnsurePostgreSQLDatabase failed: %v", err)
	}
}

func hasPassword(t *testing.T, log logr.Logger, host, username string) bool {
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "admin",
		Password: "admin",
	})
	if err != nil {
		t.Fatalf("connect to database as admin failed: %v", err)
	}

	row := db.QueryRow("SELECT passwd FROM pg_shadow WHERE usename = $1", username)
	if row.Err() != nil {
		t.Fatalf("get password failed: %v", row.Err())
	}

	var password string
	err = row.Scan(&password)
	return err == nil
}

func roleCanLogin(t *testing.T, db *sql.DB, role string) bool {
	t.Helper()
	row := db.QueryRow("SELECT rolcanlogin FROM pg_roles WHERE rolname = $1", role)

	var rolcanlogin bool
	err := row.Scan(&rolcanlogin)
	if err != nil {
		t.Fatalf("get rolcanlogin failed: %v", err)
	}
	return rolcanlogin
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

func createManagerRole(log logr.Logger, db *sql.DB, roleName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE ROLE %s LOGIN;", roleName))
	if err != nil {
		pqError, ok := err.(*pq.Error)
		if !ok || pqError.Code.Name() != "duplicate_object" {
			return err
		}
		log.Info("role already exists", "errorCode", pqError.Code, "errorName", pqError.Code.Name())
	} else {
		log.Info("role created")
	}
	return nil
}
