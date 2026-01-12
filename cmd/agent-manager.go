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
	"net/http"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	// typically only use blank imports in main
	// but we treat each of these cmd's as their own
	// "main", they are all subcommands
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	agentcontroller "github.com/ngrok/ngrok-operator/internal/controller/agent"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/ngrok/ngrok-operator/internal/healthcheck"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	// +kubebuilder:scaffold:imports
)

func init() {
	rootCmd.AddCommand(agentCmd())

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type agentManagerOpts struct {
	// flags
	metricsAddr    string
	probeAddr      string
	serverAddr     string
	description    string
	managerName    string
	watchNamespace string
	zapOpts        *zap.Options

	// feature flags
	enableFeatureIngress          bool
	enableFeatureGateway          bool
	enableFeatureBindings         bool
	disableGatewayReferenceGrants bool

	// agent(tunnel driver) flags
	region  string
	rootCAs string

	defaultDomainReclaimPolicy string

	// env vars
	namespace string
}

func agentCmd() *cobra.Command {
	var opts agentManagerOpts
	c := &cobra.Command{
		Use: "agent-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return runAgentController(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.description, "description", "Created by the ngrok-operator", "Description for this installation")
	// TODO(operator-rename): Same as above, but for the manager name.
	c.Flags().StringVar(&opts.managerName, "manager-name", "agent-manager", "Manager name to identify unique ngrok operator agent instances")
	c.Flags().StringVar(&opts.watchNamespace, "watch-namespace", "", "Namespace to watch for AgentEndpoint resources. Defaults to all namespaces.")

	// agent(tunnel driver) flags
	c.Flags().StringVar(&opts.region, "region", "", "The region to use for ngrok tunnels")
	c.Flags().StringVar(&opts.serverAddr, "server-addr", "", "The address of the ngrok server to use for tunnels")
	c.Flags().StringVar(&opts.rootCAs, "root-cas", "trusted", "trusted (default) or host: use the trusted ngrok agent CA or the host CA")

	// feature flags
	c.Flags().BoolVar(&opts.enableFeatureIngress, "enable-feature-ingress", true, "Enables the Ingress controller")
	c.Flags().BoolVar(&opts.enableFeatureGateway, "enable-feature-gateway", true, "When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled")
	c.Flags().BoolVar(&opts.disableGatewayReferenceGrants, "disable-reference-grants", false, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")
	c.Flags().BoolVar(&opts.enableFeatureBindings, "enable-feature-bindings", false, "Enables the Endpoint Bindings controller")

	c.Flags().StringVar(&opts.defaultDomainReclaimPolicy, "default-domain-reclaim-policy", string(ingressv1alpha1.DomainReclaimPolicyDelete), "The default domain reclaim policy to apply to created domains")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

func runAgentController(_ context.Context, opts agentManagerOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))

	defaultDomainReclaimPolicy, err := validateDomainReclaimPolicy(opts.defaultDomainReclaimPolicy)
	if err != nil {
		return err
	}

	buildInfo := version.Get()
	setupLog.Info("starting agent-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	var ok bool
	opts.namespace, ok = os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: opts.metricsAddr,
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         false,
	}

	if opts.watchNamespace != "" {
		setupLog.Info("watching namespace", "namespace", opts.watchNamespace)
		options.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.watchNamespace: {},
			},
		}
	}

	// create default config and clientset for use outside the mgr.Start() blocking loop
	k8sConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(k8sConfig, options)
	if err != nil {
		return fmt.Errorf("unable to start agent-manager: %w", err)
	}

	// shared features between Ingress and Gateway (tunnels)
	agentComments := []string{}
	if opts.enableFeatureGateway {
		agentComments = append(agentComments, `{"gateway": "gateway-api"}`)
	}

	rootCAs := "trusted"
	if opts.rootCAs != "" {
		rootCAs = opts.rootCAs
	}

	ad, err := agent.NewDriver(
		agent.WithAgentConnectURL(opts.serverAddr),
		agent.WithAgentConnectCAs(rootCAs),
		agent.WithLogger(ctrl.Log.WithName("drivers").WithName("agent")),
		agent.WithAgentComments(agentComments...),
	)

	if err != nil {
		return fmt.Errorf("unable to create agent driver: %w", err)
	}

	// register healthcheck for tunnel driver
	healthcheck.RegisterHealthChecker(ad)

	if err = (&agentcontroller.AgentEndpointReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("agentendpoint"),
		Scheme:                     mgr.GetScheme(),
		Recorder:                   mgr.GetEventRecorderFor("agentendpoint-controller"),
		AgentDriver:                ad,
		DefaultDomainReclaimPolicy: defaultDomainReclaimPolicy,
		ControllerLabels:           labels.NewControllerLabelValues(opts.namespace, opts.managerName),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AgentEndpoint")
		os.Exit(1)
	}

	// register healthchecks
	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		return healthcheck.Ready(req.Context(), req)
	}); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		return healthcheck.Alive(req.Context(), req)
	}); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	setupLog.Info("starting agent-manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting agent-manager: %w", err)
	}

	return nil
}
