package controller

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
	"go.lunarway.com/postgresql-controller/test"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())

	// Before tests
	if err := beforeTests(ctx); err != nil {
		log.Fatalf("failed to setup dependencies for tests: %s", err.Error())
	}

	exitCode := m.Run()

	// Tear down after tests
	if err := afterTests(ctx, cancel); err != nil {
		log.Fatalf("failed to cleanup after tests: %s", err.Error())
	}

	os.Exit(exitCode)
}

func beforeTests(ctx context.Context) error {
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

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return err
	}

	// Add Reconcilers here mimicing the setup in cmd/main.go

	if err = (&PostgreSQLDatabaseReconciler{
		Client: k8sManager.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("PostgreSQLDatabase"),

		ManagerRoleName: managerRole,
		HostCredentials: map[string]postgres.Credentials{
			test.GetHost(): {
				Name:     "admin",
				User:     "admin",
				Password: "admin",
			},
		},
	}).SetupWithManager(k8sManager); err != nil {
		return err
	}

	if err = (&PostgreSQLHostCredentialsReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager); err != nil {
		return err
	}

	if err = (&PostgreSQLServiceUserReconciler{
		Client: k8sClient,
		Scheme: scheme.Scheme,
	}).SetupWithManager(k8sManager); err != nil {
		return err
	}

	go func() {
		err = k8sManager.Start(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}()

	// wait until k8s has been elected, i.e. it is ready
	<-k8sManager.Elected()

	return nil
}

func afterTests(_ context.Context, cancel func()) error {
	cancel()

	return testEnv.Stop()
}
