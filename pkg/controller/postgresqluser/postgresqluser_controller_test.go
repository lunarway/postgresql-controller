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
				"localhost:5432": []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             nil,
		},
		{
			name: "multiple hosts with credentials",
			credentials: map[string]postgres.Credentials{
				"localhost:5432": postgres.Credentials{
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
				"localhost:5432": []ReadWriteAccess{},
				"unknown":        []ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             fmt.Errorf("connect to postgresql://iam_creator:***@unknown?sslmode=disable: dial tcp: lookup unknown: no such host"),
		},
		{
			name:        "missing credentials",
			credentials: map[string]postgres.Credentials{},
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
				accessSpec("localhost:5432", "I'am a developer"),
			},
			writes: nil,
			output: HostAccess{
				"localhost:5432/database": []ReadWriteAccess{
					access("localhost:5432", "database", postgres.PrivilegeRead, "I'am a developer"),
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
				"localhost:5432/database": []ReadWriteAccess{
					access("localhost:5432", "database", postgres.PrivilegeRead, "I'am a developer"),
					access("localhost:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
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
				"host1:5432/database": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I'am a developer"),
				},
				"host2:5432/database": []ReadWriteAccess{
					access("host2:5432", "database", postgres.PrivilegeRead, "I really am a developer"),
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
				"host1:5432/database": []ReadWriteAccess{
					access("host1:5432", "database", postgres.PrivilegeRead, "I'am a developer"),
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
