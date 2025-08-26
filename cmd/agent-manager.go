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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/config"
	agentcontroller "github.com/ngrok/ngrok-operator/internal/controller/agent"
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

func agentCmd() *cobra.Command {
	var configPath string
	c := &cobra.Command{
		Use: "agent-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return runAgentController(c.Context(), configPath)
		},
	}

	c.Flags().StringVar(&configPath, "config", "", "Path to configuration directory")

	return c
}

func runAgentController(_ context.Context, configPath string) error {
	buildInfo := version.Get()

	// Load and validate configuration from config file
	operatorConfig, err := config.LoadAndValidateConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Set up logging
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(operatorConfig.GetZapOptions())))
	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("starting agent-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	defaultDomainReclaimPolicy := config.GetDomainReclaimPolicy(operatorConfig.API.DefaultDomainReclaimPolicy)

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: operatorConfig.MetricsBindAddress,
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: operatorConfig.HealthProbeBindAddress,
		LeaderElection:         false,
	}

	// create default config and clientset for use outside the mgr.Start() blocking loop
	k8sConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(k8sConfig, options)
	if err != nil {
		return fmt.Errorf("unable to start agent-manager: %w", err)
	}

	// shared features between Ingress and Gateway (tunnels)
	agentComments := []string{}
	if operatorConfig.EnableFeatureGateway {
		agentComments = append(agentComments, `{"gateway": "gateway-api"}`)
	}

	ad, err := agent.NewDriver(
		agent.WithAgentConnectURL(operatorConfig.ServerAddr),
		agent.WithAgentConnectCAs(operatorConfig.RootCAs),
		agent.WithLogger(ctrl.Log.WithName("drivers").WithName("agent")),
		agent.WithAgentComments(agentComments...),
	)

	if err != nil {
		return fmt.Errorf("unable to create agent driver: %w", err)
	}

	// register healthcheck for agent driver
	healthcheck.RegisterHealthChecker(ad)

	if err = (&agentcontroller.AgentEndpointReconciler{
		Client:                     mgr.GetClient(),
		Log:                        ctrl.Log.WithName("controllers").WithName("agentendpoint"),
		Scheme:                     mgr.GetScheme(),
		Recorder:                   mgr.GetEventRecorderFor("agentendpoint-controller"),
		AgentDriver:                ad,
		DefaultDomainReclaimPolicy: defaultDomainReclaimPolicy,
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
