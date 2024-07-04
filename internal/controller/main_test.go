package controller

import (
	"log"
	"path/filepath"
	"testing"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	// Before tests
	if err := beforeTests(); err != nil {
		log.Fatalf("failed to setup dependencies for tests: %s", err.Error())
	}

	// Tear down after tests
	defer func() {
		if err := afterTests(); err != nil {
			log.Fatalf("failed to cleanup after tests: %s", err.Error())
		}
	}()

	m.Run()
}

func beforeTests() error {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	if err != nil {
		return err
	}

	err = postgresqlv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return err
	}

	return nil
}

func afterTests() error {
	return testEnv.Stop()
}
