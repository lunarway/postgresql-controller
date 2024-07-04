package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestPostgreSQLServiceUser(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	host := test.Integration(t)

	var (
		namespace = "default"

		timeout  = time.Second * 5
		interval = time.Millisecond * 250
	)

	t.Run("can reconcile user", func(t *testing.T) {
		var (
			ctx = context.Background()

			epoch        = time.Now().UnixNano()
			databaseName = fmt.Sprintf("database_service_user_%d", epoch)
			userName     = fmt.Sprintf("service_user_%d", epoch)
			resourceName = strings.ReplaceAll(userName, "_", "-")
			password     = fmt.Sprintf("service_user_password_%d", epoch)

			serviceUserResource = &v1alpha1.PostgreSQLServiceUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: v1alpha1.PostgreSQLServiceUserSpec{
					Username: v1alpha1.ResourceVar{
						Value: userName,
					},
					Host: v1alpha1.ResourceVar{
						Value: host,
					},
					Password: &v1alpha1.ResourceVar{
						Value: password,
					},
					Roles: []v1alpha1.PostgreSQLServiceUserRole{},
				},
			}

			databaseResource = &v1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: v1alpha1.PostgreSQLDatabaseSpec{
					Name: databaseName,
					User: v1alpha1.ResourceVar{
						Value: databaseName,
					},
					Password: &v1alpha1.ResourceVar{
						Value: databaseName,
					},
					Host: v1alpha1.ResourceVar{
						Value: host,
					},
				},
			}
		)

		seededDatabase(t, host, databaseName, userName, managerRole)

		assert.EventuallyWithT(
			t,
			func(collect *assert.CollectT) {
				err := k8sClient.Create(ctx, databaseResource)
				assert.NoError(collect, err)
			},
			timeout,
			interval,
		)
		assert.EventuallyWithT(
			t,
			func(collect *assert.CollectT) {
				err := k8sClient.Create(ctx, serviceUserResource)
				assert.NoError(collect, err)
			},
			timeout,
			interval,
		)
	})
}
