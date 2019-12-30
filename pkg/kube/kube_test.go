package kube_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	"go.lunarway.com/postgresql-controller/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceValue(t *testing.T) {
	tt := []struct {
		name      string
		resource  lunarwayv1alpha1.ResourceVar
		namespace string
		objs      []runtime.Object
		output    string
		err       error
	}{
		{
			name: "raw value resource",
			resource: lunarwayv1alpha1.ResourceVar{
				Value: "hello",
			},
			namespace: "default",
			objs:      nil,
			output:    "hello",
			err:       nil,
		},
		{
			name: "empty raw value resource",
			resource: lunarwayv1alpha1.ResourceVar{
				Value: "",
			},
			namespace: "default",
			objs:      nil,
			output:    "",
			err:       errors.New("no value"),
		},
		{
			name: "secret value resource",
			resource: lunarwayv1alpha1.ResourceVar{
				ValueFrom: &lunarwayv1alpha1.ResourceVarSource{
					SecretKeyRef: &lunarwayv1alpha1.KeySelector{
						Name: "secret",
						Key:  "key",
					},
				},
			},
			namespace: "default",
			objs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key": []byte("password"),
					},
				},
			},
			output: "password",
			err:    nil,
		},
		{
			name: "config map resource",
			resource: lunarwayv1alpha1.ResourceVar{
				ValueFrom: &lunarwayv1alpha1.ResourceVarSource{
					ConfigMapKeyRef: &lunarwayv1alpha1.KeySelector{
						Name: "configmap",
						Key:  "key",
					},
				},
			},
			namespace: "default",
			objs: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "configmap",
						Namespace: "default",
					},
					Data: map[string]string{
						"key": "host",
					},
				},
			},
			output: "host",
			err:    nil,
		},
		{
			name: "no value",
			resource: lunarwayv1alpha1.ResourceVar{
				Value:     "",
				ValueFrom: nil,
			},
			namespace: "default",
			objs:      nil,
			output:    "",
			err:       errors.New("no value"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewFakeClient(tc.objs...)

			value, err := kube.ResourceValue(client, tc.resource, tc.namespace)

			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error(), "output error not as expected")
			} else {
				assert.NoError(t, err, "output error unexpected")
			}
			assert.Equal(t, tc.output, value, "output not as expected")
		})
	}
}

func TestSecretValue(t *testing.T) {
	tt := []struct {
		name       string
		secretName string
		namespace  string
		key        string
		data       map[string][]byte
		output     string
		err        error
	}{
		{
			name:       "sunshine",
			secretName: "test",
			namespace:  "test",
			key:        "test",
			data: map[string][]byte{
				"test": []byte("password"),
			},
			output: "password",
			err:    nil,
		},
		{
			name:       "unknown key",
			secretName: "test",
			namespace:  "test",
			key:        "anotherkey",
			data: map[string][]byte{
				"test": []byte("password"),
			},
			output: "",
			err:    errors.New("unknown key"),
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
				Data: tc.data,
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
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error")
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}

func TestConfigMapValue(t *testing.T) {
	tt := []struct {
		name          string
		configMapName string
		namespace     string
		key           string
		data          map[string]string
		output        string
		err           error
	}{
		{
			name:          "sunshine",
			configMapName: "test",
			namespace:     "test",
			key:           "test",
			data: map[string]string{
				"test": "test",
			},
			output: "test",
			err:    nil,
		},
		{
			name:          "unknown key",
			configMapName: "test",
			namespace:     "test",
			key:           "anotherkey",
			data: map[string]string{
				"test": "test",
			},
			output: "",
			err:    errors.New("unknown key"),
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
				Data: tc.data,
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
				assert.EqualErrorf(t, err, tc.err.Error(), "wrong output error")
			} else {
				assert.NoError(t, err, "unexpected output error")
			}
			assert.Equal(t, tc.output, password, "password not as expected")
		})
	}
}
