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
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.ngrok.com/ngrok/privatedial"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	privateendpointscontroller "github.com/ngrok/ngrok-operator/internal/controller/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/flags"
	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/internal/version"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

func init() {
	rootCmd.AddCommand(privateEndpointForwarderCmd())
}

type privateEndpointForwarderManagerOpts struct {
	flags.ManagerOptions

	log               *flags.LogOptions
	privateDialServer string
}

func privateEndpointForwarderCmd() *cobra.Command {
	var opts privateEndpointForwarderManagerOpts
	c := &cobra.Command{
		Use: "private-endpoint-forwarder-manager",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPrivateEndpointForwarder(opts)
		},
	}
	flags.Manager(c.Flags(), &opts.ManagerOptions, "private-endpoint-forwarder-manager")
	flags.PrivateEndpointsDialServer(c.Flags(), &opts.privateDialServer)
	opts.log = flags.Log(c.Flags())
	return c
}

func runPrivateEndpointForwarder(opts privateEndpointForwarderManagerOpts) error {
	logger, err := opts.log.Logger()
	if err != nil {
		return err
	}
	ctrl.SetLogger(logger)
	warnUnknownEnv()
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
		Metrics:                server.Options{BindAddress: opts.MetricsAddr},
		HealthProbeBindAddress: opts.ProbeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		return fmt.Errorf("unable to start private-endpoint-forwarder: %w", err)
	}

	dialer := privatedial.New(privatedial.Config{
		QUICServerAddr: opts.privateDialServer,
		// privatedial's HTTP/2 transport panics on Go 1.27, so force QUIC.
		ForceProtocol: privatedial.ProtocolQUIC,
		AuthToken:     token,
	})
	table := forwarder.NewTable()
	forwarderReconciler := &privateendpointscontroller.ForwarderReconciler{
		Client:       mgr.GetClient(),
		Namespace:    namespace,
		Table:        table,
		Listeners:    bindingsdriver.New(),
		DrainTimeout: 5 * time.Second,
		Log:          log.WithName("forward"),
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
	}
	defer forwarderReconciler.Close()
	if err := forwarderReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PrivateEndpoint forwarder controller: %w", err)
	}

	session := &dialSession{dialer: dialer, log: log.WithName("private-dial"), maxBackoff: 30 * time.Second}
	if err := mgr.Add(session); err != nil {
		return fmt.Errorf("setting up private dial session: %w", err)
	}

	if err := mgr.AddReadyzCheck("routing-table", forwarderReadyz(table, forwarderReconciler)); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("private-dial", session.readyz); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", func(*http.Request) error { return nil }); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	log.Info("serving", "privateDialServer", opts.privateDialServer)
	return mgr.Start(ctrl.SetupSignalHandler())
}

// forwarderReadyz is ready once the routing table has synced and every
// wanted listener is bound.
func forwarderReadyz(table *forwarder.Table, r *privateendpointscontroller.ForwarderReconciler) healthz.Checker {
	return func(*http.Request) error {
		if !table.Synced() {
			return errors.New("routing table not synced")
		}
		return r.BindErr()
	}
}

// sessionDialer is the part of *privatedial.Dialer that dialSession watches.
type sessionDialer interface {
	Connect(ctx context.Context) error
	Done() <-chan struct{}
	Err() error
}

// dialSession connects the dialer at startup and ends the process once the
// dialer is permanently done.
//
// Initial connect failures are not terminal in the SDK (even a 401 leaves the
// dialer lazily retrying), so they hold readiness and retry with backoff
// rather than exiting: a bad token leaves the pod running but never Ready.
// After the first connect the SDK's supervisor absorbs transient outages
// without closing Done, so readiness never flaps on them. Done closes only on
// a fatal session error such as an auth rejection on reconnect (revoked
// token); then Start returns an error, which stops the manager and exits
// non-zero so the kubelet restarts the container and re-reads the token.
type dialSession struct {
	dialer     sessionDialer
	log        logr.Logger
	maxBackoff time.Duration
	connected  atomic.Bool
}

func (s *dialSession) Start(ctx context.Context) error {
	backoff := min(time.Second, s.maxBackoff)
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.dialer.Connect(attemptCtx)
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return nil
		}
		s.log.Error(err, "connecting to private dial server, will retry", "backoff", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-s.dialer.Done():
			return s.terminal()
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, s.maxBackoff)
	}
	s.connected.Store(true)
	s.log.Info("connected to private dial server")

	select {
	case <-ctx.Done():
		return nil
	case <-s.dialer.Done():
		return s.terminal()
	}
}

func (s *dialSession) terminal() error {
	err := fmt.Errorf("private dial session ended permanently, exiting so the pod restarts: %w", s.dialer.Err())
	s.log.Error(err, "private dial session is done")
	return err
}

func (s *dialSession) readyz(*http.Request) error {
	if !s.connected.Load() {
		return errors.New("not connected to private dial server")
	}
	return s.dialer.Err()
}
