package postgres_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
)

func TestUserDatabases(t *testing.T) {
	host := test.Integration(t)
	log := test.SetLogger(t)

	adminDB, err := postgres.Connect(log, postgres.ConnectionString{
		Host:     host,
		Database: "postgres",
		User:     "iam_creator",
		Password: "iam_creator",
	})
	require.NoError(t, err)
	defer adminDB.Close()

	epoch := time.Now().UnixNano()
	dbName := fmt.Sprintf("test_%d", epoch)

	require.NoError(t, createManagerRole(log, adminDB, "postgres_role_name"))
	require.NoError(t, postgres.Database(log, host,
		postgres.Credentials{User: "iam_creator", Password: "iam_creator"},
		postgres.Credentials{Name: dbName, User: dbName, Password: "test"},
		"postgres_role_name", nil,
	))

	databases, err := postgres.UserDatabases(adminDB)
	require.NoError(t, err)

	assert.Contains(t, databases, dbName, "created database should appear in list")
	assert.NotContains(t, databases, "postgres", "postgres maintenance database should be excluded")
	assert.NotContains(t, databases, "rdsadmin", "RDS internal database should be excluded")
}
