package kube

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	lunarwayv1alpha1 "go.lunarway.com/postgresql-controller/pkg/apis/lunarway/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceValue returns the value of a ResourceVar in a specific namespace.
func ResourceValue(client client.Client, resource lunarwayv1alpha1.ResourceVar, namespace string) (string, error) {
	if resource.Value != "" {
		return resource.Value, nil
	}

	if resource.ValueFrom != nil && resource.ValueFrom.SecretKeyRef != nil && resource.ValueFrom.SecretKeyRef.Key != "" {
		return SecretValue(client, types.NamespacedName{Name: resource.ValueFrom.SecretKeyRef.Name, Namespace: namespace}, resource.ValueFrom.SecretKeyRef.Key)
	}

	if resource.ValueFrom != nil && resource.ValueFrom.ConfigMapKeyRef != nil && resource.ValueFrom.ConfigMapKeyRef.Key != "" {
		return ConfigMapValue(client, types.NamespacedName{Name: resource.ValueFrom.ConfigMapKeyRef.Name, Namespace: namespace}, resource.ValueFrom.ConfigMapKeyRef.Key)
	}

	return "", fmt.Errorf("no value")
}

func SecretValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, secret)
	if err != nil {
		return "", fmt.Errorf("get secret %s/%s: %w", namespacedName.Namespace, namespacedName.Name, err)
	}
	secretData, ok := secret.Data[key]
	if !ok {
		return "", errors.New("unknown secret key")
	}
	password, err := base64.StdEncoding.DecodeString(string(secretData))
	if err != nil {
		return "", fmt.Errorf("base64 decode secret %s/%s key '%s': %w", namespacedName.Namespace, namespacedName.Name, key, err)
	}
	return string(password), nil
}

func ConfigMapValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, configMap)
	if err != nil {
		return "", fmt.Errorf("get config map %s/%s: %w", namespacedName.Namespace, namespacedName.Name, err)
	}
	data, ok := configMap.Data[key]
	if !ok {
		return "", errors.New("unknown config map key")
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
