package grants_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

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

	access := func(host, database string, privilige postgres.Privilege, reason string) grants.ReadWriteAccess {
		return grants.ReadWriteAccess{
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
		output grants.HostAccess
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
			output: grants.HostAccess{
				"localhost:5432/database": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"localhost:5432/database": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database": []grants.ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database": []grants.ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database", postgres.PrivilegeWrite, "I'am a writing developer"),
				},
				"host2:5432/database": []grants.ReadWriteAccess{
					access("host2:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
					access("host2:5432", "database", postgres.PrivilegeWrite, "I really am a writing developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := grants.Granter{
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					t.Fatalf("allDatabases was not expected to be used")
					return nil, nil
				},
			}

			output, err := r.GroupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses_withAllDatabases(t *testing.T) {
	database := func(host, name string) lunarwayv1alpha1.PostgreSQLDatabase {
		return lunarwayv1alpha1.PostgreSQLDatabase{
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: "user",
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
	access := func(host, database string, privilige postgres.Privilege, reason string) grants.ReadWriteAccess {
		return grants.ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     "user",
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
		output    grants.HostAccess
	}{
		{
			name:      "no databases on host",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: nil,
		},
		{
			name: "single allDatabases read",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database"),
			},
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: grants.HostAccess{
				"host1:5432/database": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database1": []grants.ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database1", postgres.PrivilegeWrite, "I am a writing developer"),
				},
				"host1:5432/database2": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database1": []grants.ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database2": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database1": []grants.ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host1:5432/database2": []grants.ReadWriteAccess{
					access("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432/database3": []grants.ReadWriteAccess{
					access("host2:5432", "database3", postgres.PrivilegeWrite, "I am a writing developer"),
				},
				"host2:5432/database4": []grants.ReadWriteAccess{
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
			output: grants.HostAccess{
				"host1:5432/database1": []grants.ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := grants.Granter{
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  true,
				AllDatabasesWriteEnabled: true,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.GroupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestReconcilePostgreSQLUser_groupAccesses_allDatabasesFeatureFlags(t *testing.T) {
	database := func(host, name string) lunarwayv1alpha1.PostgreSQLDatabase {
		return lunarwayv1alpha1.PostgreSQLDatabase{
			Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
				Name: name,
				Host: lunarwayv1alpha1.ResourceVar{
					Value: host,
				},
				User: lunarwayv1alpha1.ResourceVar{
					Value: "user",
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
	access := func(host, database string, privilige postgres.Privilege, reason string) grants.ReadWriteAccess {
		return grants.ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     "user",
				Privileges: privilige,
			},
			Access: spec(host, reason),
		}
	}
	tt := []struct {
		name                string
		databases           []lunarwayv1alpha1.PostgreSQLDatabase
		readFeatureEnabled  bool
		writeFeatureEnabled bool
		reads               []lunarwayv1alpha1.AccessSpec
		writes              []lunarwayv1alpha1.AccessSpec
		output              grants.HostAccess
	}{
		{
			name: "read allDatabases disabled",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database"),
			},
			readFeatureEnabled:  false,
			writeFeatureEnabled: true,
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: nil,
		},
		{
			name: "write allDatabases disabled",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database"),
			},
			readFeatureEnabled:  true,
			writeFeatureEnabled: false,
			reads:               nil,
			writes: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			output: nil,
		},
		{
			name: "write allDatabases disabled with read request",
			databases: []lunarwayv1alpha1.PostgreSQLDatabase{
				database("host1:5432", "database"),
			},
			readFeatureEnabled:  true,
			writeFeatureEnabled: false,
			reads: []lunarwayv1alpha1.AccessSpec{
				spec("host1:5432", "I am a developer"),
			},
			writes: nil,
			output: grants.HostAccess{
				"host1:5432/database": []grants.ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := grants.Granter{
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  tc.readFeatureEnabled,
				AllDatabasesWriteEnabled: tc.writeFeatureEnabled,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.GroupAccesses(logger, "namespace", tc.reads, tc.writes)

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
				User: lunarwayv1alpha1.ResourceVar{
					Value: "user",
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
				Value: "user",
			},
			Reason: reason,
		}
	}
	allDatabasesAccess := func(host, database string, privilige postgres.Privilege, reason string) grants.ReadWriteAccess {
		return grants.ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     "user",
				Privileges: privilige,
			},
			Access: allDatabasesSpec(host, reason),
		}
	}
	singleDatabaseAccess := func(host, database string, privilige postgres.Privilege, reason string) grants.ReadWriteAccess {
		return grants.ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     "user",
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
		output    grants.HostAccess
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
			output: grants.HostAccess{
				"host1:5432/database1": []grants.ReadWriteAccess{
					allDatabasesAccess("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host1:5432/database2": []grants.ReadWriteAccess{
					allDatabasesAccess("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
					singleDatabaseAccess("host1:5432", "database2", postgres.PrivilegeWrite, "I am a writing developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := grants.Granter{
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  true,
				AllDatabasesWriteEnabled: true,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.GroupAccesses(logger, "namespace", tc.reads, tc.writes)

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
	expectedError := "resolve host: access to host host1:5432: no value; resolve host: access to host from secret 'secret' key 'key': no value; resolve host: access to host from config map 'configmap' key 'key': no value"

	logger := test.NewLogger(t)
	r := grants.Granter{
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, fmt.Errorf("no value")
		},
	}
	output, err := r.GroupAccesses(logger, "namespace", reads, nil)

	assert.EqualError(t, err, expectedError, "output error not as exepcted")
	assert.Equal(t, grants.HostAccess(nil), output, "output map not as expected")
}

// TestGranter_GroupAccesses_noUserSchemaFallback_allDatabases tests that the
// database name is used as a fallback if no user is specified in the access
// spec and the spec has AllDatabases set.
func TestGranter_GroupAccesses_noUserSchemaFallback_allDatabases(t *testing.T) {
	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: "host1:5432",
			},
			AllDatabases: true,
			Reason:       "I am a developer",
		},
	}
	expectedHostAccesses := grants.HostAccess{
		"host1:5432/db1": []grants.ReadWriteAccess{
			grants.ReadWriteAccess{
				Host: "host1:5432",
				Access: lunarwayv1alpha1.AccessSpec{
					Host: lunarwayv1alpha1.ResourceVar{
						Value: "host1:5432",
					},
					AllDatabases: true,
					Reason:       "I am a developer",
				},
				Database: postgres.DatabaseSchema{
					Name:       "db1",
					Schema:     "db1", // this is the important part of the expectation
					Privileges: postgres.PrivilegeRead,
				},
			},
		},
	}

	logger := test.NewLogger(t)
	r := grants.Granter{
		AllDatabasesReadEnabled: true,
		AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
			return []lunarwayv1alpha1.PostgreSQLDatabase{
				lunarwayv1alpha1.PostgreSQLDatabase{
					Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
						Name: "db1",
						User: lunarwayv1alpha1.ResourceVar{
							// will resolve to a kube.ErrNoValue error in the resource
							// resolver
							Value: "user",
						},
						Host: lunarwayv1alpha1.ResourceVar{
							Value: "host1:5432",
						},
					},
				},
			}, nil
		},
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			// fake that the user lookup has no value
			if r.Value == "user" {
				return "", kube.ErrNoValue
			}
			return r.Value, nil
		},
	}
	output, err := r.GroupAccesses(logger, "namespace", reads, nil)

	assert.NoError(t, err, "unexpected output error")
	assert.Equal(t, expectedHostAccesses, output, "output map not as expected")
}
