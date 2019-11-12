package postgresqldatabase

// func TestReconcile_ensurePostgreSQLRole(t *testing.T) {
// 	postgresqlHost := os.Getenv("POSTGRESQL_CONTROLLER_INTEGRATION_HOST")
// 	if postgresqlHost == "" {
// 		t.Skip("Integration test host not specified")
// 	}
// 	connectionString := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
// 	db, err := postgresqlConnection(connectionString)
// 	if err != nil {
// 		t.Fatalf("connect to database failed: %v", err)
// 	}
// 	var (
// 		epoch            = time.Now().UnixNano()
// 		RoleRDSIAM       = fmt.Sprintf("rds_iam_%d", epoch)
// 		RoleIAMDeveloper = fmt.Sprintf("iam_developer_%d", epoch)
// 		RoleOther        = fmt.Sprintf("other_role_%d", epoch)
// 	)
// 	// roles used for testing
// 	roles := []string{
// 		RoleRDSIAM,
// 		RoleIAMDeveloper,
// 		RoleOther,
// 	}
// 	// bootstrap the database with the roles that can be granted by the controller
// 	for _, role := range roles {
// 		dropRole(t, db, role)
// 		_, err = db.Exec(fmt.Sprintf("CREATE ROLE %s", role))
// 		if err != nil {
// 			t.Fatalf("Seeding role %s failed: %v", role, err)
// 		}
// 	}
// 	defer func() {
// 		for _, role := range roles {
// 			dropRole(t, db, role)
// 		}
// 	}()
// 	tt := []struct {
// 		name          string
// 		createRole    bool
// 		existingRoles []string
// 		roles         []string
// 	}{
// 		{
// 			name:          "new user without any roles",
// 			createRole:    false,
// 			existingRoles: nil,
// 			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
// 		},
// 		{
// 			name:          "existing user without any roles",
// 			createRole:    true,
// 			existingRoles: nil,
// 			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
// 		},
// 		{
// 			name:          "user exists with correct roles",
// 			createRole:    true,
// 			existingRoles: []string{RoleIAMDeveloper, RoleRDSIAM},
// 			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
// 		},
// 		{
// 			name:          "user exists with incomplete roles",
// 			createRole:    true,
// 			existingRoles: []string{RoleRDSIAM},
// 			roles:         []string{RoleIAMDeveloper, RoleRDSIAM},
// 		},
// 		{
// 			name:          "user exists with other roles",
// 			createRole:    true,
// 			existingRoles: []string{RoleOther},
// 			roles:         []string{RoleIAMDeveloper, RoleOther, RoleRDSIAM},
// 		},
// 	}
// 	for _, tc := range tt {
// 		t.Run(tc.name, func(t *testing.T) {
// 			logger := testLogger{t: t}
// 			logf.SetLogger(logf.ZapLoggerTo(&logger, true))

// 			userName := fmt.Sprintf("test_user_%d", time.Now().UnixNano())
// 			t.Logf("Using user name %s", userName)

// 			if tc.createRole {
// 				createRole(t, db, userName)
// 			}
// 			defer dropRole(t, db, userName)

// 			if len(tc.existingRoles) != 0 {
// 				seedRole(t, db, userName, tc.existingRoles)
// 			}

// 			r := ReconcilePostgreSQLUser{
// 				db: db,
// 				grantRoles: []string{
// 					RoleRDSIAM,
// 					RoleIAMDeveloper,
// 				},
// 			}

// 			// act
// 			err = r.ensurePostgreSQLRole(logf.Log, userName)

// 			// assert
// 			assert.NoError(t, err, "unexpected output error")

// 			roles := storedRoles(t, db, userName)
// 			t.Logf("Stored roles: %v", roles)
// 			assert.Equal(t, tc.roles, roles, "roles on user not as expected")
// 		})
// 	}
// }

// var _ io.Writer = &testLogger{}

// // testLogger is an io.Writer used for reporting logs to the test runner.
// type testLogger struct {
// 	t *testing.T
// }

// func (t *testLogger) Write(p []byte) (int, error) {
// 	t.t.Logf("%s", p)
// 	return len(p), nil
// }

// func createRole(t *testing.T, db *sql.DB, userName string) {
// 	t.Helper()
// 	query := fmt.Sprintf("CREATE ROLE %s WITH LOGIN", userName)
// 	_, err := db.Exec(query)
// 	if err != nil {
// 		t.Fatalf("create existing user failed: %v", err)
// 	}
// }

// func seedRole(t *testing.T, db *sql.DB, userName string, roles []string) {
// 	t.Helper()
// 	query := fmt.Sprintf("GRANT %s TO %s", strings.Join(roles, ", "), userName)
// 	_, err := db.Exec(query)
// 	if err != nil {
// 		t.Fatalf("create existing user failed: %v", err)
// 	}
// }

// func dropRole(t *testing.T, db *sql.DB, userName string) {
// 	t.Helper()
// 	query := fmt.Sprintf("DROP ROLE IF EXISTS %s;", userName)
// 	_, err := db.Exec(query)
// 	if err != nil {
// 		t.Fatalf("drop user failed: %v", err)
// 	}
// }

// // storedRoles returns roles for a specific user name sorted by name.
// func storedRoles(t *testing.T, db *sql.DB, userName string) []string {
// 	t.Helper()

// 	rows, err := db.Query("SELECT rolname FROM pg_user JOIN pg_auth_members ON (pg_user.usesysid=pg_auth_members.member) JOIN pg_roles ON (pg_roles.oid=pg_auth_members.roleid) WHERE pg_user.usename=$1", fmt.Sprintf("%s", userName))
// 	if err != nil {
// 		t.Fatalf("get roles for user query failed: %v", err)
// 	}
// 	defer rows.Close()
// 	var roles []string
// 	for rows.Next() {
// 		var rolName string
// 		err = rows.Scan(&rolName)
// 		if err != nil {
// 			t.Fatalf("scan row for user query failed: %v", err)
// 		}
// 		roles = append(roles, rolName)
// 	}
// 	err = rows.Err()
// 	if err != nil {
// 		t.Fatalf("scanning rows for user query failed: %v", err)
// 	}
// 	sort.Strings(roles)
// 	return roles
// }
