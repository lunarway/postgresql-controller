package kube_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretValue(t *testing.T) {
	tt := []struct {
		name       string
		secretName string
		namespace  string
		key        string
		value      string
		output     string
		err        error
	}{
		{
			name:       "sunshine",
			secretName: "test",
			namespace:  "test",
			key:        "test",
			value:      "dGVzdA==",
			output:     "test",
			err:        nil,
		},
		{
			name:       "illegal base64",
			secretName: "test",
			namespace:  "test",
			key:        "test",
			value:      "dGVzdA",
			output:     "",
			err:        errors.New("illegal base64 data at input byte 4"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			test.SetLogger(t)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.secretName,
					Namespace: tc.namespace,
				},
				Data: map[string][]byte{
					tc.key: []byte(tc.value),
				},
			}
			// Objects to track in the fake client.
			objs := []runtime.Object{
				secret,
			}

			// Create a fake client to mock API calls.
			cl := fake.NewFakeClient(objs...)

			namespacedName := types.NamespacedName{
				Name:      tc.secretName,
				Namespace: tc.namespace,
			}

			password, err := kube.SecretValue(cl, namespacedName, tc.key)
			if tc.err != nil {
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error: %v", err.Error())
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}

func TestReconcilePostgreSQLDatabase_getConfigMapValue(t *testing.T) {
	tt := []struct {
		name          string
		configMapName string
		namespace     string
		key           string
		value         string
		output        string
		err           error
	}{
		{
			name:          "sunshine",
			configMapName: "test",
			namespace:     "test",
			key:           "test",
			value:         "test",
			output:        "test",
			err:           nil,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			test.SetLogger(t)
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.configMapName,
					Namespace: tc.namespace,
				},
				Data: map[string]string{
					tc.key: tc.value,
				},
			}
			// Objects to track in the fake client.
			objs := []runtime.Object{
				configMap,
			}

			// Create a fake client to mock API calls.
			cl := fake.NewFakeClient(objs...)

			namespacedName := types.NamespacedName{
				Name:      tc.configMapName,
				Namespace: tc.namespace,
			}
			password, err := kube.ConfigMapValue(cl, namespacedName, tc.key)
			if tc.err != nil {
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error: %v", err.Error())
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}
