package controller

import (
	"testing"

	"go.lunarway.com/postgresql-controller/internal/controller/fixtures"
)

func TestPostgreSQLServiceUser(t *testing.T) {
	t.Parallel()

	t.Run(
		"can reconcile service user against database",
		fixtures.Test(
			func(f *fixtures.Fixture) {
				f.
					GivenASeededDatabase().
					GivenADatabaseResourceExists().
					WhenAServiceUserResourceIsAdded().
					ThenAServiceUserIsSetup()
			},
			fixtures.WithKubeClient(k8sClient),
		),
	)

	t.Run(
		"can reconcile against two databases",
		fixtures.Test(
			func(f *fixtures.Fixture) {
				f.
					GivenASeededDatabase().
					GivenTwoDatabaseResourcesExists().
					WhenAServiceUserResourceIsAdded().
					ThenAServiceUserIsSetup()
			},
			fixtures.WithKubeClient(k8sClient),
		),
	)
}
