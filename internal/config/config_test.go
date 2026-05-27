package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

func TestHostCredentials_Set(t *testing.T) {
	tt := []struct {
		name   string
		value  string
		err    error
		output map[string]postgres.Credentials
	}{
		{
			name:   "empty input",
			value:  "",
			err:    nil,
			output: map[string]postgres.Credentials{},
		},
		{
			name:   "invalid key value pair",
			value:  "host:5432",
			err:    errors.New("host:5432 must be formatted as key=value"),
			output: map[string]postgres.Credentials{},
		},
		{
			name:   "empty host name",
			value:  "=user:pass",
			err:    errors.New("=user:pass must be formatted as key=value"),
			output: map[string]postgres.Credentials{},
		},
		{
			name:  "single host",
			value: "host:5432=user:pass",
			err:   nil,
			output: map[string]postgres.Credentials{
				"host:5432": {
					User:     "user",
					Password: "pass",
				},
			},
		},
		{
			name:  "multiple hosts",
			value: "host1:5432=user1:pass1,host2:5432=user2:pass2",
			err:   nil,
			output: map[string]postgres.Credentials{
				"host1:5432": {
					User:     "user1",
					Password: "pass1",
				},
				"host2:5432": {
					User:     "user2",
					Password: "pass2",
				},
			},
		},
		{
			name:   "single host without user and password",
			value:  "host1:5432=",
			err:    errors.New("parse host 'host1:5432=' failed: username empty"),
			output: map[string]postgres.Credentials{},
		},
		{
			name:  "host with ssl configured",
			value: "host1:5432=user1:pass1=sslmode=enabled",
			err:   nil,
			output: map[string]postgres.Credentials{
				"host1:5432": {
					User:     "user1",
					Password: "pass1",
					Params:   "sslmode=enabled",
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output := make(map[string]postgres.Credentials)
			h := HostCredentials{
				value: &output,
			}

			err := h.Set(tc.value)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "error not as expected")
			} else {
				assert.NoError(t, err, "unexpected error")
			}

			assert.Equal(t, tc.output, output, "parsed output not as expected")
		})
	}
}

func TestHostCredentials_String(t *testing.T) {
	tt := []struct {
		name   string
		value  map[string]postgres.Credentials
		output string
	}{
		{
			name:   "nil input",
			value:  nil,
			output: "[]",
		},
		{
			name:   "empty input",
			value:  map[string]postgres.Credentials{},
			output: "[]",
		},
		{
			name: "single host",
			value: map[string]postgres.Credentials{
				"host:5432": {
					User:     "user",
					Password: "pass",
				},
			},
			output: "[host:5432=user:********]",
		},
		{
			name: "multiple hosts",
			value: map[string]postgres.Credentials{
				"host1:5432": {
					User:     "user1",
					Password: "pass1",
				},
				"host2:5432": {
					User:     "user2",
					Password: "pass2",
				},
			},
			output: "[host1:5432=user1:********,host2:5432=user2:********]",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			h := HostCredentials{
				value: &tc.value,
			}

			output := h.String()

			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}

func TestControllerConfiguration_GetGlobalExtensions(t *testing.T) {
	tt := []struct {
		name   string
		input  string
		output []string
	}{
		{
			name:   "empty string",
			input:  "",
			output: []string{},
		},
		{
			name:   "single extension",
			input:  "pgcrypto",
			output: []string{"pgcrypto"},
		},
		{
			name:   "multiple extensions",
			input:  "pgcrypto,uuid-ossp,pg_stat_statements",
			output: []string{"pgcrypto", "uuid-ossp", "pg_stat_statements"},
		},
		{
			name:   "extensions with whitespace",
			input:  "pgcrypto, uuid-ossp , pg_stat_statements",
			output: []string{"pgcrypto", "uuid-ossp", "pg_stat_statements"},
		},
		{
			name:   "trailing comma",
			input:  "pgcrypto,uuid-ossp,",
			output: []string{"pgcrypto", "uuid-ossp"},
		},
		{
			name:   "empty elements",
			input:  "pgcrypto,,uuid-ossp",
			output: []string{"pgcrypto", "uuid-ossp"},
		},
		{
			name:   "invalid extension with spaces",
			input:  "pgcrypto,some thing with spaces,uuid-ossp",
			output: []string{"pgcrypto", "uuid-ossp"},
		},
		{
			name:   "invalid extension with special characters",
			input:  "pgcrypto,invalid$ext,uuid-ossp",
			output: []string{"pgcrypto", "uuid-ossp"},
		},
		{
			name:   "valid extension with underscores and hyphens",
			input:  "pg_stat_statements,uuid-ossp,my_ext-123",
			output: []string{"pg_stat_statements", "uuid-ossp", "my_ext-123"},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ControllerConfiguration{
				GlobalExtensionsToInstall: tc.input,
			}
			output := cfg.GetGlobalExtensions()
			assert.Equal(t, tc.output, output, "parsed extensions not as expected")
		})
	}
}
