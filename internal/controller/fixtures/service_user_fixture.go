package fixtures

import (
	"go.lunarway.com/postgresql-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Fixture) WhenAServiceUserResourceIsAdded() *Fixture {
	serviceUserResource := &v1alpha1.PostgreSQLServiceUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.toResourceName(f.data.userName),
			Namespace: f.data.namespace,
		},
		Spec: v1alpha1.PostgreSQLServiceUserSpec{
			Username: v1alpha1.ResourceVar{
				Value: f.data.userName,
			},
			Host: v1alpha1.ResourceVar{
				Value: f.host,
			},
			Password: &v1alpha1.ResourceVar{
				Value: f.data.password,
			},
			Roles: []v1alpha1.PostgreSQLServiceUserRole{},
		},
	}

	f.addK8sResources(serviceUserResource)

	return f
}

func (f *Fixture) ThenAServiceUserIsSetup() *Fixture {
	// TODO: needs actual implementation to check that the user with password can connect to the database

	return f
}
