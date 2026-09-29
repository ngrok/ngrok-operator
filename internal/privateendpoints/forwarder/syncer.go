package forwarder

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch

// Syncer keeps one listener open per Ready PrivateEndpoint's forwarder port
// and forwards each connection to that endpoint's host:port.
type Syncer struct {
	client.Client
	Namespace string
	Table     *Table
	// Listeners opens and closes the per-port listeners. It predates this
	// feature (the bindings forwarder uses it too) but is generic.
	Listeners    *bindingsdriver.BindingsDriver
	Dial         DialFunc
	DrainTimeout time.Duration
	Log          logr.Logger

	mu   sync.Mutex
	open map[int32]bool
}

func (s *Syncer) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, s.Sync(ctx)
}

// Sync rebuilds the routing table from all PrivateEndpoints and opens or
// closes listeners to match. ctx is used for the dials of connections the
// listeners accept, so it must outlive this call.
func (s *Syncer) Sync(ctx context.Context) error {
	var list ngrokv1.PrivateEndpointList
	if err := s.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var entries []Entry
	want := map[int32]bool{}
	for _, cr := range list.Items {
		if cr.Status.ClusterIP == "" || cr.Status.ForwarderPort == 0 {
			continue // not wired up by the controller yet
		}
		entries = append(entries, Entry{Hostname: cr.Spec.Hostname, Port: cr.Spec.Port, ForwarderPort: cr.Status.ForwarderPort})
		want[cr.Status.ForwarderPort] = true
	}
	s.Table.Replace(entries)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open == nil {
		s.open = map[int32]bool{}
	}
	var errs []error
	for port := range want {
		if s.open[port] {
			continue
		}
		if err := s.Listeners.Listen(port, s.handler(ctx, port)); err != nil {
			errs = append(errs, fmt.Errorf("listening on forwarder port %d: %w", port, err))
			continue
		}
		s.open[port] = true
	}
	for port := range s.open {
		if !want[port] {
			s.Listeners.Close(port)
			delete(s.open, port)
		}
	}
	return errors.Join(errs...)
}

// handler resolves the target at accept time, so a port reassigned to a
// different endpoint routes to the new one without reopening the listener.
func (s *Syncer) handler(ctx context.Context, port int32) bindingsdriver.ConnectionHandler {
	return func(c net.Conn) error {
		target, ok := s.Table.PortTarget(port)
		if !ok {
			return c.Close()
		}
		err := ForwardConn(ctx, s.Dial, c, target, s.DrainTimeout)
		if err != nil {
			s.Log.Error(err, "forwarding connection", "port", port)
		}
		return err
	}
}

func (s *Syncer) Ports() []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int32
	for p := range s.open {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Close closes every listener.
func (s *Syncer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for port := range s.open {
		s.Listeners.Close(port)
		delete(s.open, port)
	}
}

// SetupWithManager syncs once after the cache fills (so the table goes
// Synced even with zero PrivateEndpoints), then on every change.
//
// Listeners opened from Reconcile use the ctx controller-runtime passes to
// reconcilers, which is the controller's run ctx, not a per-request one.
func (s *Syncer) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("PrivateEndpoint cache did not sync")
		}
		return s.Sync(ctx)
	})); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("private-endpoint-forwarder").
		For(&ngrokv1.PrivateEndpoint{}).
		Complete(s)
}
