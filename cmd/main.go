/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	postgresqllunartechv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/internal/config"
	"go.lunarway.com/postgresql-controller/internal/controller"
	"go.lunarway.com/postgresql-controller/pkg/grants"
	"go.lunarway.com/postgresql-controller/pkg/iam"
	"go.lunarway.com/postgresql-controller/pkg/kube"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(postgresqlv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	flagSet := flag.NewFlagSet("postgresql-controller", flag.ExitOnError)

	config := config.ControllerConfiguration{}
	config.RegisterFlags(flagSet)

	//zapFlagSet := flag.NewFlagSet("zap-flags", flag.ExitOnError)
	loggerOptions := zap.Options{}
	loggerOptions.BindFlags(flagSet)

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&loggerOptions)))

	config.Log(setupLog)

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !config.EnableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
		Port:    9443,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   config.MetricsAddress,
			SecureServing: config.SecureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: config.ProbeAddress,
		LeaderElection:         config.EnableLeaderElection,
		Cache: cache.Options{
			SyncPeriod: &config.ResyncPeriod,
		},
		LeaderElectionID: "b64d2659.lunar.tech",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.PostgreSQLDatabaseReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("PostgreSQLDatabase"),

		ManagerRoleName: config.ManagerRoleName,
		HostCredentials: config.HostCredentials,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLDatabase")
		os.Exit(1)
	}
	if err = (&controller.PostgreSQLUserReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),

		Log: ctrl.Log.WithName("controllers").WithName("PostgreSQLUser"),

		Granter: grants.Granter{
			AllDatabasesReadEnabled:  config.AllDatabasesReadEnabled,
			AllDatabasesWriteEnabled: config.AllDatabasesWriteEnabled,
			ExtendedWritesEnabled:    config.ExtendedWriteEnabled,
			HostCredentials:          config.HostCredentials,
			StaticRoles:              config.GetUserRoles(),

			Now: time.Now,
			AllDatabases: func(namespace string) ([]postgresqlv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(mgr.GetClient(), namespace)
			},
			ResourceResolver: func(resource postgresqlv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(mgr.GetClient(), resource, namespace)
			},
		},
		EnsureIAMUser: iam.EnsureUser,
		RemoveIAMUser: iam.RemoveUser,

		RolePrefix:         config.UserRolePrefix,
		AWSPolicyName:      config.AWS.PolicyName,
		AWSRegion:          config.AWS.Region,
		AWSAccountID:       config.AWS.AccountID,
		AWSProfile:         config.AWS.Profile,
		AWSAccessKeyID:     config.AWS.AccessKeyID,
		AWSSecretAccessKey: config.AWS.SecretAccessKey,
		IAMPolicyPrefix:    config.IAMPolicyPrefix,
		AWSLoginRoles:      config.GetLoginRoles(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLUser")
		os.Exit(1)
	}
	if err = (&controller.PostgreSQLHostCredentialsReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLHostCredentials")
		os.Exit(1)
	}
	if err = (&controllers.PostgreSQLServiceUserReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLServiceUser")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
