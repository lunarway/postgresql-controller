package postgresqluser

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcile_ensurePostgreSQLRole(t *testing.T) {
	postgresqlHost := os.Getenv("POSTGRESQL_CONTROLLER_INTEGRATION_HOST")
	if postgresqlHost == "" {
		t.Skip("Integration test host not specified")
	}
	connectionString := fmt.Sprintf("postgresql://iam_creator:@%s?sslmode=disable", postgresqlHost)
	db, err := postgresqlConnection(connectionString)
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
			log := test.SetLogger(t)

			userName := fmt.Sprintf("test_user_%d", time.Now().UnixNano())
			t.Logf("Using user name %s", userName)

			if tc.createRole {
				createRole(t, db, userName)
			}
			defer dropRole(t, db, userName)

			if len(tc.existingRoles) != 0 {
				seedRole(t, db, userName, tc.existingRoles)
			}

			r := ReconcilePostgreSQLUser{
				db: db,
				grantRoles: []string{
					RoleRDSIAM,
					RoleIAMDeveloper,
				},
			}

			// act
			err = r.ensurePostgreSQLRole(log, userName)

			// assert
			assert.NoError(t, err, "unexpected output error")

			roles := storedRoles(t, db, userName)
			t.Logf("Stored roles: %v", roles)
			assert.Equal(t, tc.roles, roles, "roles on user not as expected")
		})
	}
}

var _ io.Writer = &testLogger{}

// testLogger is an io.Writer used for reporting logs to the test runner.
type testLogger struct {
	t *testing.T
}

func (t *testLogger) Write(p []byte) (int, error) {
	t.t.Logf("%s", p)
	return len(p), nil
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

func TestReconcile_connectToHosts(t *testing.T) {
	test.Integration(t)
	tt := []struct {
		name            string
		credentials     map[string]Credentials
		hostAccess      HostAccess
		connectionCount int
		err             error
	}{
		{
			name: "single host with credentials",
			credentials: map[string]Credentials{
				"localhost:5432": Credentials{
					Name:     "iam_creator",
					Password: "",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             nil,
		},
		{
			name: "multiple hosts with credentials",
			credentials: map[string]Credentials{
				"localhost:5432": Credentials{
					Name:     "iam_creator",
					Password: "",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             nil,
		},
		{
			name: "multiple hosts without upstream",
			credentials: map[string]Credentials{
				"localhost:5432": Credentials{
					Name:     "iam_creator",
					Password: "",
				},
				"unknown": Credentials{
					Name:     "iam_creator",
					Password: "12345678",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{},
				"unknown":        []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             fmt.Errorf("connect to postgresql://iam_creator:***@unknown?sslmode=disable: dial tcp: lookup unknown: no such host"),
		},
		{
			name:        "missing credentials",
			credentials: map[string]Credentials{},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{},
			},
			connectionCount: 0,
			err:             fmt.Errorf("no credentials for host 'localhost:5432'"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_ = test.SetLogger(t)

			r := ReconcilePostgreSQLUser{
				hostCredentials: tc.credentials,
			}

			// act
			connections, err := r.connectToHosts(tc.hostAccess)

			defer func() {
				for _, db := range connections {
					db.Close()
				}
			}()

			// assert
			t.Logf("Connections: %v", connections)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "error not as expected")
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Len(t, connections, tc.connectionCount, "connection count not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses(t *testing.T) {
	accessSpec := func(host, reason string) lunarwayv1alpha1.AccessSpec {
		return lunarwayv1alpha1.AccessSpec{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: host,
			},
			Reason: reason,
		}
	}

	tt := []struct {
		name   string
		reads  []lunarwayv1alpha1.AccessSpec
		writes []lunarwayv1alpha1.AccessSpec
		output HostAccess
	}{
		{
			name:   "no reads",
			reads:  nil,
			output: nil,
		},
		{
			name: "single read and single host",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("localhost:5432", "I'am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"localhost:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("localhost:5432", "I'am a developer"),
					},
				},
			},
		},
		{
			name: "multiple reads and single host",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("localhost:5432", "I'am a developer"),
				accessSpec("localhost:5432", "I really am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"localhost:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("localhost:5432", "I'am a developer"),
					},
					{
						Type:   AccessTypeRead,
						Access: accessSpec("localhost:5432", "I really am a developer"),
					},
				},
			},
		},
		{
			name: "multiple reads and multiple hosts",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I'am a developer"),
				accessSpec("host2:5432", "I really am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"host1:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("host1:5432", "I'am a developer"),
					},
				},
				"host2:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("host2:5432", "I really am a developer"),
					},
				},
			},
		},
		{
			name: "multiple reads and writes and multiple hosts",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I'am a developer"),
				accessSpec("host2:5432", "I really am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I'am a writing developer"),
				accessSpec("host2:5432", "I really am a writing developer"),
			},
			output: HostAccess{
				"host1:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("host1:5432", "I'am a developer"),
					},
					{
						Type:   AccessTypeWrite,
						Access: accessSpec("host1:5432", "I'am a writing developer"),
					},
				},
				"host2:5432": []ReadWriteAccess{
					{
						Type:   AccessTypeRead,
						Access: accessSpec("host2:5432", "I really am a developer"),
					},
					{
						Type:   AccessTypeWrite,
						Access: accessSpec("host2:5432", "I really am a writing developer"),
					},
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r := ReconcilePostgreSQLUser{
				resourceResolver: func(client client.Client, r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
			}

			output, err := r.groupAccesses("namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses_errors(t *testing.T) {
	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: "host1:5432",
			},
			Reason: "I'am a developer",
		},
		{
			Host: lunarwayv1alpha1.ResourceVar{
				ValueFrom: &lunarwayv1alpha1.ResourceVarSource{
					SecretKeyRef: &lunarwayv1alpha1.KeySelector{
						Name: "secret",
						Key:  "key",
					},
				},
			},
			Reason: "I really am a developer",
		},
		{
			Host: lunarwayv1alpha1.ResourceVar{
				ValueFrom: &lunarwayv1alpha1.ResourceVarSource{
					ConfigMapKeyRef: &lunarwayv1alpha1.KeySelector{
						Name: "configmap",
						Key:  "key",
					},
				},
			},
			Reason: "I'm not a developer",
		},
	}
	expectedError := "access to host host1:5432: no value; access to host from secret 'secret' key 'key': no value; access to host from config map 'configmap' key 'key': no value"

	r := ReconcilePostgreSQLUser{
		resourceResolver: func(client client.Client, r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, fmt.Errorf("no value")
		},
	}
	output, err := r.groupAccesses("namespace", reads, nil)

	assert.EqualError(t, err, expectedError, "output error not as exepcted")
	assert.Equal(t, HostAccess(nil), output, "output map not as expected")
}
