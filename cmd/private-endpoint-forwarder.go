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
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"
	"golang.ngrok.com/ngrok/privatedial"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	dnsAddr           string
	httpAddr          string
	httpsAddr         string
	dnsUpstream       string
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
	c.Flags().StringVar(&opts.dnsAddr, "dns-bind-address", ":5353", "UDP and TCP address the DNS server binds to")
	c.Flags().StringVar(&opts.httpAddr, "http-bind-address", ":8000", "Address of the shared listener for http endpoints on port 80")
	c.Flags().StringVar(&opts.httpsAddr, "https-bind-address", ":8443", "Address of the shared listener for https/tls endpoints on port 443")
	c.Flags().StringVar(&opts.dnsUpstream, "dns-upstream", "", "Resolver (host:port) for .internal names that aren't private endpoints. Defaults to the first nameserver in /etc/resolv.conf")
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
	upstream := opts.dnsUpstream
	if upstream == "" {
		rc, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			return fmt.Errorf("reading /etc/resolv.conf (set --dns-upstream): %w", err)
		}
		if len(rc.Servers) == 0 {
			return errors.New("no nameserver in /etc/resolv.conf (set --dns-upstream)")
		}
		upstream = net.JoinHostPort(rc.Servers[0], rc.Port)
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
		Table:       table,
		Log:         log.WithName("proxy"),
		PeekTimeout: 10 * time.Second,
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
	}
	ports := &forwarder.PortListeners{Proxy: proxy, Log: log.WithName("ports")}
	defer ports.Close()

	if err := (&forwarder.Syncer{Client: mgr.GetClient(), Namespace: namespace, Table: table, Ports: ports}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PrivateEndpoint syncer: %w", err)
	}

	dnsHandler := &forwarder.DNSHandler{Table: table, Upstream: upstream, TTL: 5, Exchange: forwarder.UDPExchange}
	for _, network := range []string{"udp", "tcp"} {
		srv := &dns.Server{Addr: opts.dnsAddr, Net: network, Handler: dnsHandler}
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			errCh := make(chan error, 1)
			go func() { errCh <- srv.ListenAndServe() }()
			select {
			case <-ctx.Done():
				return srv.ShutdownContext(context.Background())
			case err := <-errCh:
				return fmt.Errorf("dns %s server: %w", network, err)
			}
		})); err != nil {
			return err
		}
	}

	for _, l := range []struct {
		addr string
		port int32
		peek func(*bufio.Reader) (string, error)
	}{
		{opts.httpAddr, 80, forwarder.PeekHTTPHost},
		{opts.httpsAddr, 443, forwarder.PeekSNI},
	} {
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			ln, err := net.Listen("tcp", l.addr)
			if err != nil {
				return fmt.Errorf("listening on %s: %w", l.addr, err)
			}
			return proxy.ServeShared(ctx, ln, l.port, l.peek)
		})); err != nil {
			return err
		}
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

	log.Info("serving", "dns", opts.dnsAddr, "dnsUpstream", upstream, "http", opts.httpAddr, "https", opts.httpsAddr, "privateDialServer", opts.privateDialServer)
	return mgr.Start(ctrl.SetupSignalHandler())
}
