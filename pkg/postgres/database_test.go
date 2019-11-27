package postgres_test

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestParseHostCredentials(t *testing.T) {
	tt := []struct {
		name   string
		input  string
		output postgres.Credentials
		err    error
	}{
		{
			name:   "empty string",
			input:  "",
			output: postgres.Credentials{},
			err:    errors.New("username empty"),
		},
		{
			name:  "complete",
			input: "user:password",
			output: postgres.Credentials{
				Name:     "user",
				Password: "password",
			},
			err: nil,
		},
		{
			name:  "no password",
			input: "user",
			output: postgres.Credentials{
				Name:     "user",
				Password: "",
			},
			err: nil,
		},
		{
			name:  "empty password",
			input: "user:",
			output: postgres.Credentials{
				Name:     "user",
				Password: "",
			},
			err: nil,
		},
		{
			name:   "empty username and password",
			input:  ":",
			output: postgres.Credentials{},
			err:    errors.New("username empty"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			output, err := postgres.ParseUsernamePassword(tc.input)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as expected")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
			assert.Equal(t, tc.output, output, "output not as expected")
		})
	}
}

func TestDatabase_sunshine(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(logf.Log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	newDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: name,
		User:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	// Validate Schema
	schemas := storedSchema(t, newDB, name)
	assert.Equal(t, []string{name}, schemas, "schema not as expected")

	// Validate iam_creator not able to see schema
	schemas = storedSchema(t, db, name)
	assert.Equal(t, []string(nil), schemas, "schema not as expected")

	// Validate owner of database
	owners := validateOwner(t, db, name)
	t.Logf("Owners of database: %v", owners)
	assert.Equal(t, []string{name}, owners, "owner not as expected")
}

func TestDatabase_idempotency(t *testing.T) {
	postgresqlHost := test.Integration(t)
	log := test.SetLogger(t)
	db, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     postgresqlHost,
		Database: "",
		User:     "iam_creator",
		Password: "",
	})
	if err != nil {
		t.Fatalf("connect to database failed: %v", err)
	}

	name := fmt.Sprintf("test_%d", time.Now().UnixNano())
	password := "test"

	err = postgres.Database(log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Fatalf("EnsurePostgreSQLDatabase failed: %v", err)
	}

	// Invoke again with same name
	err = postgres.Database(log, db, postgresqlHost, postgres.Credentials{
		Name:     name,
		Password: password,
	})
	if err != nil {
		t.Logf("The error: %#v", err)
		t.Fatalf("Second EnsurePostgreSQLDatabase failed: %v", err)
	}
}

func validateOwner(t *testing.T, db *sql.DB, owner string) []string {
	t.Helper()
	rows, err := db.Query("SELECT pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname = $1", owner)
	if err != nil {
		t.Fatalf("get owner failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

// storedRoles returns roles for a specific user name sorted by name.
func storedSchema(t *testing.T, db *sql.DB, schemaName string) []string {
	t.Helper()
	rows, err := db.Query("select schema_name from information_schema.schemata where schema_name = $1", schemaName)
	if err != nil {
		t.Fatalf("get schema for schema query failed: %v", err)
	}
	defer rows.Close()
	return stringsResult(t, rows)
}

func stringsResult(t *testing.T, rows *sql.Rows) []string {
	var results []string
	for rows.Next() {
		var result string
		err := rows.Scan(&result)
		if err != nil {
			t.Fatalf("scan row failed: %v", err)
		}
		results = append(results, result)
	}
	err := rows.Err()
	if err != nil {
		t.Fatalf("scanning rows failed: %v", err)
	}
	sort.Strings(results)
	return results
}
