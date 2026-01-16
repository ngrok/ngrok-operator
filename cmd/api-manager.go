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

package cmd

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

	"k8s.io/client-go/discovery"
	// typically only use blank imports in main
	// but we treat each of these cmd's as their own
	// "main", they are all subcommands
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	"github.com/spf13/cobra"
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
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-api-go/v7/api_keys"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	bindingscontroller "github.com/ngrok/ngrok-operator/internal/controller/bindings"
	gatewaycontroller "github.com/ngrok/ngrok-operator/internal/controller/gateway"
	ingresscontroller "github.com/ngrok/ngrok-operator/internal/controller/ingress"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	ngrokcontroller "github.com/ngrok/ngrok-operator/internal/controller/ngrok"
	servicecontroller "github.com/ngrok/ngrok-operator/internal/controller/service"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// +kubebuilder:scaffold:imports
)

func init() {
	rootCmd.AddCommand(apiCmd())

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1beta1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type apiManagerOpts struct {
	// flags
	releaseName           string
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

	// when true, ngrok-op will allow required fields to be optional
	// then it will go Ready and log errors about registration state due to missing required fields
	// this is useful for marketplace installations where our users do not have a chance to add their required configuration
	// yet we still want a 1-click install to work
	//
	// when false, ngrok-op will require all required fields to be present before going Ready
	// and will log errors about missing required fields
	oneClickDemoMode bool

	// feature flags
	enableFeatureIngress          bool
	enableFeatureGateway          bool
	enableFeatureBindings         bool
	disableGatewayReferenceGrants bool

	bindings struct {
		endpointSelectors  []string
		serviceAnnotations string
		serviceLabels      string
		ingressEndpoint    string
	}

	// env vars
	namespace   string
	ngrokAPIKey string

	region string

	defaultDomainReclaimPolicy string
	drainPolicy                ngrokv1alpha1.DrainPolicy
}

func apiCmd() *cobra.Command {
	var opts apiManagerOpts
	c := &cobra.Command{
		Use: "api-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return startOperator(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.releaseName, "release-name", "ngrok-operator", "Helm Release name for the deployed operator")
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
	c.Flags().StringVar(&opts.clusterDomain, "cluster-domain", common.DefaultClusterDomain, "Cluster domain used in the cluster")
	c.Flags().BoolVar(&opts.oneClickDemoMode, "one-click-demo-mode", false, "Run the operator in one-click-demo mode (Ready, but not running)")

	// feature flags
	c.Flags().BoolVar(&opts.enableFeatureIngress, "enable-feature-ingress", true, "Enables the Ingress controller")
	c.Flags().BoolVar(&opts.enableFeatureGateway, "enable-feature-gateway", true, "When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled")
	c.Flags().BoolVar(&opts.disableGatewayReferenceGrants, "disable-reference-grants", false, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")
	c.Flags().BoolVar(&opts.enableFeatureBindings, "enable-feature-bindings", false, "Enables the Endpoint Bindings controller")
	c.Flags().StringSliceVar(&opts.bindings.endpointSelectors, "bindings-endpoint-selectors", []string{"true"}, "Endpoint Selectors for Endpoint Bindings")
	c.Flags().StringVar(&opts.bindings.serviceAnnotations, "bindings-service-annotations", "", "Service Annotations to propagate to the target service")
	c.Flags().StringVar(&opts.bindings.serviceLabels, "bindings-service-labels", "", "Service Labels to propagate to the target service")
	c.Flags().StringVar(&opts.bindings.ingressEndpoint, "bindings-ingress-endpoint", "", "The endpoint the bindings forwarder connects to")
	c.Flags().StringVar(&opts.defaultDomainReclaimPolicy, "default-domain-reclaim-policy", string(ingressv1alpha1.DomainReclaimPolicyDelete), "The default domain reclaim policy to apply to created domains")
	c.Flags().StringVar((*string)(&opts.drainPolicy), "drain-policy", string(ngrokv1alpha1.DrainPolicyRetain), "Policy for draining resources during uninstall: Delete or Retain")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

// startOperator starts the ngrok-op
func startOperator(ctx context.Context, opts apiManagerOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))

	buildInfo := version.Get()
	setupLog.Info("starting api-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	// create default kubernetes config and clientset
	k8sConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("unable to create k8s client: %w", err)
	}

	tlsRouteCRDInstalled := false
	tcpRouteCRDInstalled := false
	// Unless we are fully opting-out of GWAPI support, check if the CRDs are installed. If not, disable GWAPI support
	if opts.enableFeatureGateway {
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(k8sConfig)
		if err != nil {
			return fmt.Errorf("unable to create discovery client: %w", err)
		}

		apiGroupList, err := discoveryClient.ServerGroups()
		if err != nil {
			return fmt.Errorf("unable to list server groups: %w", err)
		}

		gatewayAPIGroupInstalled := false
		for _, group := range apiGroupList.Groups {
			if group.Name == "gateway.networking.k8s.io" {
				gatewayAPIGroupInstalled = true
				break
			}
		}
		if !gatewayAPIGroupInstalled {
			setupLog.Info("Gateway API CRDs not detected, Gateway feature set will be disabled")
			opts.enableFeatureGateway = false
		} else {
			// Check for optional TLSRoute/TCPRoute CRDs. They are in the experimental channel but not the standard channel, so depending on
			// which set of the Gateway API CRDs the user installed, we may or may not need to enable support for them.
			resourceList, err := discoveryClient.ServerResourcesForGroupVersion("gateway.networking.k8s.io/v1alpha2")
			if err != nil {
				setupLog.Error(err, "unable to check if TLSRoute/TCPRoute CRDs are installed, support for them will not be enabled")
			} else {
				for _, r := range resourceList.APIResources {
					if strings.EqualFold(r.Name, "TLSRoutes") {
						tlsRouteCRDInstalled = true
						continue
					}
					if strings.EqualFold(r.Name, "TCPRoutes") {
						tcpRouteCRDInstalled = true
						continue
					}
					// If we found both, no need to check other resources
					if tcpRouteCRDInstalled && tlsRouteCRDInstalled {
						break
					}
				}
			}

			if tcpRouteCRDInstalled {
				setupLog.Info("TCPRoute CRD detected, enabling TCPRoute support")
			} else {
				setupLog.Info("TCPRoute CRD not detected, disabling TCPRoute support. If you would like to use TCPRoute, make sure they are installed using the experimental CRD channel when installing the Gateway API CRDs")
			}

			if tlsRouteCRDInstalled {
				setupLog.Info("TLSRoute CRD detected, enabling TLSRoute support")
			} else {
				setupLog.Info("TLSRoute CRD not detected, disabling TLSRoute support. If you would like to use TLSRoutes, make sure they are installed using the experimental CRD channel when installing the Gateway API CRDs")
			}

		}
	}

	var ok bool
	opts.namespace, ok = os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}

	mgr, err := loadManager(k8sConfig, opts)
	if err != nil {
		return fmt.Errorf("unable to load manager: %w", err)
	}

	if opts.oneClickDemoMode {
		return runOneClickDemoMode(ctx, mgr)
	}

	return runNormalMode(ctx, opts, k8sClient, mgr, tcpRouteCRDInstalled, tlsRouteCRDInstalled)
}

// runOneClickDemoMode runs the operator in a one-click demo mode, meaning:
// - the operator will start even if required fields are missing
// - the operator will log errors about missing required fields
// - the operator will go Ready and log errors about registration state due to missing required fields
func runOneClickDemoMode(ctx context.Context, mgr ctrl.Manager) error {
	// register healthchecks
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	// start a ticker to print demo log messages
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ctx.Done():
				break
			case <-ticker.C:
				setupLog.Error(errors.New("Running in one-click-demo mode"), "Ready even if required fields are missing!")
				setupLog.Info("The ngrok-operator is running in one-click-demo mode which means the operator is not actually reconciling resources.")
				setupLog.Info("Please provide ngrok API key and ngrok Authtoken in your Helm values to run the operator for real.")
				setupLog.Info("Please set `oneClickDemoMode: false` in your Helm values to run the operator for real.")
			}
		}
	}()

	setupLog.Info("starting api-manager in one-click-demo mode")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting api-manager: %w", err)
	}

	return nil
}

// runNormalMode runs the operator in normal operation mode
func runNormalMode(ctx context.Context, opts apiManagerOpts, k8sClient client.Client, mgr ctrl.Manager, tcpRouteCRDInstalled, tlsRouteCRDInstalled bool) error {
	defaultDomainReclaimPolicy, err := validateDomainReclaimPolicy(opts.defaultDomainReclaimPolicy)
	if err != nil {
		return err
	}

	ngrokClientset, err := loadNgrokClientset(ctx, opts)
	if err != nil {
		return fmt.Errorf("Unable to load ngrokClientSet: %w", err)
	}

	// Create drain state checker - all controllers will use this to check if draining
	drainState := drain.NewStateChecker(mgr.GetClient(), opts.namespace, opts.releaseName)

	// register the k8sop in the ngrok API
	if err := createKubernetesOperator(ctx, k8sClient, opts); err != nil {
		return fmt.Errorf("unable to create KubernetesOperator: %w", err)
	}

	// k8sResourceDriver is the driver that will be used to interact with the k8s resources for all controllers
	// but primarily for kinds Ingress, Gateway, and ngrok CRDs
	var k8sResourceDriver *managerdriver.Driver
	if opts.enableFeatureIngress || opts.enableFeatureGateway {
		// we only need a driver if these features are enabled
		k8sResourceDriver, err = getK8sResourceDriver(ctx, mgr, opts, tcpRouteCRDInstalled, tlsRouteCRDInstalled, *defaultDomainReclaimPolicy, drainState)
		if err != nil {
			return fmt.Errorf("unable to create Driver: %w", err)
		}
	}

	if opts.enableFeatureIngress {
		setupLog.Info("Ingress feature set enabled")
		if err := enableIngressFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset, *defaultDomainReclaimPolicy, drainState); err != nil {
			return fmt.Errorf("unable to enable Ingress feature set: %w", err)
		}
	} else {
		setupLog.Info("Ingress feature set disabled")
	}

	if opts.enableFeatureGateway {
		setupLog.Info("Gateway feature set enabled")
		if err := enableGatewayFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset, tcpRouteCRDInstalled, tlsRouteCRDInstalled, drainState); err != nil {
			return fmt.Errorf("unable to enable Gateway feature set: %w", err)
		}

		if opts.disableGatewayReferenceGrants {
			setupLog.Info("Opting out of requiring ReferenceGrants in Gateway API config for cross namespace references")
		} else {
			setupLog.Info("ReferenceGrants will be required for cross namespace references in GatewayAPI Config")
		}
	} else {
		setupLog.Info("Gateway feature set disabled")
	}

	if opts.enableFeatureBindings {
		setupLog.Info("Endpoint Bindings feature set enabled")
		if err := enableBindingsFeatureSet(ctx, opts, mgr, k8sResourceDriver, ngrokClientset, drainState); err != nil {
			return fmt.Errorf("unable to enable Bindings feature set: %w", err)
		}
	} else {
		setupLog.Info("Endpoint Bindings feature set disabled")
	}

	// new kubebuilder controllers will be generated here
	// please attach these to a feature set
	// +kubebuilder:scaffold:builder

	// Always register the ngrok KubernetesOperator controller. It is independent of the feature set.
	if err := (&ngrokcontroller.KubernetesOperatorReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("KubernetesOperator"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("kubernetes-operator-controller"),
		Namespace:      opts.namespace,
		ControllerName: opts.managerName,
		NgrokClientset: ngrokClientset,
		DrainState:     drainState,
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

	setupLog.Info("starting api-manager in normal mode")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting api-manager: %w", err)
	}

	return nil
}

// loadManager loads the controller-runtime manager with the provided options
func loadManager(k8sConfig *rest.Config, opts apiManagerOpts) (manager.Manager, error) {
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

	mgr, err := ctrl.NewManager(k8sConfig, options)
	if err != nil {
		return nil, fmt.Errorf("unable to start api-manager: %w", err)
	}

	return mgr, nil
}

// loadNgrokClientset loads the ngrok API clientset from the environment and managerOpts
func loadNgrokClientset(ctx context.Context, opts apiManagerOpts) (ngrokapi.Clientset, error) {
	var ok bool
	opts.ngrokAPIKey, ok = os.LookupEnv("NGROK_API_KEY")
	if !ok {
		return nil, errors.New("NGROK_API_KEY environment variable should be set, but was not")
	}

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

	// validate the API key and Authtoken works with ngrok API
	// by making a dummy request to list API keys
	// and checking for errors
	cApiKeys := api_keys.NewClient(ngrokClientConfig)
	cIter := cApiKeys.List(&ngrok.Paging{Limit: ptr.To("1")})
	cIter.Next(ctx)
	if cIter.Err() != nil {
		return nil, fmt.Errorf("Unable to verify API Key: %w", cIter.Err())
	}

	ngrokClientset := ngrokapi.NewClientSet(ngrokClientConfig)
	return ngrokClientset, nil
}

// getK8sResourceDriver returns a new Driver instance that is seeded with the current state of the cluster.
func getK8sResourceDriver(ctx context.Context, mgr manager.Manager, options apiManagerOpts, tcpRouteCRDInstalled, tlsRouteCRDInstalled bool, defaultDomainReclaimPolicy ingressv1alpha1.DomainReclaimPolicy, drainState managerdriver.DrainState) (*managerdriver.Driver, error) {
	logger := mgr.GetLogger().WithName("cache-store-driver")

	driverOpts := []managerdriver.DriverOpt{
		managerdriver.WithGatewayEnabled(options.enableFeatureGateway),
		managerdriver.WithClusterDomain(options.clusterDomain),
		managerdriver.WithDisableGatewayReferenceGrants(options.disableGatewayReferenceGrants),
		managerdriver.WithDefaultDomainReclaimPolicy(defaultDomainReclaimPolicy),
		managerdriver.WithEventRecorder(mgr.GetEventRecorderFor("k8s-resource-driver")),
		managerdriver.WithDrainState(drainState),
	}

	if tcpRouteCRDInstalled {
		driverOpts = append(driverOpts, managerdriver.WithGatewayTCPRouteEnabled(true))
	}

	if tlsRouteCRDInstalled {
		driverOpts = append(driverOpts, managerdriver.WithGatewayTLSRouteEnabled(true))
	}

	d := managerdriver.NewDriver(
		logger,
		mgr.GetScheme(),
		options.ingressControllerName,
		types.NamespacedName{
			Namespace: options.namespace,
			Name:      options.managerName,
		},
		driverOpts...,
	)
	if options.ngrokMetadata != "" {
		customMetadata, err := util.ParseHelmDictionary(options.ngrokMetadata)
		if err != nil {
			return nil, fmt.Errorf("unable to parse ngrokMetadata: %w", err)
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
func enableIngressFeatureSet(_ context.Context, opts apiManagerOpts, mgr ctrl.Manager, driver *managerdriver.Driver, ngrokClientset ngrokapi.Clientset, defaultDomainReclaimPolicy ingressv1alpha1.DomainReclaimPolicy, drainState controller.DrainState) error {
	controllerLabels := labels.NewControllerLabelValues(opts.namespace, opts.managerName)

	if err := (&ingresscontroller.IngressReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("ingress"),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("ingress-controller"),
		Namespace: opts.namespace,
		Driver:    driver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ingress controller: %w", err)
	}

	if err := (&servicecontroller.ServiceReconciler{
		Client:           mgr.GetClient(),
		Log:              ctrl.Log.WithName("controllers").WithName("service"),
		Scheme:           mgr.GetScheme(),
		Recorder:         mgr.GetEventRecorderFor("service-controller"),
		ControllerLabels: controllerLabels,
		ClusterDomain:    opts.clusterDomain,
		// TODO(stacks): Once we have a way to support unqualified tcp addresses(i.e. 'tcp://') in the Cloud & Agent Endpoint CRs,
		// we can remove this. It feels weird to have this here since the ServiceReconciler should only be performing translations
		// and not dependent on the ngrok API.
		TCPAddresses: ngrokClientset.TCPAddresses(),
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
		DrainState:    drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Domain")
		os.Exit(1)
	}

	if err := (&ingresscontroller.IPPolicyReconciler{
		Client:              mgr.GetClient(),
		Log:                 ctrl.Log.WithName("controllers").WithName("ip-policy"),
		Scheme:              mgr.GetScheme(),
		Recorder:            mgr.GetEventRecorderFor("ip-policy-controller"),
		IPPoliciesClient:    ngrokClientset.IPPolicies(),
		IPPolicyRulesClient: ngrokClientset.IPPolicyRules(),
		DrainState:          drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPPolicy")
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

	if err := (&ngrokcontroller.CloudEndpointReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("cloud-endpoint"),
		Scheme:                     mgr.GetScheme(),
		Recorder:                   mgr.GetEventRecorderFor("cloud-endpoint-controller"),
		NgrokClientset:             ngrokClientset,
		DefaultDomainReclaimPolicy: ptr.To(defaultDomainReclaimPolicy),
		ControllerLabels:           controllerLabels,
		DrainState:                 drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudEndpoint")
		os.Exit(1)
	}

	return nil
}

// enableGatewayFeatureSet enables the Gateway feature set for the operator
func enableGatewayFeatureSet(_ context.Context, opts apiManagerOpts, mgr ctrl.Manager, driver *managerdriver.Driver, _ ngrokapi.Clientset, tcpRouteCRDInstalled, tlsRouteCRDInstalled bool, drainState controller.DrainState) error {
	if err := (&gatewaycontroller.GatewayClassReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("GatewayClass"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("gateway-class"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayClass")
		os.Exit(1)
	}

	if err := (&gatewaycontroller.GatewayReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("Gateway"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("gateway-controller"),
		Driver:     driver,
		DrainState: drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Gateway")
		os.Exit(1)
	}

	if err := (&gatewaycontroller.HTTPRouteReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("Gateway"),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("gateway-controller"),
		Driver:     driver,
		DrainState: drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPRoute")
		os.Exit(1)
	}

	if tcpRouteCRDInstalled {
		if err := (&gatewaycontroller.TCPRouteReconciler{
			Client:     mgr.GetClient(),
			Log:        ctrl.Log.WithName("controllers").WithName("TCPRoute"),
			Scheme:     mgr.GetScheme(),
			Recorder:   mgr.GetEventRecorderFor("tcp-route"),
			Driver:     driver,
			DrainState: drainState,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "TCPRoute")
			os.Exit(1)
		}
	}

	if tlsRouteCRDInstalled {
		if err := (&gatewaycontroller.TLSRouteReconciler{
			Client:     mgr.GetClient(),
			Log:        ctrl.Log.WithName("controllers").WithName("TLSRoute"),
			Scheme:     mgr.GetScheme(),
			Recorder:   mgr.GetEventRecorderFor("tls-route"),
			Driver:     driver,
			DrainState: drainState,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "TLSRoute")
			os.Exit(1)
		}
	}

	// Even if we aren't using ReferenceGrants, watch namespaces for Gateway.Listeners.AllowedRoutes.Namespaces
	if err := (&gatewaycontroller.NamespaceReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("gateway-controller"),
		Driver:   driver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Namespace")
		os.Exit(1)
	}

	// Start a controller for ReferenceGrants unless they are disabled
	if !opts.disableGatewayReferenceGrants {
		if err := (&gatewaycontroller.ReferenceGrantReconciler{
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("controllers").WithName("Gateway"),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("gateway-controller"),
			Driver:   driver,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ReferenceGrant")
			os.Exit(1)
		}
	}

	return nil
}

// enableBindingsFeatureSet enables the Bindings feature set for the operator
func enableBindingsFeatureSet(_ context.Context, opts apiManagerOpts, mgr ctrl.Manager, _ *managerdriver.Driver, ngrokClientset ngrokapi.Clientset, drainState *drain.StateChecker) error {
	targetServiceAnnotations, err := util.ParseHelmDictionary(opts.bindings.serviceAnnotations)
	if err != nil {
		setupLog.WithValues("serviceAnnotations", opts.bindings.serviceAnnotations).Error(err, "unable to parse service annotations")
		targetServiceAnnotations = make(map[string]string)
	}

	targetServiceLabels, err := util.ParseHelmDictionary(opts.bindings.serviceLabels)
	if err != nil {
		setupLog.WithValues("serviceLabels", opts.bindings.serviceLabels).Error(err, "unable to parse service labels")
		targetServiceLabels = make(map[string]string)
	}

	// BoundEndpoints
	if err := (&bindingscontroller.BoundEndpointReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log.WithName("controllers").WithName("BoundEndpoint"),
		Recorder:      mgr.GetEventRecorderFor("bindings-controller"),
		ClusterDomain: opts.clusterDomain,
		UpstreamServiceLabelSelector: map[string]string{
			"app.kubernetes.io/component": "bindings-forwarder",
		},
		RefreshDuration: time.Minute * 10,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BoundEndpoint")
		os.Exit(1)
	}

	// Create a new Runnable that implements Start that the manager can manage running
	if err := mgr.Add(&bindingscontroller.BoundEndpointPoller{
		Client:                       mgr.GetClient(),
		Log:                          ctrl.Log.WithName("controllers").WithName("BoundEndpointPoller"),
		Recorder:                     mgr.GetEventRecorderFor("endpoint-binding-poller"),
		Namespace:                    opts.namespace,
		KubernetesOperatorConfigName: opts.releaseName,
		TargetServiceAnnotations:     targetServiceAnnotations,
		TargetServiceLabels:          targetServiceLabels,
		PollingInterval:              10 * time.Second,
		NgrokClientset:               ngrokClientset,
		DrainState:                   drainState,
		// NOTE: This range must stay static for the current implementation.
		PortRange: bindingscontroller.PortRangeConfig{Min: 10000, Max: 65535},
	}); err != nil {
		return err
	}

	return nil
}

func createKubernetesOperator(ctx context.Context, client client.Client, opts apiManagerOpts) error {
	k8sOperator := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.releaseName,
			Namespace: opts.namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, client, k8sOperator, func() error {
		k8sOperator.Spec.Description = opts.description
		k8sOperator.Spec.Deployment = &ngrokv1alpha1.KubernetesOperatorDeployment{
			Name:      opts.releaseName,
			Namespace: opts.namespace,
			Version:   version.GetVersion(),
		}
		k8sOperator.Spec.Region = opts.region

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
				TlsSecretName:     "ngrok-operator-default-tls",
				EndpointSelectors: opts.bindings.endpointSelectors,
			}
			if opts.bindings.ingressEndpoint != "" {
				k8sOperator.Spec.Binding.IngressEndpoint = &opts.bindings.ingressEndpoint
			}
		}
		k8sOperator.Spec.EnabledFeatures = features

		// Set drain policy but preserve Enabled if already set (during drain)
		if k8sOperator.Spec.Drain == nil {
			k8sOperator.Spec.Drain = &ngrokv1alpha1.DrainConfig{}
		}
		k8sOperator.Spec.Drain.Policy = opts.drainPolicy

		setupLog.Info("created KubernetesOperator", "name", k8sOperator.Name, "namespace", k8sOperator.Namespace, "op", fmt.Sprintf("%+v", k8sOperator.Spec.Binding))
		return nil
	})
	return err
}
