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
	"log/slog"
	"os"
	"strconv"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	// typically only use blank imports in main
	// but we treat each of these cmd's as their own
	// "main", they are all subcommands
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/cobra"
	"golang.ngrok.com/ngrok/privatedial"
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
	releaseName           string
	metricsAddr           string
	probeAddr             string
	description           string
	managerName           string
	usePrivateDial        bool
	privateDialConnectURL string
	zapOpts               *zap.Options

	// env vars
	namespace      string
	privateDialPAT string
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
	c.Flags().BoolVar(&opts.usePrivateDial, "use-private-dial", envBool("USE_PRIVATE_DIAL"), "Reach endpoints over private dial instead of the mTLS bindings ingress")
	c.Flags().StringVar(&opts.privateDialConnectURL, "private-dial-connect-url", "h2.connect-endpoint.ngrok.com:443", "host:port of the private-dial connect ingress")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)

	return c
}

// envBool is the default for a boolean flag that a Helm chart sets through the
// environment. An unparseable value is false, the same as unset.
func envBool(name string) bool {
	v, _ := strconv.ParseBool(os.Getenv(name))
	return v
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

	// Private dial authenticates the whole session with a Personal Access
	// Token, and the account it belongs to is what resolves the endpoint. This
	// is not the agent authtoken that serves the endpoint: the private-dial
	// handler rejects any credential that is not a PAT.
	var privateDialer bindingscontroller.PrivateDialer
	if opts.usePrivateDial {
		opts.privateDialPAT, ok = os.LookupEnv("NGROK_PRIVATE_DIAL_PAT")
		if !ok || opts.privateDialPAT == "" {
			return errors.New("NGROK_PRIVATE_DIAL_PAT environment variable should be set when private dial is enabled, but was not")
		}
		if opts.privateDialConnectURL == "" {
			return errors.New("--private-dial-connect-url is required when private dial is enabled")
		}

		dialer := privatedial.New(privatedial.Config{
			H2ServerAddr: opts.privateDialConnectURL,
			// The forwarder runs wherever the cluster runs, and UDP egress is
			// not something we can assume, so skip the QUIC race.
			ForceProtocol: privatedial.ProtocolH2,
			AuthToken:     opts.privateDialPAT,
			ClientVersion: "ngrok-operator/" + buildInfo.Version,
			Logger:        slog.Default(),
		})
		defer dialer.Close()
		privateDialer = dialer

		setupLog.Info("private dial enabled", "connectURL", opts.privateDialConnectURL)
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
		PrivateDialer:          privateDialer,
		DrainState:             drainState,
	}).SetupWithManager(mgr); err != nil {
		// Return rather than os.Exit, so the deferred dialer Close still runs.
		return fmt.Errorf("unable to create the BindingsForwarder controller: %w", err)
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
