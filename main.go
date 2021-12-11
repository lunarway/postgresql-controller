/*
Copyright 2021.

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
	"flag"
	"os"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	postgresqllunartechv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	postgresqlv1alpha1 "go.lunarway.com/postgresql-controller/api/v1alpha1"
	"go.lunarway.com/postgresql-controller/controllers"
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

	utilruntime.Must(postgresqllunartechv1alpha1.AddToScheme(scheme))
	utilruntime.Must(postgresqlv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	flagSet := pflag.NewFlagSet("postgresql-controller", pflag.ExitOnError)
	config := controllerConfiguration{}
	config.RegisterFlags(flagSet)
	flagSet.AddGoFlagSet(flag.CommandLine)
	zapFlagSet := flag.NewFlagSet("zap-flags", flag.ExitOnError)
	loggerOptions := zap.Options{}
	loggerOptions.BindFlags(zapFlagSet)
	flagSet.AddGoFlagSet(zapFlagSet)

	flagSet.Parse(os.Args[1:])

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&loggerOptions)))
	config.Log(setupLog)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     config.MetricsAddress,
		HealthProbeBindAddress: config.ProbeAddress,
		Port:                   9443,
		LeaderElection:         config.EnableLeaderElection,
		LeaderElectionID:       "b64d2659.lunar.tech",
		SyncPeriod:             &config.ResyncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.PostgreSQLUserReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("ccontrollers").WithName("PostgreSQLUser"),
		Scheme: mgr.GetScheme(),

		Granter: grants.Granter{
			AllDatabasesReadEnabled:  config.AllDatabasesReadEnabled,
			AllDatabasesWriteEnabled: config.AllDatabasesWriteEnabled,
			ExtendedWritesEnabled:    config.ExtendedWriteEnabled,
			HostCredentials:          config.HostCredentials,
			StaticRoles:              config.UserRoles,

			Now: time.Now,
			AllDatabases: func(namespace string) ([]postgresqllunartechv1alpha1.PostgreSQLDatabase, error) {
				return kube.PostgreSQLDatabases(mgr.GetClient(), namespace)
			},
			ResourceResolver: func(resource postgresqllunartechv1alpha1.ResourceVar, namespace string) (string, error) {
				return kube.ResourceValue(mgr.GetClient(), resource, namespace)
			},
		},
		AddUser:    iam.AddUser,
		RemoveUser: iam.RemoveUser,

		RolePrefix:         config.UserRolePrefix,
		AWSPolicyName:      config.AWS.PolicyName,
		AWSRegion:          config.AWS.Region,
		AWSAccountID:       config.AWS.AccountID,
		AWSProfile:         config.AWS.Profile,
		AWSAccessKeyID:     config.AWS.AccessKeyID,
		AWSSecretAccessKey: config.AWS.SecretAccessKey,
		IAMPolicyPrefix:    config.IAMPolicyPrefix,
		AWSLoginRoles:      strings.Split(config.AWS.LoginRoles, ","),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLUser")
		os.Exit(1)
	}
	if err = (&controllers.PostgreSQLDatabaseReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("PostgreSQLDatabase"),
		Scheme: mgr.GetScheme(),

		HostCredentials: config.HostCredentials,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLDatabase")
		os.Exit(1)
	}
	if err = (&controllers.PostgreSQLHostCredentialsReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgreSQLHostCredentials")
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
