package kube

import (
	"context"
	"errors"
	"fmt"

	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	ctrerrors "go.lunarway.com/postgresql-controller/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errNoValue    = ctrerrors.NewInvalid(errors.New("no value"))
	errNotFound   = ctrerrors.NewInvalid(errors.New("not found"))
	errUnknownKey = ctrerrors.NewInvalid(errors.New("unknown key"))
)

// ResourceValue returns the value of a ResourceVar in a specific namespace.
func ResourceValue(client client.Client, resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
	if resource.Value != "" {
		return resource.Value, nil
	}

	if resource.ValueFrom != nil && resource.ValueFrom.SecretKeyRef != nil && resource.ValueFrom.SecretKeyRef.Key != "" {
		namespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      resource.ValueFrom.SecretKeyRef.Name,
		}
		key := resource.ValueFrom.SecretKeyRef.Key
		v, err := SecretValue(client, namespacedName, key)
		if err != nil {
			return "", fmt.Errorf("secret %s key %s: %w", namespacedName, key, err)
		}
		return v, nil
	}

	if resource.ValueFrom != nil && resource.ValueFrom.ConfigMapKeyRef != nil && resource.ValueFrom.ConfigMapKeyRef.Key != "" {
		namespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      resource.ValueFrom.ConfigMapKeyRef.Name,
		}
		key := resource.ValueFrom.ConfigMapKeyRef.Key
		v, err := ConfigMapValue(client, namespacedName, key)
		if err != nil {
			return "", fmt.Errorf("config map %s key %s: %w", namespacedName, key, err)
		}
		return v, nil
	}

	return "", errNoValue
}

func SecretValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), namespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", errNotFound
		}
		return "", err
	}
	secretData, ok := secret.Data[key]
	if !ok {
		return "", errUnknownKey
	}
	return string(secretData), nil
}

func ConfigMapValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), namespacedName, configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", errNotFound
		}
		return "", err
	}
	data, ok := configMap.Data[key]
	if !ok {
		return "", errUnknownKey
	}
	return string(data), nil
}

func PostgreSQLDatabases(c client.Client, namespace string) ([]lunarwayv1alpha1.PostgreSQLDatabase, error) {
	var databases lunarwayv1alpha1.PostgreSQLDatabaseList
	err := c.List(context.TODO(), &databases, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("get databases in namespace: %w", err)
	}
	return databases.Items, nil
}
