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
	"flag"
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
	bindingscontroller "github.com/ngrok/ngrok-operator/internal/controller/bindings"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
	"golang.ngrok.com/ngrok/privatedial"
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
	// flags
	releaseName string
	metricsAddr string
	probeAddr   string
	description string
	managerName string
	zapOpts     *zap.Options

	// Private-dial POC (K8SOP-292, see specs/private-dial/08-poc-build-guide.md).
	// Gated behind usePrivateDial so the shipping mTLS egress leg stays intact.
	usePrivateDial    bool
	privateDialServer string

	// env vars
	namespace string
}

func bindingsForwarderCmd() *cobra.Command {
	var opts bindingsForwarderManagerOpts
	c := &cobra.Command{
		Use: "bindings-forwarder-manager",
		RunE: func(c *cobra.Command, _ []string) error {
			return runController(c.Context(), opts)
		},
	}

	c.Flags().StringVar(&opts.releaseName, "release-name", "ngrok-operator", "Helm Release name for the deployed operator")
	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	c.Flags().StringVar(&opts.description, "description", "Created by the ngrok-operator", "Description for this installation")
	c.Flags().StringVar(&opts.managerName, "manager-name", "bindings-forwarder-manager", "Manager name to identify unique ngrok operator agent instances")

	c.Flags().BoolVar(&opts.usePrivateDial, "use-private-dial", false,
		"POC (K8SOP-292): dial bound endpoints via private-dial instead of mTLS + UpgradeToBindingConnection. Also settable via USE_PRIVATE_DIAL=true. Requires NGROK_PRIVATE_DIAL_PAT (a PAT, ngrok_pat_*) in the environment.")
	c.Flags().StringVar(&opts.privateDialServer, "private-dial-server", "quic.connect-endpoint.ngrok.com:443",
		"Private-dial gateway address (host:port) for the QUIC transport. Only used with --use-private-dial. "+
			"QUIC only, not a typo: golang.ngrok.com/ngrok/privatedial's HTTP/2 transport panics on Go 1.27 "+
			"(newH2Transport calls http2.Transport.NewClientConn without initializing it, which Go 1.27's "+
			"native net/http HTTP/2 wrapper in golang.org/x/net requires) — this operator builds with Go 1.27, "+
			"so the forwarder forces privatedial.ProtocolQUIC to avoid crashing the pod. Fix upstream before "+
			"ever wiring in ProtocolH2 or ProtocolAuto here.")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

func runController(_ context.Context, opts bindingsForwarderManagerOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))

	buildInfo := version.Get()
	setupLog.Info("starting bindings-forwarder-manager", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	var ok bool
	opts.namespace, ok = os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}

	if !opts.usePrivateDial && os.Getenv("USE_PRIVATE_DIAL") == "true" {
		opts.usePrivateDial = true
	}
	var privateDialer *privatedial.Dialer
	if opts.usePrivateDial {
		pat := os.Getenv("NGROK_PRIVATE_DIAL_PAT")
		if pat == "" {
			return errors.New("--use-private-dial is set but NGROK_PRIVATE_DIAL_PAT environment variable was not")
		}
		privateDialer = privatedial.New(privatedial.Config{
			QUICServerAddr: opts.privateDialServer,
			ForceProtocol:  privatedial.ProtocolQUIC, // see the --private-dial-server flag help for why
			AuthToken:      pat,
		})
		setupLog.Info("private-dial POC enabled", "server", opts.privateDialServer, "protocol", "quic")
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
		UsePrivateDial:         opts.usePrivateDial,
		PrivateDialer:          privateDialer,
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
