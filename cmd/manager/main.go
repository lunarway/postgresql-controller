package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.lunarway.com/postgresql-controller/pkg/apis"
	"go.lunarway.com/postgresql-controller/pkg/controller"
	"go.lunarway.com/postgresql-controller/pkg/controller/postgresqldatabase"
	"go.lunarway.com/postgresql-controller/pkg/controller/postgresqluser"
	"go.lunarway.com/postgresql-controller/pkg/daemon"
	"go.lunarway.com/postgresql-controller/pkg/instrumentation"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	"github.com/operator-framework/operator-sdk/pkg/restmapper"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	runtimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.CommandLine.AddFlagSet(postgresqldatabase.FlagSet)
	pflag.CommandLine.AddFlagSet(postgresqluser.FlagSet)
	resyncDuration := pflag.CommandLine.Duration("resync-period", 10*time.Hour, "determines the minimum frequency at which watched resources are reconciled")
	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "postgresql-controller-lock")
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:          namespace,
		MapperProvider:     restmapper.NewDynamicRESTMapper,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		SyncPeriod:         resyncDuration,
	})
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err = serveCRMetrics(cfg); err != nil {
		log.Info("Could not generate and serve custom resource metrics", "error", err.Error())
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
		{Port: operatorMetricsPort, Name: metrics.CRPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: operatorMetricsPort}},
	}
	// Create Service object to expose the metrics port(s).
	service, err := metrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		log.Info("Could not create metrics Service", "error", err.Error())
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, namespace, services)
	if err != nil {
		log.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			log.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}

	// used to signal Go routines to stop execution
	shutdown := make(chan struct{})
	// used to know when any of the started Go routines are stopped
	componentErr := make(chan error)
	// used to wait for all Go routines to stop
	var shutdownWg sync.WaitGroup

	// listen for shutdown signals and signal termination to components
	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		// blocks until a signal is triggered or shutdown is closed
		select {
		case <-signals.SetupSignalHandler():
			// signal that the controller should stop but without an error as this is
			// expected behavour on signals
			componentErr <- nil
		case <-shutdown:
		}
	}()

	instrumentation, err := instrumentation.New(runtimemetrics.Registry)
	if err != nil {
		log.Error(err, "Instantiate instrumentation probes failed")
		os.Exit(1)
	}

	log.Info("Starting grant expiration daemon")
	daemon := daemon.New(daemon.Configuration{
		Logger:       log.WithName("daemon"),
		SyncInterval: 5 * time.Second,
		Sync: func() {
			s := time.Now()
			log.Info("Syncing resources...")
			var err error
			// TODO: do actual syncing
			if err != nil {
				log.Error(err, "Syncronization of resources failed")
			}
			instrumentation.ObserveSyncDuration(time.Since(s), err == nil)
		},
	})
	shutdownWg.Add(1)
	// no link to componentErr as the daemon loop should only ever exit on the
	// shutdown signal.
	go func() {
		defer shutdownWg.Done()
		daemon.Loop(shutdown)
	}()

	log.Info("Starting the Cmd.")

	// Start the Cmd
	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		err := mgr.Start(shutdown)
		if err != nil {
			log.Error(err, "Manager exited non-zero")
			os.Exit(1)
		}
	}()
	// wait for any component to stop, ie. signals or the manager
	err = <-componentErr
	if err != nil {
		log.Error(err, "Controller exiting unexpectedly")
	} else {
		log.Info("Controller exiting")
	}
	close(shutdown)
	log.Info("Waiting for all components to shutdown")
	shutdownWg.Wait()
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
func serveCRMetrics(cfg *rest.Config) error {
	// Below function returns filtered operator/CustomResource specific GVKs.
	// For more control override the below GVK list with your own custom logic.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(apis.AddToScheme)
	if err != nil {
		return err
	}
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err
	}
	// To generate metrics in other namespaces, add the values below.
	ns := []string{operatorNs}
	// Generate and serve custom resource specific metrics.
	err = kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, operatorMetricsPort)
	if err != nil {
		return err
	}
	return nil
}
