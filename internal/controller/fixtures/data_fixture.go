package fixtures

import (
	"fmt"
	"time"
)

type fixtureData struct {
	epoch int64

	databaseName string
	userName     string
	password     string
	managerRole  string

	hostCredentialsName string
	adminUsername       string
	adminPassword       string

	namespace string
}

func newFixtureData() fixtureData {
	epoch := time.Now().UnixNano()

	return fixtureData{
		epoch:               epoch,
		databaseName:        fmt.Sprintf("database_%d", epoch),
		userName:            fmt.Sprintf("user_%d", epoch),
		password:            fmt.Sprintf("user_%d", epoch),
		managerRole:         "postgres_role_manager",
		hostCredentialsName: fmt.Sprintf("hostcredentials_%d", epoch),

		adminUsername: "admin",
		adminPassword: "admin",

		namespace: "default",
	}
}
