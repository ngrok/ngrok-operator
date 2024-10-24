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
	"strings"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ngrok/ngrok-api-go/v5"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	bindingscontroller "github.com/ngrok/ngrok-operator/internal/controller/bindings"
	gatewaycontroller "github.com/ngrok/ngrok-operator/internal/controller/gateway"
	ingresscontroller "github.com/ngrok/ngrok-operator/internal/controller/ingress"
	ngrokcontroller "github.com/ngrok/ngrok-operator/internal/controller/ngrok"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	if err := cmd().Execute(); err != nil {
		setupLog.Error(err, "error running api-manager")
		os.Exit(1)
	}
}

type managerOpts struct {
	// flags
	metricsAddr           string
	electionID            string
	probeAddr             string
	serverAddr            string
	apiURL                string
	ingressControllerName string
	ingressWatchNamespace string
	ngrokMetadata         string
	description           string
	managerName           string
	zapOpts               *zap.Options
	clusterDomain         string

	// feature flags
	enableFeatureIngress  bool
	enableFeatureGateway  bool
	enableFeatureBindings bool

	bindings struct {
		allowedURLs []string
		name        string
	}

	// env vars
	namespace   string
	ngrokAPIKey string

	region string
}

func cmd() *cobra.Command {
	var opts managerOpts
	c := &cobra.Command{
		Use: "api-manager",
		RunE: func(c *cobra.Command, args []string) error {
			return runController(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.electionID, "election-id", "ngrok-operator-leader", "The name of the configmap that is used for holding the leader lock")
	c.Flags().StringVar(&opts.ngrokMetadata, "ngrokMetadata", "", "A comma separated list of key=value pairs such as 'key1=value1,key2=value2' to be added to ngrok api resources as labels")
	c.Flags().StringVar(&opts.description, "description", "Created by the ngrok-operator", "Description for this installation")
	c.Flags().StringVar(&opts.region, "region", "", "The region to use for ngrok tunnels")
	c.Flags().StringVar(&opts.serverAddr, "server-addr", "", "The address of the ngrok server to use for tunnels")
	c.Flags().StringVar(&opts.apiURL, "api-url", "", "The base URL to use for the ngrok api")
	// TODO(operator-rename): This probably needs to be on a per controller basis. Each of the controllers will have their own value or we migrate this to k8s.ngrok.com/ngrok-operator.
	c.Flags().StringVar(&opts.ingressControllerName, "ingress-controller-name", "k8s.ngrok.com/ingress-controller", "The name of the controller to use for matching ingresses classes")
	c.Flags().StringVar(&opts.ingressWatchNamespace, "ingress-watch-namespace", "", "Namespace to watch for Kubernetes Ingress resources. Defaults to all namespaces.")
	// TODO(operator-rename): Same as above, but for the manager name.
	c.Flags().StringVar(&opts.managerName, "manager-name", "ngrok-ingress-controller-manager", "Manager name to identify unique ngrok ingress controller instances")
	c.Flags().StringVar(&opts.clusterDomain, "cluster-domain", "svc.cluster.local", "Cluster domain used in the cluster")

	// feature flags
	c.Flags().BoolVar(&opts.enableFeatureIngress, "enable-feature-ingress", true, "Enables the Ingress controller")
	c.Flags().BoolVar(&opts.enableFeatureGateway, "enable-feature-gateway", false, "Enables the Gateway controller")
	c.Flags().BoolVar(&opts.enableFeatureBindings, "enable-feature-bindings", false, "Enables the Endpoint Bindings controller")
	c.Flags().StringSliceVar(&opts.bindings.allowedURLs, "bindings-allowed-urls", []string{"*"}, "Allowed URLs for Endpoint Bindings")
	c.Flags().StringVar(&opts.bindings.name, "bindings-name", "default", "Name of the Endpoint Binding Configuration")

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
	setupLog.Info("starting api-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	clientConfigOpts := []ngrok.ClientConfigOption{
		ngrok.WithUserAgent(version.GetUserAgent()),
	}

	ngrokClientConfig := ngrok.NewClientConfig(opts.ngrokAPIKey, clientConfigOpts...)
	if opts.apiURL != "" {
		u, err := url.Parse(opts.apiURL)
		if err != nil {
			setupLog.Error(err, "api-url must be a valid ngrok API URL")
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

	if opts.ingressWatchNamespace != "" {
		options.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.ingressWatchNamespace: {},
			},
		}
	}

	// create default config and clientset for use outside the mgr.Start() blocking loop
	k8sConfig := ctrl.GetConfigOrDie()
	k8sClinet, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("unable to create k8s client: %w", err)
	}

	if err := createKubernetesOperator(ctx, k8sClinet, opts); err != nil {
		return fmt.Errorf("unable to create KubernetesOperator: %w", err)
	}

	mgr, err := ctrl.NewManager(k8sConfig, options)
	if err != nil {
		return fmt.Errorf("unable to start api-manager: %w", err)
	}

	// k8sResourceDriver is the driver that will be used to interact with the k8s resources for all controllers
	// but primarily for kinds Ingress, Gateway, and ngrok CRDs
	var k8sResourceDriver *store.Driver
	if opts.enableFeatureIngress || opts.enableFeatureGateway {
		// we only need a driver if these features are enabled
		if driver, err := getK8sResourceDriver(ctx, mgr, opts); err != nil {
			return fmt.Errorf("unable to create Driver: %w", err)
		} else {
			k8sResourceDriver = driver
		}
	}

	if opts.enableFeatureIngress {
		setupLog.Info("Ingress feature set enabled")
		if err := enableIngressFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset); err != nil {
			return fmt.Errorf("unable to enable Ingress feature set: %w", err)
		}
	} else {
		setupLog.Info("Ingress feature set disabled")
	}

	if opts.enableFeatureGateway {
		setupLog.Info("Gateway feature set enabled")
		if err := enableGatewayFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset); err != nil {
			return fmt.Errorf("unable to enable Gateway feature set: %w", err)
		}
	} else {
		setupLog.Info("Gateway feature set disabled")
	}

	if opts.enableFeatureBindings {
		setupLog.Info("Endpoint Bindings feature set enabled")
		if err := enableBindingsFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset); err != nil {
			return fmt.Errorf("unable to enable Bindings feature set: %w", err)
		}
	} else {
		setupLog.Info("Endpoint Bindings feature set disabled")
	}

	// new kubebuilder controllers will be generated here
	// please attach these to a feature set
	//+kubebuilder:scaffold:builder

	// Always register the ngrok KubernetesOperator controller. It is independent of the feature set.
	if err := (&ngrokcontroller.KubernetesOperatorReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("KubernetesOperator"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("kubernetes-operator-controller"),
		Namespace:      opts.namespace,
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubernetesOperator")
		os.Exit(1)
	}

	// register healthchecks
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	setupLog.Info("starting api-manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting api-manager: %w", err)
	}

	return nil
}

// getK8sResourceDriver returns a new Driver instance that is seeded with the current state of the cluster.
func getK8sResourceDriver(ctx context.Context, mgr manager.Manager, options managerOpts) (*store.Driver, error) {
	logger := mgr.GetLogger().WithName("cache-store-driver")
	d := store.NewDriver(
		logger,
		mgr.GetScheme(),
		options.ingressControllerName,
		types.NamespacedName{
			Namespace: options.namespace,
			Name:      options.managerName,
		},
		store.WithGatewayEnabled(options.enableFeatureGateway),
		store.WithClusterDomain(options.clusterDomain),
	)
	if options.ngrokMetadata != "" {
		metadata := strings.TrimSuffix(options.ngrokMetadata, ",")
		// metadata is a comma separated list of key=value pairs.
		// e.g. "foo=bar,baz=qux"
		customMetadata := make(map[string]string)
		pairs := strings.Split(metadata, ",")
		for _, pair := range pairs {
			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid metadata pair: %q", pair)
			}
			customMetadata[kv[0]] = kv[1]
		}
		d.WithNgrokMetadata(customMetadata)
	}

	if err := d.Seed(ctx, mgr.GetAPIReader()); err != nil {
		return nil, fmt.Errorf("unable to seed cache store: %w", err)
	}

	d.PrintState(setupLog)

	return d, nil
}

// enableIngressFeatureSet enables the Ingress feature set for the operator
func enableIngressFeatureSet(_ context.Context, opts managerOpts, mgr ctrl.Manager, driver *store.Driver, ngrokClientset ngrokapi.Clientset) error {
	if err := (&ingresscontroller.IngressReconciler{
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

	if err := (&ingresscontroller.ServiceReconciler{
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

	if err := (&ingresscontroller.DomainReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("domain"),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("domain-controller"),
		DomainsClient: ngrokClientset.Domains(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Domain")
		os.Exit(1)
	}

	if err := (&ingresscontroller.TCPEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("tcp-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("tcp-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TCPEdge")
		os.Exit(1)
	}

	if err := (&ingresscontroller.TLSEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("tls-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("tls-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TLSEdge")
		os.Exit(1)
	}

	if err := (&ingresscontroller.HTTPSEdgeReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("https-edge"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("https-edge-controller"),
		NgrokClientset: ngrokClientset,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPSEdge")
		os.Exit(1)
	}

	if err := (&ingresscontroller.IPPolicyReconciler{
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

	if err := (&ingresscontroller.ModuleSetReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("ngrok-module-set"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ngrok-module-set-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NgrokModuleSet")
		os.Exit(1)
	}

	if err := (&ngrokcontroller.NgrokTrafficPolicyReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("traffic-policy"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("policy-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrafficPolicy")
		os.Exit(1)
	}

	return nil
}

// enableGatewayFeatureSet enables the Gateway feature set for the operator
func enableGatewayFeatureSet(_ context.Context, _ managerOpts, mgr ctrl.Manager, driver *store.Driver, _ ngrokapi.Clientset) error {
	if err := (&gatewaycontroller.GatewayReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("gateway-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Gateway")
		os.Exit(1)
	}

	if err := (&gatewaycontroller.HTTPRouteReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("gateway-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPRoute")
		os.Exit(1)
	}

	return nil
}

// enableBindingsFeatureSet enables the Bindings feature set for the operator
func enableBindingsFeatureSet(_ context.Context, opts managerOpts, mgr ctrl.Manager, _ *store.Driver, _ ngrokapi.Clientset) error {
	// EndpointBindings
	if err := (&bindingscontroller.EndpointBindingReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log.WithName("controllers").WithName("EndpointBinding"),
		Recorder:      mgr.GetEventRecorderFor("bindings-controller"),
		ClusterDomain: opts.clusterDomain,
		UpstreamServiceLabelSelector: map[string]string{
			"app.kubernetes.io/component": "bindings-forwarder",
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EndpointBinding")
		os.Exit(1)
	}

	// Create a new Runnable that implements Start that the manager can manage running
	if err := mgr.Add(&bindingscontroller.EndpointBindingPoller{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Log:             ctrl.Log.WithName("controllers").WithName("EndpointBindingPoller"),
		Recorder:        mgr.GetEventRecorderFor("endpoint-binding-poller"),
		Namespace:       opts.namespace,
		PollingInterval: 5 * time.Minute,
		// NOTE: This range must stay static for the current implementation.
		PortRange: bindingscontroller.PortRangeConfig{Min: 10000, Max: 65535},
	}); err != nil {
		return err
	}

	return nil
}

func createKubernetesOperator(ctx context.Context, client client.Client, opts managerOpts) error {
	k8sOperator := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ngrok-operator",
			Namespace: opts.namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, client, k8sOperator, func() error {
		k8sOperator.Spec = ngrokv1alpha1.KubernetesOperatorSpec{
			Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
				Name:      "ngrok-operator",
				Namespace: opts.namespace,
				Version:   version.GetVersion(),
			},
			Region: opts.region,
		}

		features := []string{}
		if opts.enableFeatureIngress {
			features = append(features, ngrokv1alpha1.KubernetesOperatorFeatureIngress)
		}

		if opts.enableFeatureGateway {
			features = append(features, ngrokv1alpha1.KubernetesOperatorFeatureGateway)
		}

		if opts.enableFeatureBindings {
			features = append(features, ngrokv1alpha1.KubernetesOperatorFeatureBindings)
			k8sOperator.Spec.Binding = &ngrokv1alpha1.KubernetesOperatorBinding{
				Name:          opts.bindings.name,
				TlsSecretName: "ngrok-operator-default-tls",
				AllowedURLs:   opts.bindings.allowedURLs,
			}
		}
		k8sOperator.Spec.EnabledFeatures = features
		return nil
	})
	return err
}
