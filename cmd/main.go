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
	"net/http"
	"net/url"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ngrok/ngrok-api-go/v5"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ngrok/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	gatewaycontroller "github.com/ngrok/kubernetes-ingress-controller/internal/controller/gateway"
	controllers "github.com/ngrok/kubernetes-ingress-controller/internal/controller/ingress"
	ngrokctr "github.com/ngrok/kubernetes-ingress-controller/internal/controller/ngrok"
	"github.com/ngrok/kubernetes-ingress-controller/internal/ngrokapi"
	"github.com/ngrok/kubernetes-ingress-controller/internal/store"
	"github.com/ngrok/kubernetes-ingress-controller/internal/version"
	"github.com/ngrok/kubernetes-ingress-controller/pkg/tunneldriver"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	if err := cmd().Execute(); err != nil {
		setupLog.Error(err, "error running manager")
		os.Exit(1)
	}
}

type managerOpts struct {
	// flags
	metricsAddr               string
	electionID                string
	probeAddr                 string
	serverAddr                string
	apiURL                    string
	controllerName            string
	watchNamespace            string
	metaData                  string
	managerName               string
	useExperimentalGatewayAPI bool
	zapOpts                   *zap.Options
	clusterDomain             string

	// feature flags
	enableFeatureIngress  bool
	enableFeatureBindings bool

	// env vars
	namespace   string
	ngrokAPIKey string

	region string

	rootCAs string
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
	c.Flags().StringVar(&opts.metaData, "metadata", "", "A comma separated list of key value pairs such as 'key1=value1,key2=value2' to be added to ngrok api resources as labels")
	c.Flags().StringVar(&opts.region, "region", "", "The region to use for ngrok tunnels")
	c.Flags().StringVar(&opts.serverAddr, "server-addr", "", "The address of the ngrok server to use for tunnels")
	c.Flags().StringVar(&opts.apiURL, "api-url", "", "The base URL to use for the ngrok api")
	c.Flags().StringVar(&opts.controllerName, "controller-name", "k8s.ngrok.com/ingress-controller", "The name of the controller to use for matching ingresses classes")
	c.Flags().StringVar(&opts.watchNamespace, "watch-namespace", "", "Namespace to watch for Kubernetes resources. Defaults to all namespaces.")
	c.Flags().StringVar(&opts.managerName, "manager-name", "ngrok-ingress-controller-manager", "Manager name to identify unique ngrok ingress controller instances")
	c.Flags().BoolVar(&opts.useExperimentalGatewayAPI, "use-experimental-gateway-api", false, "sets up experemental gatewayAPI")
	c.Flags().StringVar(&opts.clusterDomain, "cluster-domain", "svc.cluster.local", "Cluster domain used in the cluster")
	c.Flags().StringVar(&opts.rootCAs, "root-cas", "trusted", "trusted (default) or host: use the trusted ngrok agent CA or the host CA")

	// feature flags
	// default always enabled for now
	// c.Flags().BoolVar(&opts.enableFeatureIngress, "enable-feature-ingress", true, "Enables the Ingress controller")
	opts.enableFeatureIngress = true
	c.Flags().BoolVar(&opts.enableFeatureBindings, "enable-feature-bindings", false, "Enables the Endpoint Bindings controller")

	opts.zapOpts = &zap.Options{}
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

	buildInfo := version.Get()
	setupLog.Info("starting manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	clientConfigOpts := []ngrok.ClientConfigOption{
		ngrok.WithUserAgent(version.GetUserAgent()),
	}

	ngrokClientConfig := ngrok.NewClientConfig(opts.ngrokAPIKey, clientConfigOpts...)
	apiBaseURL := os.Getenv("NGROK_API_ADDR")
	if opts.apiURL != "" {
		apiBaseURL = opts.apiURL
	}
	if apiBaseURL != "" {
		u, err := url.Parse(apiBaseURL)
		if err != nil {
			setupLog.Error(err, "invalid NGROK_API_ADDR")
		}
		ngrokClientConfig.BaseURL = u
	}
	setupLog.Info("configured API client", "base_url", ngrokClientConfig.BaseURL)

	ngrokClientset := ngrokapi.NewClientSet(ngrokClientConfig)
	options := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: opts.metricsAddr,
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         opts.electionID != "",
		LeaderElectionID:       opts.electionID,
	}

	if opts.watchNamespace != "" {
		options.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.watchNamespace: {},
			},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	driver, err := getDriver(ctx, mgr, opts)
	if err != nil {
		return fmt.Errorf("unable to create Driver: %w", err)
	}

	if err := (&controllers.IngressReconciler{
		Client:               mgr.GetClient(),
		Log:                  ctrl.Log.WithName("controllers").WithName("ingress"),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("ingress-controller"),
		Namespace:            opts.namespace,
		AnnotationsExtractor: annotations.NewAnnotationsExtractor(),
		Driver:               driver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ingress controller: %w", err)
	}

	if err = (&controllers.ServiceReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("service"),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("service-controller"),
		Namespace: opts.namespace,
		Driver:    driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
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

	var comments tunneldriver.TunnelDriverComments

	if opts.useExperimentalGatewayAPI {
		comments = tunneldriver.TunnelDriverComments{
			Gateway: "gateway-api",
		}
	}

	rootCAs := "trusted"

	if opts.rootCAs != "" {
		rootCAs = opts.rootCAs
	}

	td, err := tunneldriver.New(ctx, ctrl.Log.WithName("drivers").WithName("tunnel"),
		tunneldriver.TunnelDriverOpts{
			ServerAddr: opts.serverAddr,
			Region:     opts.region,
			RootCAs:    rootCAs,
			Comments:   &comments,
		},
	)

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
	if err = (&controllers.TLSEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("tls-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("tls-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TLSEdge")
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
	if err = (&controllers.ModuleSetReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("ngrok-module-set"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ngrok-module-set-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NgrokModuleSet")
		os.Exit(1)
	}
	if opts.useExperimentalGatewayAPI {
		if err = (&gatewaycontroller.GatewayReconciler{
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("gateway-controller"),
			Driver:   driver,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Gateway")
			os.Exit(1)
		}

		if err = (&gatewaycontroller.HTTPRouteReconciler{
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("gateway-controller"),
			Driver:   driver,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "HTTPRoute")
			os.Exit(1)
		}
	}

	if err = (&ngrokctr.NgrokTrafficPolicyReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("traffic-policy"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("policy-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrafficPolicy")
		os.Exit(1)
	}

	if opts.enableFeatureBindings {
		setupLog.Info("Endpoint Bindings controller enabled")
		setupLog.Info("not yet implemented")
	} else {
		setupLog.Info("Endpoint Bindings controller disabled")
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		return td.Ready()
	}); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		return td.Healthy()
	}); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting manager: %w", err)
	}

	return nil
}

// getDriver returns a new Driver instance that is seeded with the current state of the cluster.
func getDriver(ctx context.Context, mgr manager.Manager, options managerOpts) (*store.Driver, error) {
	logger := mgr.GetLogger().WithName("cache-store-driver")
	d := store.NewDriver(
		logger,
		mgr.GetScheme(),
		options.controllerName,
		types.NamespacedName{
			Namespace: options.namespace,
			Name:      options.managerName,
		},
		store.WithGatewayEnabled(options.useExperimentalGatewayAPI),
		store.WithClusterDomain(options.clusterDomain),
	)
	if options.metaData != "" {
		metaData := strings.TrimSuffix(options.metaData, ",")
		// metadata is a comma separated list of key=value pairs.
		// e.g. "foo=bar,baz=qux"
		customMetaData := make(map[string]string)
		pairs := strings.Split(metaData, ",")
		for _, pair := range pairs {
			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid metadata pair: %q", pair)
			}
			customMetaData[kv[0]] = kv[1]
		}
		d.WithMetaData(customMetaData)
	}

	if err := d.Seed(ctx, mgr.GetAPIReader()); err != nil {
		return nil, fmt.Errorf("unable to seed cache store: %w", err)
	}

	d.PrintState(setupLog)

	return d, nil
}
