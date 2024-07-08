package fixtures

import (
	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Fixture) GivenAHostCredentialResourceExists() *Fixture {
	f.log.Info("given a host credential resource exists")

	hostCredentialResource := &v1alpha1.PostgreSQLHostCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.hostCredentialsName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLHostCredentialsSpec{
			Host: v1alpha1.ResourceVar{
				Value: f.host,
			},
			User: v1alpha1.ResourceVar{
				Value: f.data.adminUsername,
			},
			Password: v1alpha1.ResourceVar{
				Value: f.data.adminPassword,
			},
		},
	}

	f.log.Info("adding kubernetes resources")
	f.addK8sResources(hostCredentialResource)

	f.log.Info("checking databse resources exists")
	checkResource(f,
		f.toNamespacedName(f.data.hostCredentialsName),
		func(t *assert.CollectT, obj *v1alpha1.PostgreSQLHostCredentials) {
			assert.Equal(t, f.host, obj.Spec.Host.Value)
		},
	)

	return f
}
