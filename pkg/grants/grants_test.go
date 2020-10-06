package grants

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGranter_groupAccesses(t *testing.T) {
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
				"localhost:5432": []ReadWriteAccess{
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
				"localhost:5432": []ReadWriteAccess{
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432": []ReadWriteAccess{
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database", postgres.PrivilegeWrite, "I'am a writing developer"),
				},
				"host2:5432": []ReadWriteAccess{
					access("host2:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
					access("host2:5432", "database", postgres.PrivilegeWrite, "I really am a writing developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now: time.Now,
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					t.Fatalf("allDatabases was not expected to be used")
					return nil, nil
				},
			}

			output, err := r.groupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

// TestGranter_groupAccesses_startStopHandling tests that groupAccesses filteres
// future and expired access requests correctly.
func TestGranter_groupAccesses_startStopHandling(t *testing.T) {
	var (
		now          = time.Date(2020, 4, 30, 13, 0, 0, 0, time.UTC)
		past1Hour    = now.Add(-1 * time.Hour)
		past2Hours   = now.Add(-2 * time.Hour)
		future1Hour  = now.Add(1 * time.Hour)
		future2Hours = now.Add(2 * time.Hour)

		// dummy values for access instances
		host      = "localhost:5432"
		database  = "database"
		privilige = postgres.PrivilegeRead
		reason    = "A good reason"
	)

	accessSpec := func(start, stop time.Time) lunarwayv1alpha1.AccessSpec {
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
			Start:  v1.NewTime(start),
			Stop:   v1.NewTime(stop),
		}
	}

	access := func(start, stop time.Time) *ReadWriteAccess {
		return &ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     database,
				Privileges: privilige,
			},
			Access: accessSpec(start, stop),
		}
	}
	tt := []struct {
		name   string
		access lunarwayv1alpha1.AccessSpec
		output *ReadWriteAccess
	}{
		{
			name:   "read starting in the future",
			access: accessSpec(future1Hour, future2Hours),
			output: nil,
		},
		{
			name:   "read started and ends in the future",
			access: accessSpec(past1Hour, future1Hour),
			output: access(past1Hour, future1Hour),
		},
		{
			name:   "read started and ends in the past",
			access: accessSpec(past2Hours, past1Hour),
			output: nil,
		},
		{
			name:   "empty start time",
			access: accessSpec(time.Time{}, future1Hour),
			output: access(time.Time{}, future1Hour),
		},
		{
			name:   "empty stop time",
			access: accessSpec(past1Hour, time.Time{}),
			output: access(past1Hour, time.Time{}),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now: func() time.Time {
					return now
				},
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					t.Fatalf("allDatabases was not expected to be used")
					return nil, nil
				},
			}

			output, err := r.groupAccesses(logger, "namespace", []lunarwayv1alpha1.AccessSpec{tc.access}, nil)

			assert.NoError(t, err, "unexpected output error")
			var hostAccess HostAccess
			if tc.output != nil {
				hostAccess = HostAccess{
					host: []ReadWriteAccess{
						*tc.output,
					},
				}
			}
			assert.Equal(t, hostAccess, output, "output map not as expected")
		})
	}
}

func TestGranter_groupAccesses_withAllDatabases(t *testing.T) {
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
	access := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
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
		output    HostAccess
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
			output: HostAccess{
				"host1:5432": []ReadWriteAccess{
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database1", postgres.PrivilegeWrite, "I am a writing developer"),
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432": []ReadWriteAccess{
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
					access("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
				},
				"host2:5432": []ReadWriteAccess{
					access("host2:5432", "database3", postgres.PrivilegeWrite, "I am a writing developer"),
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
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now: time.Now,
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  true,
				AllDatabasesWriteEnabled: true,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.groupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestGranter_groupAccesses_allDatabasesFeatureFlags(t *testing.T) {
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
	access := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
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
		output              HostAccess
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
			output: HostAccess{
				"host1:5432": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I am a developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now: time.Now,
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  tc.readFeatureEnabled,
				AllDatabasesWriteEnabled: tc.writeFeatureEnabled,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.groupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestGranter_groupAccesses_mixedSpecs(t *testing.T) {
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
	allDatabasesAccess := func(host, database string, privilige postgres.Privilege, reason string) ReadWriteAccess {
		return ReadWriteAccess{
			Host: host,
			Database: postgres.DatabaseSchema{
				Name:       database,
				Schema:     "user",
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
				"host1:5432": []ReadWriteAccess{
					allDatabasesAccess("host1:5432", "database1", postgres.PrivilegeRead, "I am a developer"),
					allDatabasesAccess("host1:5432", "database2", postgres.PrivilegeRead, "I am a developer"),
					singleDatabaseAccess("host1:5432", "database2", postgres.PrivilegeWrite, "I am a writing developer"),
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now: time.Now,
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					return r.Value, nil
				},
				AllDatabasesReadEnabled:  true,
				AllDatabasesWriteEnabled: true,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return tc.databases, nil
				},
			}

			output, err := r.groupAccesses(logger, "namespace", tc.reads, tc.writes)

			assert.NoError(t, err, "unexpected output error")
			assert.Equal(t, tc.output, output, "output map not as expected")
		})
	}
}

func TestGranter_groupAccesses_errors(t *testing.T) {
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
	r := Granter{
		Now: time.Now,
		ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
			return r.Value, fmt.Errorf("no value")
		},
	}
	output, err := r.groupAccesses(logger, "namespace", reads, nil)

	assert.EqualError(t, err, expectedError, "output error not as exepcted")
	assert.Equal(t, HostAccess(nil), output, "output map not as expected")
}

func TestGranter_connectToHosts(t *testing.T) {
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
				"localhost:5432": {
					Name:     "iam_creator",
					Password: "",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{
					{
						Host: "localhost:5432",
						Database: postgres.DatabaseSchema{
							Name: "postgres",
						},
					},
				},
			},
			connectionCount: 1,
			err:             nil,
		},
		{
			name: "multiple hosts without upstream",
			credentials: map[string]postgres.Credentials{
				"localhost:5432": {
					Name:     "iam_creator",
					Password: "",
				},
				"unknown": {
					Name:     "iam_creator",
					Password: "12345678",
				},
			},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{
					{
						Host: "localhost:5432",
						Database: postgres.DatabaseSchema{
							Name: "postgres",
						},
					}},
				"unknown": []ReadWriteAccess{
					{
						Host: "unknown",
						Database: postgres.DatabaseSchema{
							Name: "postgres",
						},
					}},
			},
			connectionCount: 1,
			err:             fmt.Errorf("connect to postgresql://iam_creator:********@unknown/postgres?sslmode=disable: dial tcp:"),
		},
		{
			name:        "missing credentials",
			credentials: map[string]postgres.Credentials{},
			hostAccess: HostAccess{
				"localhost:5432": []ReadWriteAccess{
					{
						Host: "localhost:5432",
						Database: postgres.DatabaseSchema{
							Name: "postgres",
						},
					},
				},
			},
			connectionCount: 0,
			err:             fmt.Errorf("no credentials for host 'localhost:5432'"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)

			r := Granter{
				Now:             time.Now,
				HostCredentials: tc.credentials,
			}

			// act
			connections, err := r.connectToHosts(logger, tc.hostAccess)

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

// TestGranter_groupAccesses_partialErrors tests that GroupAccesses returns a
// partial result in case of errors, eg. database2 cannot be resolved but
// database1 can.
func TestGranter_groupAccesses_partialErrors(t *testing.T) {
	tt := []struct {
		name            string
		reads           []lunarwayv1alpha1.AccessSpec
		resourceResults map[string]struct {
			value string
			err   error
		}
		hosts HostAccess
		err   error
	}{
		{
			name: "all good results",
			reads: []lunarwayv1alpha1.AccessSpec{
				{
					Host: lunarwayv1alpha1.ResourceVar{
						Value: "host1",
					},
					AllDatabases: true,
				},
			},
			hosts: HostAccess{
				"host1": []ReadWriteAccess{
					{
						Host: "host1",
						Database: postgres.DatabaseSchema{
							Name:   "database1",
							Schema: "database1",
						},
						Access: lunarwayv1alpha1.AccessSpec{
							Host: lunarwayv1alpha1.ResourceVar{
								Value: "host1",
							},
							AllDatabases: true,
						},
					},
				},
			},
			err: errors.New("all databases: access to host host1: resolve database 'database2' host name: no value"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			logger := test.NewLogger(t)
			r := Granter{
				Now:                     time.Now,
				AllDatabasesReadEnabled: true,
				AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
					return []lunarwayv1alpha1.PostgreSQLDatabase{
						{
							Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
								Name: "database1",
								Host: lunarwayv1alpha1.ResourceVar{
									Value: "host1",
								},
							},
						},
						{
							Spec: lunarwayv1alpha1.PostgreSQLDatabaseSpec{
								Name: "database2",
								Host: lunarwayv1alpha1.ResourceVar{
									Value: "host1",
									// used in test to indicate that this should not be found
									ValueFrom: &lunarwayv1alpha1.ResourceVarSource{},
								},
							},
						},
					}, nil
				},
				ResourceResolver: func(r lunarwayv1alpha1.ResourceVar, ns string) (string, error) {
					if r.ValueFrom != nil {
						return "", kube.ErrNoValue
					}
					return r.Value, nil
				},
			}
			output, err := r.groupAccesses(logger, "namespace", tc.reads, nil)

			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as exepcted")
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.hosts, output, "output map not as expected")
		})
	}
}

// TestGranter_groupAccesses_noUserSchemaFallback_allDatabases tests that the
// database name is used as a fallback if no user is specified in the access
// spec and the spec has AllDatabases set.
func TestGranter_groupAccesses_noUserSchemaFallback_allDatabases(t *testing.T) {
	reads := []lunarwayv1alpha1.AccessSpec{
		{
			Host: lunarwayv1alpha1.ResourceVar{
				Value: "host1:5432",
			},
			AllDatabases: true,
			Reason:       "I am a developer",
		},
	}
	expectedHostAccesses := HostAccess{
		"host1:5432": []ReadWriteAccess{
			{
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
	r := Granter{
		Now:                     time.Now,
		AllDatabasesReadEnabled: true,
		AllDatabases: func(namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
			return []lunarwayv1alpha1.PostgreSQLDatabase{
				{
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
	output, err := r.groupAccesses(logger, "namespace", reads, nil)

	assert.NoError(t, err, "unexpected output error")
	assert.Equal(t, expectedHostAccesses, output, "output map not as expected")
}
