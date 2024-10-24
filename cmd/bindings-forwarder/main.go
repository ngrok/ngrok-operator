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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	bindingscontroller "github.com/ngrok/ngrok-operator/internal/controller/bindings"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(bindingsv1alpha1.AddToScheme(scheme))
}

func main() {
	if err := cmd().Execute(); err != nil {
		setupLog.Error(err, "error running bindings-forwarder-manager")
		os.Exit(1)
	}
}

type managerOpts struct {
	// flags
	metricsAddr string
	probeAddr   string
	description string
	managerName string
	zapOpts     *zap.Options

	// env vars
	namespace string
}

func cmd() *cobra.Command {
	var opts managerOpts
	c := &cobra.Command{
		Use: "bindings-forwarder-manager",
		RunE: func(c *cobra.Command, args []string) error {
			return runController(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.description, "description", "Created by the ngrok-operator", "Description for this installation")
	c.Flags().StringVar(&opts.managerName, "manager-name", "bindings-forwarder-manager", "Manager name to identify unique ngrok operator agent instances")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

func runController(_ context.Context, opts managerOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))

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

	if err = (&bindingscontroller.ForwarderReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("bindings-forwarder"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("bindings-forwarder-controller"),
		BindingsDriver: bd,
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
