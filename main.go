/*
Copyright 2022.

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
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ngrok/ngrok-api-go/v5"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/controllers"
	"github.com/ngrok/kubernetes-ingress-controller/internal/ngrokapi"
	"github.com/ngrok/kubernetes-ingress-controller/internal/store"
	"github.com/ngrok/kubernetes-ingress-controller/pkg/tunneldriver"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
}

func main() {
	if err := cmd().Execute(); err != nil {
		setupLog.Error(err, "error running manager")
		os.Exit(1)
	}
}

type managerOpts struct {
	// flags
	metricsAddr string
	electionID  string
	probeAddr   string
	serverAddr  string
	zapOpts     *zap.Options

	// env vars
	namespace   string
	ngrokAPIKey string

	region string
}

func cmd() *cobra.Command {
	var opts managerOpts
	c := &cobra.Command{
		Use: "manager",
		RunE: func(c *cobra.Command, args []string) error {
			return runController(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.electionID, "election-id", "ngrok-ingress-controller-leader", "The name of the configmap that is used for holding the leader lock")
	c.Flags().StringVar(&opts.region, "region", "us", "The region to use for ngrok tunnels")
	c.Flags().StringVar(&opts.serverAddr, "server-addr", "", "The address of the ngrok server to use for tunnels")
	opts.zapOpts = &zap.Options{Development: true, StacktraceLevel: zapcore.DPanicLevel}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

func runController(ctx context.Context, opts managerOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))

	var ok bool
	opts.namespace, ok = os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}

	opts.ngrokAPIKey, ok = os.LookupEnv("NGROK_API_KEY")
	if !ok {
		return errors.New("NGROK_API_KEY environment variable should be set, but was not")
	}

	clientConfigOpts := []ngrok.ClientConfigOption{
		ngrok.WithUserAgent("ngrok-ingress-controller/v1-alpha"),
	}

	ngrokClientConfig := ngrok.NewClientConfig(opts.ngrokAPIKey, clientConfigOpts...)
	apiBaseURL := os.Getenv("NGROK_API_ADDR")
	if apiBaseURL != "" {
		u, err := url.Parse(apiBaseURL)
		if err != nil {
			setupLog.Error(err, "invalid NGROK_API_ADDR")
		}
		ngrokClientConfig.BaseURL = u
	}

	ngrokClientset := ngrokapi.NewClientSet(ngrokClientConfig)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     opts.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         opts.electionID != "",
		LeaderElectionID:       opts.electionID,
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	driver, err := getDriver(ctx, mgr)
	if err != nil {
		return fmt.Errorf("unable to create Driver: %w", err)
	}

	if err := (&controllers.IngressReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("ingress"),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("ingress-controller"),
		Namespace: opts.namespace,
		Driver:    driver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ingress controller: %w", err)
	}

	if err = (&controllers.DomainReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("domain"),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("domain-controller"),
		DomainsClient: ngrokClientset.Domains(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Domain")
		os.Exit(1)
	}

	td, err := tunneldriver.New(tunneldriver.TunnelDriverOpts{
		ServerAddr: opts.serverAddr,
		Region:     opts.region,
	})
	if err != nil {
		return fmt.Errorf("unable to create tunnel driver: %w", err)
	}
	if err = (&controllers.TunnelReconciler{
		Client:       mgr.GetClient(),
		Log:          ctrl.Log.WithName("controllers").WithName("tunnel"),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("tunnel-controller"),
		TunnelDriver: td,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tunnel")
		os.Exit(1)
	}
	if err = (&controllers.TCPEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("tcp-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("tcp-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TCPEdge")
		os.Exit(1)
	}
	if err = (&controllers.HTTPSEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("https-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("https-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPSEdge")
		os.Exit(1)
	}
	if err = (&controllers.IPPolicyReconciler{
		Client:              mgr.GetClient(),
		Log:                 ctrl.Log.WithName("controllers").WithName("ip-policy"),
		Scheme:              mgr.GetScheme(),
		Recorder:            mgr.GetEventRecorderFor("ip-policy-controller"),
		IPPoliciesClient:    ngrokClientset.IPPolicies(),
		IPPolicyRulesClient: ngrokClientset.IPPolicyRules(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPPolicy")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting manager: %w", err)
	}

	return nil
}

// getDriver returns a new Driver instance that is seeded with the current state of the cluster.
func getDriver(ctx context.Context, mgr manager.Manager) (*store.Driver, error) {
	logger := mgr.GetLogger().WithName("cache-store-driver")
	d := store.NewDriver(logger, mgr.GetScheme())
	if err := d.Seed(ctx, mgr.GetAPIReader()); err != nil {
		return nil, fmt.Errorf("unable to seed cache store: %w", err)
	}

	ings := d.ListNgrokIngressesV1()
	for _, ing := range ings {
		setupLog.Info("found matching ingress", "ingress-name", ing.Name, "ingress-namespace", ing.Namespace)
	}

	// Helpful debug information if someone doesn't have their ingress class set up correctly.
	if len(ings) == 0 {
		ingresses := d.ListIngressesV1()
		ngrokIngresses := d.ListNgrokIngressesV1()
		ingressClasses := d.ListIngressClassesV1()
		ngrokIngressClasses := d.ListNgrokIngressClassesV1()
		setupLog.Info("no matching ingresses found",
			"all ingresses", ingresses,
			"all ngrok ingresses", ngrokIngresses,
			"all ingress classes", ingressClasses,
			"all ngrok ingress classes", ngrokIngressClasses,
		)
	}

	return d, nil
}
