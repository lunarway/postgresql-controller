package main

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
					Name:     "user",
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
					Name:     "user1",
					Password: "pass1",
				},
				"host2:5432": {
					Name:     "user2",
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
					Name:     "user1",
					Password: "pass1",
					Params:   "sslmode=enabled",
				},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output := make(map[string]postgres.Credentials)
			h := hostCredentials{
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
					Name:     "user",
					Password: "pass",
				},
			},
			output: "[host:5432=user:********]",
		},
		{
			name: "multiple hosts",
			value: map[string]postgres.Credentials{
				"host1:5432": {
					Name:     "user1",
					Password: "pass1",
				},
				"host2:5432": {
					Name:     "user2",
					Password: "pass2",
				},
			},
			output: "[host1:5432=user1:********,host2:5432=user2:********]",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			h := hostCredentials{
				value: &tc.value,
			}

			output := h.String()

			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}
