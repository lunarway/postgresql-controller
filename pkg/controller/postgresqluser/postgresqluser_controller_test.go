package postgresqluser

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

func TestReconcile_connectToHosts(t *testing.T) {
	test.Integration(t)
	tt := []struct {
		name            string
		credentials     map[string]postgres.Credentials
		hostAccess      grants.HostAccess
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
			hostAccess: grants.HostAccess{
				"localhost:5432/postgres": []grants.ReadWriteAccess{},
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
			hostAccess: grants.HostAccess{
				"localhost:5432/postgres": []grants.ReadWriteAccess{},
				"unknown/postgres":        []grants.ReadWriteAccess{},
			},
			connectionCount: 1,
			err:             fmt.Errorf("connect to postgresql://iam_creator:********@unknown/postgres?sslmode=disable: dial tcp:"),
		},
		{
			name:        "missing credentials",
			credentials: map[string]postgres.Credentials{},
			hostAccess: grants.HostAccess{
				"localhost:5432/postgres": []grants.ReadWriteAccess{},
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
