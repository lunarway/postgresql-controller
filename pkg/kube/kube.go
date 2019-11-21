package kube

import (
	"context"
	"encoding/base64"
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

	if resource.ValueFrom.SecretKeyRef.Key != "" {
		return SecretValue(client, types.NamespacedName{Name: resource.ValueFrom.SecretKeyRef.Name, Namespace: namespace}, resource.ValueFrom.SecretKeyRef.Key)
	}

	if resource.ValueFrom.ConfigMapKeyRef.Key != "" {
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
	//TODO: Add guard against non-existing keys
	password, err := base64.StdEncoding.DecodeString(string(secret.Data[key]))
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
	//TODO: Add guard against non-existing keys
	return string(configMap.Data[key]), nil
}
