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
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/ngrok/ngrok-ingress-controller/internal/controllers"
	k8sngrokcomv1 "github.com/ngrok/ngrok-ingress-controller/pkg/api/v1"
	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	log    = ctrl.Log.WithName("setup")
	scheme = runtime.NewScheme()
)

func init() {
	clientgoscheme.AddToScheme(scheme)

	utilruntime.Must(k8sngrokcomv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	if err := cmd().Execute(); err != nil {
		log.Error(err, "error running manager")
		os.Exit(1)
	}
}

type managerOpts struct {
	// flags
	metricsAddr string
	electionID  string
	probeAddr   string
	zapOpts     *zap.Options

	// env vars
	namespace   string
	ngrokAPIKey string

	region string
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
	c.Flags().StringVar(&opts.region, "region", "us", "The region to use for ngrok tunnels")
	opts.zapOpts = &zap.Options{Development: true}
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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     opts.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         opts.electionID != "",
		LeaderElectionID:       opts.electionID,
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	if err := (&controllers.IngressReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("ingress"),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorderFor("ingress-controller"),
		Namespace:      opts.namespace,
		NgrokAPIDriver: ngrokapidriver.NewNgrokAPIClient(opts.ngrokAPIKey, opts.region),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ingress controller: %w", err)
	}

	if err := (&controllers.TunnelReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("tunnel"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tunnel-controller"),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create tunnel controller: %w", err)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("error starting manager: %w", err)
	}

	return nil
}
