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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	// typically only use blank imports in main
	// but we treat each of these cmd's as their own
	// "main", they are all subcommands
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/config"
	bindingscontroller "github.com/ngrok/ngrok-operator/internal/controller/bindings"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	// +kubebuilder:scaffold:imports
)

func init() {
	rootCmd.AddCommand(bindingsForwarderCmd())

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1.AddToScheme(scheme))
}

type bindingsForwarderManagerOpts struct {
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

func bindingsForwarderCmd() *cobra.Command {
	opts := bindingsForwarderManagerOpts{}

	c := &cobra.Command{
		Use: "bindings-forwarder-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return runController(c.Context(), opts)
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
	c.Flags().StringVar(&opts.managerName, "manager-name", "bindings-forwarder-manager", "Manager name to identify unique ngrok operator agent instances")

	config.RegisterLogFlags(c.Flags(), cfg)
	config.RegisterNgrokFlags(c.Flags(), cfg)
	config.RegisterFeatureFlags(c.Flags(), cfg)

	c.PreRunE = func(c *cobra.Command, _ []string) error {
		return config.ApplyEnv(c.Flags())
	}

	return c
}

func runController(_ context.Context, opts bindingsForwarderManagerOpts) error {
	zapOpts, err := config.ZapOptions(opts.cfg.Log)
	if err != nil {
		return err
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(zapOpts)))

	buildInfo := version.Get()
	setupLog.Info("starting bindings-forwarder-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	var ok bool
	opts.namespace, ok = os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}

	options := ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.namespace: {},
			},
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {Namespaces: map[string]cache.Config{cache.AllNamespaces: {}}},
			},
		},
		Metrics: server.Options{
			BindAddress: opts.metricsAddr,
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         false,
	}

	// create default config and clientset for use outside the mgr.Start() blocking loop
	k8sConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(k8sConfig, options)
	if err != nil {
		return fmt.Errorf("unable to start bindings-forwarder-manager: %w", err)
	}

	bd := bindingsdriver.New()

	certPool, err := util.LoadCerts()
	if err != nil {
		return err
	}

	// Create drain state checker - controller will use this to check if draining
	drainState := drain.NewStateChecker(mgr.GetClient(), opts.namespace, opts.releaseName)

	if err = (&bindingscontroller.ForwarderReconciler{
		Client:                 mgr.GetClient(),
		Log:                    ctrl.Log.WithName("controllers").WithName("bindings-forwarder"),
		Scheme:                 mgr.GetScheme(),
		Recorder:               mgr.GetEventRecorder("bindings-forwarder-controller"),
		BindingsDriver:         bd,
		KubernetesOperatorName: opts.releaseName,
		RootCAs:                certPool,
		DrainState:             drainState,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BindingsForwarder")
		os.Exit(1)
	}

	// register healthchecks
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	setupLog.Info("starting bindings-forwarder-manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting bindings-forwarder-manager: %w", err)
	}

	return nil
}
