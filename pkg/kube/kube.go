package kube

import (
	"context"
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SecretValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, secret)
	if err != nil {
		return "", err
	}
	//TODO: Add guard against non-existing keys
	password, err := base64.StdEncoding.DecodeString(string(secret.Data[key]))
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func ConfigMapValue(client client.Client, namespacedName types.NamespacedName, key string) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, configMap)
	if err != nil {
		return "", err
	}
	//TODO: Add guard against non-existing keys
	return string(configMap.Data[key]), nil
}
