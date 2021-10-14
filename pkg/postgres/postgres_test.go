package postgres_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestConnectionString_Raw(t *testing.T) {
	tt := []struct {
		name             string
		connectionString postgres.ConnectionString
		raw              string
	}{
		{
			name: "no password or database",
			connectionString: postgres.ConnectionString{
				Host:     "host:5432",
				Database: "",
				User:     "user",
				Password: "",
			},
			raw: "postgresql://user:@host:5432?sslmode=disable",
		},
		{
			name: "no password",
			connectionString: postgres.ConnectionString{
				Host:     "host:5432",
				Database: "database",
				User:     "user",
				Password: "",
			},
			raw: "postgresql://user:@host:5432/database?sslmode=disable",
		},
		{
			name: "complete",
			connectionString: postgres.ConnectionString{
				Host:     "host:5432",
				Database: "database",
				User:     "user",
				Password: "1234",
			},
			raw: "postgresql://user:1234@host:5432/database?sslmode=disable",
		},
		{
			name: "complete with params",
			connectionString: postgres.ConnectionString{
				Host:     "host:5432",
				Database: "database",
				User:     "user",
				Password: "1234",
				Params:   "sslmode=strict",
			},
			raw: "postgresql://user:1234@host:5432/database?sslmode=strict",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.connectionString.Raw()
			assert.Equal(t, tc.raw, raw, "raw connection string not as expected")
		})
	}
}

// TestConnectionString_String tests that ConnectionString does not expose
// password for fmt.Stringer and fmt.Formatter
func TestConnectionString_String(t *testing.T) {
	connectionString := postgres.ConnectionString{
		Host:     "host:5432",
		Database: "database",
		User:     "user",
		Password: "1234",
	}
	expected := "postgresql://user:********@host:5432/database?sslmode=disable"
	assert.Equal(t, fmt.Sprintf("%s", connectionString), expected, "connection string not as expected") //nolint:gosimple
	assert.Equal(t, fmt.Sprintf("%v", connectionString), expected, "connection string not as expected")
	assert.Equal(t, expected, connectionString.String(), "connection string not as expected")
}

// TestConnectionString_logger tests that ConnectionString does not expose
// password for the logging implementation.
func TestConnectionString_logger(t *testing.T) {
	connectionString := postgres.ConnectionString{
		Host:     "host:5432",
		Database: "database",
		User:     "user",
		Password: "1234",
	}
	var b bytes.Buffer
	log.SetLogger(zap.New(zap.WriteTo(&b), zap.UseDevMode(true)))
	log.Log.Info("Connection string", "conn", connectionString)
	assert.NotContains(t, b.String(), "1234", "password logged")
}

// TestConnect_idleConnections tests that we release connections properly for
// each connection and don't let the sql.DB connection pool keep them open.
func TestConnect_idleConnections(t *testing.T) {
	postgresqlHost := test.Integration(t)
	logger := &test.RawLogger{
		T: t,
	}
	connectionString := postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: "postgres",
		Password: "",
	}

	// connect 100 times without closing the connections before completing the
	// test. This will result in 100 idle connections short after creation and if
	// the connection pool is properly configured the idle connections will be
	// dropped before we call Close.
	for i := 0; i < 100; i++ {
		conn, err := postgres.Connect(logger, connectionString)
		if err != nil {
			t.Fatalf("connect to database failed: %v", err)
		}
		defer conn.Close()
	}
}

func TestRole_staticRoles(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: "postgres",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer db.Close()
	var (
		epoch            = time.Now().UnixNano()
		RoleRDSIAM       = fmt.Sprintf("rds_iam_%d", epoch)
		RoleIAMDeveloper = fmt.Sprintf("iam_developer_%d", epoch)
		RoleOther        = fmt.Sprintf("other_role_%d", epoch)
	)
	// roles used for testing
	roles := []string{
		RoleRDSIAM,
		RoleIAMDeveloper,
		RoleOther,
	}
	// bootstrap the database with the roles that can be granted by the controller
	for _, role := range roles {
		dbExec(t, db, "CREATE ROLE %s", role)
	}
	dbExec(t, db, "GRANT CONNECT ON DATABASE %s TO %s", "postgres", RoleRDSIAM)
	defer dbExec(t, db, "REVOKE CONNECT ON DATABASE %s FROM %s", "postgres", RoleRDSIAM)
	tt := []struct {
		name          string
		createRole    bool
		existingRoles []string
		roles         []string
	}{
		{
			name:          "new user without any roles",
			createRole:    false,
			existingRoles: nil,
			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
		},
		{
			name:          "existing user without any roles",
			createRole:    true,
			existingRoles: nil,
			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
		},
		{
			name:          "user exists with correct roles",
			createRole:    true,
			existingRoles: []string{RoleIAMDeveloper, RoleRDSIAM},
			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
		},
		{
			name:          "user exists with incomplete roles",
			createRole:    true,
			existingRoles: []string{RoleRDSIAM},
			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
		},
		{
			name:          "user exists with other roles",
			createRole:    true,
			existingRoles: []string{RoleOther},
			roles:         []string{RoleIAMDeveloper, RoleOther, RoleRDSIAM},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			test.SetLogger(t)

			userName := fmt.Sprintf("test_user_%d", time.Now().UnixNano())
			t.Logf("Using user name %s", userName)

			if tc.createRole {
				createRole(t, db, userName)
			}

			if len(tc.existingRoles) != 0 {
				seedRole(t, db, userName, tc.existingRoles)
			}

			// act
			err = postgres.Role(log, db, userName, []string{
				RoleRDSIAM,
				RoleIAMDeveloper,
			}, nil)

			// assert
			assert.NoError(t, err, "unexpected output error")

			roles := storedRoles(t, db, userName)
			t.Logf("Stored roles: %v", roles)
			assert.Equal(t, tc.roles, roles, "roles on user not as expected")
		})
	}
}

// TestRole_priviliges_databaseNameAndSchemaDiffers tests that Role can grant
// access to another schema than that of the same name as the database. Normally
// a schema with the same name as the database is expected but some services
// uses the public schema.
func TestRole_priviliges_databaseNameAndSchemaDiffers(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)

	iamCreatorRootDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: "postgres",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorRootDB.Close()

	var (
		now           = time.Now().UnixNano()
		serviceUser1  = fmt.Sprintf("test_svc_1_%d", now)
		developerUser = fmt.Sprintf("test_user_%d", now)
		roleRDSIAM    = fmt.Sprintf("rds_iam_%d", now)
	)
	log.Info(fmt.Sprintf("Running test with service user %s and developer %s", serviceUser1, developerUser))

	// create service databases and tables for testing access rights
	createServiceDatabase(t, log, iamCreatorRootDB, postgresqlHost, serviceUser1)
	createRole(t, iamCreatorRootDB, roleRDSIAM)
	dbExec(t, iamCreatorRootDB, "GRANT CONNECT ON DATABASE %s TO %s", serviceUser1, roleRDSIAM)

	// reconnect to start a new session with grants from above database creation
	iamCreatorUserDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorUserDB.Close()

	dbExec(t, iamCreatorRootDB, "GRANT CONNECT ON DATABASE %s TO %s", serviceUser1, roleRDSIAM)
	err = postgres.Role(log, iamCreatorUserDB, developerUser, []string{roleRDSIAM}, []postgres.DatabaseSchema{
		{
			Name:       serviceUser1,
			Schema:     "public",
			Privileges: postgres.PrivilegeRead,
		},
	})
	if !assert.NoError(t, err, "unexpected output error") {
		return
	}

	userDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     developerUser,
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("could not connect with new user: %v", err)
	}
	defer userDB.Close()
	_, err = userDB.Query("SELECT * FROM public.public_films")
	if err != nil {
		t.Fatalf("could not select from public.public_films table: %v", err)
	}
}

// TestRole_owningWritePriviliges tests that Role can grant owner privileges if
// requested making it possible to DROP and ALTER tables.
func TestRole_owningWritePriviliges(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)

	iamCreatorRootDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: "postgres",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorRootDB.Close()

	var (
		now           = time.Now().UnixNano()
		serviceUser1  = fmt.Sprintf("test_svc_1_%d", now)
		developerUser = fmt.Sprintf("test_user_%d", now)
		roleRDSIAM    = fmt.Sprintf("rds_iam_%d", now)
	)
	log.Info(fmt.Sprintf("Running test with service users %s and developer %s", serviceUser1, developerUser))

	// create service databases and tables for testing access rights
	createServiceDatabase(t, log, iamCreatorRootDB, postgresqlHost, serviceUser1)
	createRole(t, iamCreatorRootDB, roleRDSIAM)
	dbExec(t, iamCreatorRootDB, "GRANT CONNECT ON DATABASE %s TO %s", serviceUser1, roleRDSIAM)

	//
	// test alter and drop privilege on serviceUser1
	//

	// reconnect to start a new session with grants from above database creation
	iamCreatorUserDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorUserDB.Close()
	err = postgres.Role(log, iamCreatorUserDB, developerUser, []string{roleRDSIAM}, []postgres.DatabaseSchema{
		{
			Name:       serviceUser1,
			Schema:     serviceUser1,
			Privileges: postgres.PrivilegeOwningWrite,
		},
	})
	if !assert.NoError(t, err, "unexpected output error") {
		return
	}

	userDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     developerUser,
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("could not connect with new user: %v", err)
	}
	defer userDB.Close()
	_, err = userDB.Exec(fmt.Sprintf("ALTER TABLE %s.films ADD description text", serviceUser1))
	if err != nil {
		t.Fatalf("could not alter %s.films table: %v", serviceUser1, err)
	}
	_, err = userDB.Exec("DROP TABLE public.public_films")
	if err != nil {
		t.Fatalf("could not drop %s.films table: %v", serviceUser1, err)
	}

	// ensure we revoke the privilege again

	iamCreatorUserDB, err = postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorUserDB.Close()
	err = postgres.Role(log, iamCreatorUserDB, developerUser, []string{roleRDSIAM}, []postgres.DatabaseSchema{
		{
			Name:       serviceUser1,
			Schema:     serviceUser1,
			Privileges: postgres.PrivilegeWrite,
		},
	})
	if !assert.NoError(t, err, "unexpected output error") {
		return
	}

	userDB, err = postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     developerUser,
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("could not connect with new user: %v", err)
	}
	defer userDB.Close()
	_, err = userDB.Exec(fmt.Sprintf("ALTER TABLE %s.films ADD description text", serviceUser1))
	if err == nil {
		t.Fatalf("could still alter %s.films table", serviceUser1)
	}
	_, err = userDB.Exec(fmt.Sprintf("DROP TABLE %s.films", serviceUser1))
	if err == nil {
		t.Fatalf("could still drop %s.films table", serviceUser1)
	}
}

func TestRole_priviliges(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)

	iamCreatorRootDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: "postgres",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorRootDB.Close()

	var (
		now           = time.Now().UnixNano()
		serviceUser1  = fmt.Sprintf("test_svc_1_%d", now)
		serviceUser2  = fmt.Sprintf("test_svc_2_%d", now)
		developerUser = fmt.Sprintf("test_user_%d", now)
		roleRDSIAM    = fmt.Sprintf("rds_iam_%d", now)
	)
	log.Info(fmt.Sprintf("Running test with service users %s, %s and developer %s", serviceUser1, serviceUser2, developerUser))

	// create service databases and tables for testing access rights
	createServiceDatabase(t, log, iamCreatorRootDB, postgresqlHost, serviceUser1)
	createServiceDatabase(t, log, iamCreatorRootDB, postgresqlHost, serviceUser2)
	createRole(t, iamCreatorRootDB, roleRDSIAM)
	dbExec(t, iamCreatorRootDB, "GRANT CONNECT ON DATABASE %s TO %s", serviceUser1, roleRDSIAM)
	dbExec(t, iamCreatorRootDB, "GRANT CONNECT ON DATABASE %s TO %s", serviceUser2, roleRDSIAM)

	//
	// test read access to serviceUser1
	//

	// reconnect to start a new session with grants from above database creation
	iamCreatorUserDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorUserDB.Close()
	err = postgres.Role(log, iamCreatorUserDB, developerUser, []string{roleRDSIAM}, []postgres.DatabaseSchema{
		{
			Name:       serviceUser1,
			Schema:     serviceUser1,
			Privileges: postgres.PrivilegeRead,
		},
	})
	if !assert.NoError(t, err, "unexpected output error") {
		return
	}

	userDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     developerUser,
		Database: serviceUser1,
		Password: "",
	})
	if err != nil {
		t.Fatalf("could not connect with new user: %v", err)
	}
	defer userDB.Close()
	_, err = userDB.Query(fmt.Sprintf("SELECT * FROM %s.films", serviceUser1))
	if err != nil {
		t.Fatalf("could not select from %s.films table: %v", serviceUser1, err)
	}
	// this should not work as we only requested read rights
	_, err = userDB.Query(fmt.Sprintf("INSERT INTO %s.films VALUES('new title')", serviceUser1))
	if err == nil {
		t.Fatalf("could insert into %s.films table when it should not", serviceUser1)
	}

	//
	// test read and write access to serviceUser2
	//

	// reconnect to start a new session with grants from above database creation
	iamCreatorUserDB, err = postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     "iam_creator",
		Database: serviceUser2,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
	defer iamCreatorUserDB.Close()
	err = postgres.Role(log, iamCreatorUserDB, developerUser, nil, []postgres.DatabaseSchema{
		{
			Name:       serviceUser2,
			Schema:     serviceUser2,
			Privileges: postgres.PrivilegeRead,
		},
		{
			Name:       serviceUser2,
			Schema:     serviceUser2,
			Privileges: postgres.PrivilegeWrite,
		},
	})
	if !assert.NoError(t, err, "unexpected output error") {
		return
	}

	userDB, err = postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		User:     developerUser,
		Database: serviceUser2,
		Password: "",
	})
	if err != nil {
		t.Fatalf("could not connect with new user: %v", err)
	}
	defer userDB.Close()
	_, err = userDB.Query(fmt.Sprintf("SELECT * FROM %s.films", serviceUser2))
	if err != nil {
		t.Fatalf("could not select from %s.films table: %v", serviceUser2, err)
	}
	_, err = userDB.Query(fmt.Sprintf("INSERT INTO %s.films VALUES('new title')", serviceUser2))
	if err != nil {
		t.Fatalf("could not insert into %s.films table: %v", serviceUser2, err)
	}
}

func createServiceDatabase(t *testing.T, log logr.Logger, database *sql.DB, host, service string) {
	t.Helper()
	err := postgres.Database(log, database, host, postgres.Credentials{
		Name:     service,
		User:     service,
		Password: "1234",
	})
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	serviceUserDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		User:     service,
		Database: service,
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to service user failed: %v", err)
	}
	defer serviceUserDB.Close()
	dbExec(t, serviceUserDB, `CREATE TABLE IF NOT EXISTS %s.films (title varchar(40) NOT NULL)`, service)
	dbExec(t, serviceUserDB, `CREATE TABLE IF NOT EXISTS public.public_films (title varchar(40) NOT NULL)`)
}

func createRole(t *testing.T, db *sql.DB, userName string) {
	t.Helper()
	dbExec(t, db, "CREATE ROLE %s WITH LOGIN", userName)
}

func seedRole(t *testing.T, db *sql.DB, userName string, roles []string) {
	t.Helper()
	dbExec(t, db, "GRANT %s TO %s", strings.Join(roles, ", "), userName)
}

// storedRoles returns roles for a specific user name sorted by name.
func storedRoles(t *testing.T, db *sql.DB, userName string) []string {
	t.Helper()

	rows, err := db.Query("SELECT rolname FROM pg_user JOIN pg_auth_members ON (pg_user.usesysid=pg_auth_members.member) JOIN pg_roles ON (pg_roles.oid=pg_auth_members.roleid) WHERE pg_user.usename=$1", userName)
	if err != nil {
		t.Fatalf("get roles for user query failed: %v", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var rolName string
		err = rows.Scan(&rolName)
		if err != nil {
			t.Fatalf("scan row for user query failed: %v", err)
		}
		roles = append(roles, rolName)
	}
	err = rows.Err()
	if err != nil {
		t.Fatalf("scanning rows for user query failed: %v", err)
	}
	sort.Strings(roles)
	return roles
}

func dbExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	query = fmt.Sprintf(query, args...)
	_, err := db.Exec(query)
	if err != nil {
		t.Fatalf("DB EXEC failed: Query: %s: %v", query, err)
	}
}

func dbQuery(t *testing.T, db *sql.DB, query string, args ...interface{}) []string {
	t.Helper()
	query = fmt.Sprintf(query, args...)
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("DB QUERY failed: Query: %s: %v", query, err)
		return nil
	}
	return stringsResult(t, rows)
}
