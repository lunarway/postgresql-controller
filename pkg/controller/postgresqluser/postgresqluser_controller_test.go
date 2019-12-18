package postgresqluser

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcile_connectToHosts(t *testing.T) {
	test.Integration(t)
	tt := []struct {
		name            string
		credentials     map[string]postgres.Credentials
		hostAccess      HostAccess
		connectionCount int
		err             error
	}{
		{
			name: "single host with credentials",
			credentials: map[string]postgres.Credentials{
				"localhost:5432": postgres.Credentials{
					Name:     "iam_creator",
					Password: "",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432/postgres": []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             nil,
		},
		{
			name: "multiple hosts without upstream",
			credentials: map[string]postgres.Credentials{
				"localhost:5432": postgres.Credentials{
					Name:     "iam_creator",
					Password: "",
				},
				"unknown": postgres.Credentials{
					Name:     "iam_creator",
					Password: "12345678",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432/postgres": []ReadWriteAccess{},
				"unknown/postgres":        []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             fmt.Errorf("connect to postgresql://iam_creator:********@unknown/postgres?sslmode=disable: dial tcp:"),
		},
		{
			name:        "missing credentials",
			credentials: map[string]postgres.Credentials{},
			hostAccess: HostAccess{
				"localhost:5432/postgres": []ReadWriteAccess{},
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
				if !assert.Error(t, err, "an output error was expected") {
					return
				}
				assert.Contains(t, err.Error(), tc.err.Error(), "error not as expected")
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
			Database: lunarwayv1alpha1.ResourceVar{
				Value: "database",
			},
			Schema: lunarwayv1alpha1.ResourceVar{
				Value: "database",
			},
			Reason: reason,
		}
	}

	access := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     database,
				Privileges: privilige,
			},
			Access: accessSpec(host, reason),
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
				accessSpec("localhost:5432", "I am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"localhost:5432/database": []ReadWriteAccess{
					access("localhost:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
		{
			name: "multiple reads and single host",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("localhost:5432", "I am a developer"),
				accessSpec("localhost:5432", "I really am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"localhost:5432/database": []ReadWriteAccess{
					access("localhost:5432", "database", postgres.PrivilegeRead, "I am a developer"),
					access("localhost:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
				},
			},
		},
		{
			name: "multiple reads and multiple hosts",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I am a developer"),
				accessSpec("host2:5432", "I really am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"host1:5432/database": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database": []ReadWriteAccess{
					access("host2:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
				},
			},
		},
		{
			name: "multiple reads and writes and multiple hosts",
			reads: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I am a developer"),
				accessSpec("host2:5432", "I really am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				accessSpec("host1:5432", "I'am a writing developer"),
				accessSpec("host2:5432", "I really am a writing developer"),
			},
			output: HostAccess{
				"host1:5432/database": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database", postgres.PrivilegeWrite, "I'am a writing developer"),
				},
				"host2:5432/database": []ReadWriteAccess{
					access("host2:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
					access("host2:5432", "database", postgres.PrivilegeWrite, "I really am a writing developer"),
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
				allDatabases: func(client client.Client, namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					t.Fatalf("allDatabases was not expected to be used")
					return nil, nil
				},
			}

			output, err := r.groupAccesses("namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses_WithAllDatabases(t *testing.T) {
	database := func(host, name string) lunarwayv1alpha1.PostgreSQLDatabase {
		return lunarwayv1alpha1.PostgreSQLDatabase{
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
			},
		}
	}
	spec := func(host, reason string) lunarwayv1alpha1.AccessSpec {
		return lunarwayv1alpha1.AccessSpec{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: host,
			},
			AllDatabases: true,
			Reason:       reason,
		}
	}
	access := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     database,
				Privileges: privilige,
			},
			Access: spec(host, reason),
		}
	}
	tt := []struct {
		name      string
		databases []lunarwayv1alpha1.PostgreSQLDatabase
		reads     []lunarwayv1alpha1.AccessSpec
		writes    []lunarwayv1alpha1.AccessSpec
		output    HostAccess
	}{
		{
			name: "single allDatabases read",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"host1:5432/database": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
		{
			name: "multiple allDatabases read and write on same host",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database1"),
				database("host1:5432", "database2"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a writing developer"),
			},
			output: HostAccess{
				"host1:5432/database1": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database1", postgres.PrivilegeWrite, "I am a writing developer"),
				},
				"host1:5432/database2": []ReadWriteAccess{
					access("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database2", postgres.PrivilegeWrite, "I am a writing developer"),
				},
			},
		},
		{
			name: "multiple allDatabases read and write on different hosts",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database1"),
				database("host2:5432", "database2"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				spec("host2:5432", "I am a writing developer"),
			},
			output: HostAccess{
				"host1:5432/database1": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database2": []ReadWriteAccess{
					access("host2:5432", "database2", postgres.PrivilegeWrite, "I am a writing developer"),
				},
			},
		},
		{
			name: "multiple allDatabases read and write on different hosts with multiple databases",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database1"),
				database("host1:5432", "database2"),
				database("host2:5432", "database3"),
				database("host2:5432", "database4"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				spec("host2:5432", "I am a writing developer"),
			},
			output: HostAccess{
				"host1:5432/database1": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host1:5432/database2": []ReadWriteAccess{
					access("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database3": []ReadWriteAccess{
					access("host2:5432", "database3", postgres.PrivilegeWrite, "I am a writing developer"),
				},
				"host2:5432/database4": []ReadWriteAccess{
					access("host2:5432", "database4", postgres.PrivilegeWrite, "I am a writing developer"),
				},
			},
		},
		{
			name: "read with allDatabases and unused hosts",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database1"),
				database("host2:5432", "database2"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"host1:5432/database1": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
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
				allDatabases: func(client client.Client, namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.groupAccesses("namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses_mixedSpecs(t *testing.T) {
	database := func(host, name string) lunarwayv1alpha1.PostgreSQLDatabase {
		return lunarwayv1alpha1.PostgreSQLDatabase{
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
			},
		}
	}
	allDatabasesSpec := func(host, reason string) lunarwayv1alpha1.AccessSpec {
		return lunarwayv1alpha1.AccessSpec{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: host,
			},
			AllDatabases: true,
			Reason:       reason,
		}
	}
	singleDatabaseSpec := func(host, database, reason string) lunarwayv1alpha1.AccessSpec {
		return lunarwayv1alpha1.AccessSpec{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: host,
			},
			Database: lunarwayv1alpha1.ResourceVar{
				Value: database,
			},
			Schema: lunarwayv1alpha1.ResourceVar{
				Value: database,
			},
			Reason: reason,
		}
	}
	allDatabasesAccess := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     database,
				Privileges: privilige,
			},
			Access: allDatabasesSpec(host, reason),
		}
	}
	singleDatabaseAccess := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     database,
				Privileges: privilige,
			},
			Access: singleDatabaseSpec(host, database, reason),
		}
	}
	tt := []struct {
		name      string
		databases []lunarwayv1alpha1.PostgreSQLDatabase
		reads     []lunarwayv1alpha1.AccessSpec
		writes    []lunarwayv1alpha1.AccessSpec
		output    HostAccess
	}{
		{
			name: "single write and allDatabases read on same host",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database1"),
				database("host1:5432", "database2"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				allDatabasesSpec("host1:5432", "I am a developer"),
			},
			writes: []lunarwayv1alpha1.AccessSpec{
				singleDatabaseSpec("host1:5432", "database2", "I am a writing developer"),
			},
			output: HostAccess{
				"host1:5432/database1": []ReadWriteAccess{
					allDatabasesAccess("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host1:5432/database2": []ReadWriteAccess{
					allDatabasesAccess("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
					singleDatabaseAccess("host1:5432", "database2", postgres.PrivilegeWrite, "I am a writing developer"),
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
				allDatabases: func(client client.Client, namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
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
			Reason: "I am a developer",
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

func TestParseHostCredentials(t *testing.T) {
	tt := []struct {
		name   string
		input  map[string]string
		output map[string]postgres.Credentials
		err    error
	}{
		{
			name:   "nil map",
			input:  nil,
			output: nil,
		},
		{
			name: "single host",
			input: map[string]string{
				"host:5432": "user:password",
			},
			output: map[string]postgres.Credentials{
				"host:5432": postgres.Credentials{
					Name:     "user",
					Password: "password",
				},
			},
			err: nil,
		},
		{
			name: "multiple hosts",
			input: map[string]string{
				"host1:5432": "user1:password1",
				"host2:5432": "user2:password2",
			},
			output: map[string]postgres.Credentials{
				"host1:5432": postgres.Credentials{
					Name:     "user1",
					Password: "password1",
				},
				"host2:5432": postgres.Credentials{
					Name:     "user2",
					Password: "password2",
				},
			},
			err: nil,
		},
		{
			name: "single host without user or password",
			input: map[string]string{
				"host:5432": "",
			},
			output: nil,
			err:    errors.New("username empty"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output, err := parseHostCredentials(tc.input)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as expected")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}
