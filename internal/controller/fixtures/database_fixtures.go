package fixtures

import (
	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (f *Fixture) GivenADatabaseResourceExists() *Fixture {
	f.log.Info("given a database resource exists")

	databaseResource := &v1alpha1.PostgreSQLDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.databaseName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLDatabaseSpec{
			Name: f.data.databaseName,
			User: v1alpha1.ResourceVar{
				Value: f.data.databaseName,
			},
			Password: &v1alpha1.ResourceVar{
				Value: f.data.databaseName,
			},
			Host: v1alpha1.ResourceVar{
				Value: f.host,
			},
		},
	}

	f.log.Info("adding kubernetes resources")
	f.addK8sResources(databaseResource)

	f.log.Info("checking databse resources exists")
	checkResource(f,
		f.toNamespacedName(f.data.databaseName),
		func(t *assert.CollectT, obj *v1alpha1.PostgreSQLDatabase) {
			assert.Equal(t, f.host, obj.Spec.Host.Value)
			assert.Empty(t, obj.Status.Error, "database resource shouldn't return an error")
			assert.Equal(t, v1alpha1.PostgreSQLDatabasePhaseRunning, obj.Status.Phase)
			assert.NotEmpty(t, obj.Status.PhaseUpdated)
		},
	)

	return f
}

func (f *Fixture) WhenADatabaseResourceWithHostCredentialsIsAdded() *Fixture {
	f.log.Info("when a database resource is added")

	databaseResource := &v1alpha1.PostgreSQLDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.databaseName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLDatabaseSpec{
			Name:            f.data.databaseName,
			HostCredentials: f.toResourceName(f.data.hostCredentialsName),
			Password: &v1alpha1.ResourceVar{
				Value: f.data.password,
			},
			User: v1alpha1.ResourceVar{
				Value: f.data.databaseName,
			},
		},
	}

	f.log.Info("adding kubernetes resources")
	f.addK8sResources(databaseResource)

	return f
}

func (f *Fixture) WhenADatabaseResourceWithNoPasswordAndWithHostCredentialsIsAdded() *Fixture {
	f.log.Info("when a database resource is added with no password and host credentials")

	databaseResource := &v1alpha1.PostgreSQLDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.databaseName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLDatabaseSpec{
			Name:            f.data.databaseName,
			HostCredentials: f.toResourceName(f.data.hostCredentialsName),
			User: v1alpha1.ResourceVar{
				Value: f.data.databaseName,
			},
		},
	}

	f.log.Info("adding kubernetes resources")
	f.addK8sResources(databaseResource)

	return f
}

func (f *Fixture) WhenADatabaseResourceWithMissingHostCredentialsIsAdded() *Fixture {
	f.log.Info("when a database resource is added with missing host credentials")

	databaseResource := &v1alpha1.PostgreSQLDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.databaseName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLDatabaseSpec{
			Name:            f.data.databaseName,
			HostCredentials: "bogus",
			User: v1alpha1.ResourceVar{
				Value: f.data.databaseName,
			},
		},
	}

	f.log.Info("adding kubernetes resources")
	f.addK8sResources(databaseResource)

	return f
}

func (f *Fixture) ThenDatabaseResourceIsReconciled() *Fixture {
	f.log.Info("then database resource is reconciled")

	f.log.Info("checking database resources exists")
	checkResource(f,
		f.toNamespacedName(f.data.databaseName),
		func(t *assert.CollectT, obj *v1alpha1.PostgreSQLDatabase) {
			assert.Empty(t, obj.Status.Error, "database resource shouldn't return an error")
			assert.Equal(t, v1alpha1.PostgreSQLDatabasePhaseRunning, obj.Status.Phase)
			assert.NotEmpty(t, obj.Status.PhaseUpdated)
		},
	)

	return f
}

func (f *Fixture) ThenDatabaseResourceIsRetried() *Fixture {
	f.log.Info("then database resource is retried")

	f.log.Info("checking database resources exists")
	checkResource(f,
		f.toNamespacedName(f.data.databaseName),
		func(t *assert.CollectT, obj *v1alpha1.PostgreSQLDatabase) {
			assert.NotEmpty(t, obj.Status.Error, "database resource shouldn't return an error")
			assert.Equal(t, v1alpha1.PostgreSQLDatabasePhaseFailed, obj.Status.Phase)
			assert.NotEmpty(t, obj.Status.PhaseUpdated)
		},
	)

	return f
}

func (f *Fixture) GivenTwoDatabaseResourcesExists() *Fixture {
	resources := make([]client.Object, 0)
	for i := 0; i < 2; i++ {
		resources = append(resources, &v1alpha1.PostgreSQLDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.toResourceName(f.incrementResource(i, f.data.databaseName)),
				Namespace: f.data.namespace,
			},
			Spec: v1alpha1.PostgreSQLDatabaseSpec{
				Name: f.incrementResource(i, f.data.databaseName),
				User: v1alpha1.ResourceVar{
					Value: f.incrementResource(i, f.data.databaseName),
				},
				Password: &v1alpha1.ResourceVar{
					Value: f.data.databaseName,
				},
				Host: v1alpha1.ResourceVar{
					Value: f.host,
				},
			},
		})
	}

	f.addK8sResources(resources...)

	for i := 0; i < 2; i++ {
		f.log.Info("checking database resource", "name", f.incrementResource(i, f.data.databaseName))

		checkResource(f,
			f.toNamespacedName(f.incrementResource(i, f.data.databaseName)),
			func(t *assert.CollectT, obj *v1alpha1.PostgreSQLDatabase) {
				assert.Equal(t, f.incrementResource(i, f.data.databaseName), obj.Spec.Name)
				assert.Equal(t, f.host, obj.Spec.Host.Value)
				assert.Empty(t, obj.Status.Error, "database resource shouldn't return an error")
				assert.Equal(t, v1alpha1.PostgreSQLDatabasePhaseRunning, obj.Status.Phase)
				assert.NotEmpty(t, obj.Status.PhaseUpdated)
			},
		)
	}

	return f
}
