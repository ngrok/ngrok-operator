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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/config"
	agentcontroller "github.com/ngrok/ngrok-operator/internal/controller/agent"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/ngrok/ngrok-operator/internal/drain"
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
	utilruntime.Must(ngrokv1.AddToScheme(scheme))
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type agentManagerOpts struct {
	// deployment identity, supplied by the chart as literal args
	releaseName string
	metricsAddr string
	probeAddr   string
	managerName string

	// configPaths are the --config files, loaded before flags are registered.
	configPaths []string

	// cfg holds every app config value. Flags are registered against its
	// fields with its loaded values as their defaults, so an explicit flag
	// overrides the file and the file overrides the built-in default.
	cfg *config.Config

	// env vars
	namespace string
}

func agentCmd() *cobra.Command {
	opts := agentManagerOpts{}

	c := &cobra.Command{
		Use: "agent-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return runAgentController(c.Context(), opts)
		},
	}

	// Config must load before the remaining flags are registered so its values
	// can be used as their defaults. See config.PreParseConfigPaths.
	opts.configPaths = config.PreParseConfigPaths(os.Args[1:])

	// Registered before the load so that a malformed config file reports the
	// parse error instead of "unknown flag: --config".
	c.Flags().StringArrayVar(&opts.configPaths, config.ConfigFlag, opts.configPaths,
		"Path to a YAML config file. May be repeated; later files override earlier ones.")

	cfg, err := config.Load(opts.configPaths)
	if err != nil {
		// The rest of the flags are never registered, and the Deployment passes
		// several of them, so without this cobra fails on the first one it does
		// not know and the config error is never reached.
		c.FParseErrWhitelist.UnknownFlags = true
		// Reported through RunE so cobra prints it like any other failure.
		c.RunE = func(*cobra.Command, []string) error { return err }
		return c
	}
	opts.cfg = cfg

	c.Flags().StringVar(&opts.releaseName, "release-name", "ngrok-operator", "Helm Release name for the deployed operator")
	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.managerName, "manager-name", "agent-manager", "Manager name to identify unique ngrok operator agent instances")

	config.RegisterLogFlags(c.Flags(), cfg)
	config.RegisterNgrokFlags(c.Flags(), cfg)
	config.RegisterFeatureFlags(c.Flags(), cfg)

	// Owned by this component alone. The api-manager's equivalent for Ingress
	// resources is --ingress-watch-namespace.
	c.Flags().StringVar(&cfg.WatchNamespace, "watch-namespace", cfg.WatchNamespace, "Namespace to watch for AgentEndpoint resources. Defaults to all namespaces.")

	c.PreRunE = func(c *cobra.Command, _ []string) error {
		return config.ApplyEnv(c.Flags())
	}

	return c
}

func runAgentController(_ context.Context, opts agentManagerOpts) error {
	zapOpts, err := config.ZapOptions(opts.cfg.Log)
	if err != nil {
		return err
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(zapOpts)))

	defaultDomainReclaimPolicy, err := validateDomainReclaimPolicy(opts.cfg.Features.DefaultDomainReclaimPolicy)
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

		// The KubernetesOperator CR is a singleton owned by the operator and always
		// lives in the release namespace, regardless of `watchNamespace`. Pin its
		// cache scope to the release namespace so the drain state checker can always
		// read it, and so RBAC for it can stay narrowly scoped to the release namespace.
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&ngrokv1alpha1.KubernetesOperator{}: {
					Namespaces: map[string]cache.Config{
						opts.namespace: {},
					},
				},
			},
		}}
	if opts.cfg.WatchNamespace != "" {
		setupLog.Info("watching namespace", "namespace", opts.cfg.WatchNamespace)
		options.Cache.DefaultNamespaces = map[string]cache.Config{
			opts.cfg.WatchNamespace: {},
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
	if opts.cfg.Features.Gateway.Enabled {
		agentComments = append(agentComments, `{"gateway": "gateway-api"}`)
	}

	ad, err := agent.NewDriver(
		agent.WithAgentConnectURL(opts.cfg.Ngrok.ServerAddr),
		agent.WithAgentConnectCAs(opts.cfg.Ngrok.RootCAs),
		agent.WithLogger(ctrl.Log.WithName("drivers").WithName("agent")),
		agent.WithAgentComments(agentComments...),
	)

	if err != nil {
		return fmt.Errorf("unable to create agent driver: %w", err)
	}

	// register healthcheck for tunnel driver
	healthcheck.RegisterHealthChecker(ad)

	// Create drain state checker - controller will use this to check if draining
	drainState := drain.NewStateChecker(mgr.GetClient(), opts.namespace, opts.releaseName)

	if err = (&agentcontroller.AgentEndpointReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("agentendpoint"),
		Scheme:                     mgr.GetScheme(),
		Recorder:                   mgr.GetEventRecorder("agentendpoint-controller"),
		AgentDriver:                ad,
		DefaultDomainReclaimPolicy: defaultDomainReclaimPolicy,
		ControllerLabels:           labels.NewControllerLabelValues(opts.namespace, opts.managerName),
		DrainState:                 drainState,
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
