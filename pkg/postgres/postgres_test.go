package postgres_test

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/controller/postgresqldatabase"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

func TestRole_staticRoles(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	connectionString := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
	db, err := postgres.Connect(log, connectionString)
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}
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
		dropRole(t, db, role)
		_, err = db.Exec(fmt.Sprintf("CREATE ROLE %s", role))
		if err != nil {
			t.Fatalf("Seeding role %s failed: %v", role, err)
		}
	}
	defer func() {
		for _, role := range roles {
			dropRole(t, db, role)
		}
	}()
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
			defer dropRole(t, db, userName)

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

func TestRole_priviliges(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)

	iamCreatorRootDatabase := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
	iamCreatorRootDB, err := postgres.Connect(log, iamCreatorRootDatabase)
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
	iamCreatorUserDB, err := postgres.Connect(log, fmt.Sprintf("postgresql://iam_creator:@%s/%s?sslmode=disable", postgresqlHost, serviceUser1))
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

	userDB, err := postgres.Connect(log, fmt.Sprintf("postgresql://%s:@%s/%s?sslmode=disable", developerUser, postgresqlHost, serviceUser1))
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
	iamCreatorUserDB, err = postgres.Connect(log, fmt.Sprintf("postgresql://iam_creator:@%s/%s?sslmode=disable", postgresqlHost, serviceUser2))
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

	userDB, err = postgres.Connect(log, fmt.Sprintf("postgresql://%s:@%s/%s?sslmode=disable", developerUser, postgresqlHost, serviceUser2))
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
		t.Fatalf("could not insert into %s.films table when it should not", serviceUser2)
	}
}

func createServiceDatabase(t *testing.T, log logr.Logger, database *sql.DB, host, service string) {
	databaseController := postgresqldatabase.ReconcilePostgreSQLDatabase{
		DB: database,
	}
	err := databaseController.EnsurePostgreSQLDatabase(log, service, "")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	serviceUserDatabase := fmt.Sprintf("postgresql://%s:@%s/%s?sslmode=disable", service, host, service)
	serviceUserDB, err := postgres.Connect(log, serviceUserDatabase)
	if err != nil {
		t.Fatalf("connect to service user failed: %v", err)
	}
	defer serviceUserDB.Close()
	dbExec(t, serviceUserDB, `CREATE TABLE IF NOT EXISTS %s.films (title varchar(40) NOT NULL)`, service)
}

func createRole(t *testing.T, db *sql.DB, userName string) {
	t.Helper()
	query := fmt.Sprintf("CREATE ROLE %s WITH LOGIN", userName)
	_, err := db.Exec(query)
	if err != nil {
		t.Fatalf("create existing user failed: %v", err)
	}
}

func seedRole(t *testing.T, db *sql.DB, userName string, roles []string) {
	t.Helper()
	query := fmt.Sprintf("GRANT %s TO %s", strings.Join(roles, ", "), userName)
	_, err := db.Exec(query)
	if err != nil {
		t.Fatalf("create existing user failed: %v", err)
	}
}

func dropRole(t *testing.T, db *sql.DB, userName string) {
	t.Helper()
	query := fmt.Sprintf("DROP ROLE IF EXISTS %s;", userName)
	_, err := db.Exec(query)
	if err != nil {
		t.Fatalf("drop user failed: %v", err)
	}
}

// storedRoles returns roles for a specific user name sorted by name.
func storedRoles(t *testing.T, db *sql.DB, userName string) []string {
	t.Helper()

	rows, err := db.Query("SELECT rolname FROM pg_user JOIN pg_auth_members ON (pg_user.usesysid=pg_auth_members.member) JOIN pg_roles ON (pg_roles.oid=pg_auth_members.roleid) WHERE pg_user.usename=$1", fmt.Sprintf("%s", userName))
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
