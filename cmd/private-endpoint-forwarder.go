/*
Copyright 2026.

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
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.ngrok.com/ngrok/privatedial"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/internal/version"
)

func init() {
	rootCmd.AddCommand(privateEndpointForwarderCmd())
}

type privateEndpointForwarderOpts struct {
	metricsAddr       string
	probeAddr         string
	privateDialServer string
	zapOpts           *zap.Options
}

func privateEndpointForwarderCmd() *cobra.Command {
	var opts privateEndpointForwarderOpts
	c := &cobra.Command{
		Use: "private-endpoint-forwarder",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPrivateEndpointForwarder(opts)
		},
	}
	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to")
	c.Flags().StringVar(&opts.privateDialServer, "private-dial-server", "quic.connect-endpoint.ngrok.com:443",
		"Private dial gateway (host:port). QUIC is forced: privatedial's HTTP/2 transport panics on Go 1.27, so UDP/443 egress is required.")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)
	return c
}

func runPrivateEndpointForwarder(opts privateEndpointForwarderOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))
	log := ctrl.Log.WithName("private-endpoint-forwarder")
	buildInfo := version.Get()
	log.Info("starting private-endpoint-forwarder", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}
	token, ok := os.LookupEnv("NGROK_ACCESS_TOKEN")
	if !ok || token == "" {
		return errors.New("NGROK_ACCESS_TOKEN environment variable should be set, but was not")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{namespace: {}}},
		Metrics:                server.Options{BindAddress: opts.metricsAddr},
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		return fmt.Errorf("unable to start private-endpoint-forwarder: %w", err)
	}

	dialer := privatedial.New(privatedial.Config{
		QUICServerAddr: opts.privateDialServer,
		ForceProtocol:  privatedial.ProtocolQUIC,
		AuthToken:      token,
	})
	table := forwarder.NewTable()
	proxy := &forwarder.Proxy{
		Table:        table,
		Log:          log.WithName("proxy"),
		DrainTimeout: 5 * time.Second,
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
	}
	ports := &forwarder.PortListeners{Proxy: proxy, Log: log.WithName("ports")}
	defer ports.Close()

	if err := (&forwarder.Syncer{Client: mgr.GetClient(), Namespace: namespace, Table: table, Ports: ports}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PrivateEndpoint syncer: %w", err)
	}

	if err := mgr.AddReadyzCheck("routing-table", func(*http.Request) error {
		if !table.Synced() {
			return errors.New("routing table not synced")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", func(*http.Request) error { return nil }); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	log.Info("serving", "privateDialServer", opts.privateDialServer)
	return mgr.Start(ctrl.SetupSignalHandler())
}
