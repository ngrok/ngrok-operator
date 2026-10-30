package privateendpoints

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
	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

// ForwarderReconciler keeps one listener open per Ready PrivateEndpoint's forwarder port
// and forwards each connection to that endpoint's host:port.
type ForwarderReconciler struct {
	client.Client
	Namespace string
	Table     *forwarder.Table
	// Listeners opens and closes the per-port listeners. It predates this
	// feature (the bindings forwarder uses it too) but is generic.
	Listeners    *bindingsdriver.BindingsDriver
	Dial         forwarder.DialFunc
	DrainTimeout time.Duration
	Log          logr.Logger

	mu        sync.Mutex
	open      map[int32]bool // ports with a listener
	bindErr   error          // last Sync's bind failures, for readiness
	seenDupes []int32        // last ambiguous ports, so changes are logged once
}

// SetupWithManager syncs once after the cache fills (so the table goes
// Synced even with zero PrivateEndpoints), then on every change.
// Reconcile returns bind failures so controller-runtime retries them.
//
// Listeners opened from Reconcile use the ctx controller-runtime passes to
// reconcilers, which is the controller's run ctx, not a per-request one.
func (r *ForwarderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("PrivateEndpoint cache did not sync")
		}
		// A bind failure must not stop the manager: it shows on readiness
		// and the controller retries it with backoff.
		if err := r.Sync(ctx); err != nil && r.BindErr() == nil {
			return err
		}
		return nil
	})); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("private-endpoint-forwarder").
		For(&ngrokv1.PrivateEndpoint{}).
		Complete(r)
}

func (r *ForwarderReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, r.Sync(ctx)
}

// Sync rebuilds the routing table from all PrivateEndpoints and opens or
// closes listeners to match. It always works from the full list rather than
// the changed object: whether a port is ambiguous or can be closed depends on
// every endpoint claiming it. The list comes from the cache, so this is cheap.
// ctx is used for the dials of connections the listeners accept, so it must
// outlive this call.
func (r *ForwarderReconciler) Sync(ctx context.Context) error {
	// Lock before listing: the startup sync can overlap Reconcile, and an
	// older snapshot must not overwrite newer routes or close their listeners.
	r.mu.Lock()
	defer r.mu.Unlock()

	var list ngrokv1.PrivateEndpointList
	if err := r.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var entries []forwarder.Entry
	want := map[int32]string{} // forwarder port -> hostname, for error reporting
	for _, cr := range list.Items {
		if cr.Status.ClusterIP == "" || cr.Status.ForwarderPort == 0 {
			continue // not wired up by the controller yet
		}
		entries = append(entries, forwarder.Entry{Hostname: cr.Spec.Hostname, Port: cr.Spec.Port, ForwarderPort: cr.Status.ForwarderPort})
		want[cr.Status.ForwarderPort] = cr.Spec.Hostname
	}
	dupes := r.Table.Replace(entries)

	if len(dupes) > 0 && !slices.Equal(dupes, r.seenDupes) {
		r.Log.Error(errors.New("forwarder port claimed by more than one PrivateEndpoint"), "refusing connections on ambiguous ports", "ports", dupes)
	}
	r.seenDupes = dupes
	if r.open == nil {
		r.open = map[int32]bool{}
	}
	var errs []error
	for port, hostname := range want {
		if r.open[port] {
			continue
		}
		if err := r.Listeners.Listen(port, r.handler(ctx, port)); err != nil {
			r.Log.Error(err, "failed to bind forwarder listener", "port", port, "hostname", hostname)
			errs = append(errs, fmt.Errorf("listening on forwarder port %d for %s: %w", port, hostname, err))
			continue
		}
		r.open[port] = true
	}
	for port := range r.open {
		if _, ok := want[port]; !ok {
			r.Listeners.Close(port)
			delete(r.open, port)
		}
	}
	r.bindErr = errors.Join(errs...)
	return r.bindErr
}

// BindErr reports the listeners the last Sync wanted but could not bind, or
// nil. It reflects only local state, never ngrok reachability, so it is safe
// to gate readiness on.
func (r *ForwarderReconciler) BindErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bindErr
}

// handler resolves the target at accept time, so a port reassigned to a
// different endpoint routes to the new one without reopening the listener.
func (r *ForwarderReconciler) handler(ctx context.Context, port int32) bindingsdriver.ConnectionHandler {
	return func(c net.Conn) error {
		target, ok := r.Table.PortTarget(port)
		if !ok {
			return c.Close()
		}
		err := forwarder.ForwardConn(ctx, r.Dial, c, target, r.DrainTimeout)
		if err != nil {
			r.Log.Error(err, "forwarding connection", "port", port)
		}
		return err
	}
}

func (r *ForwarderReconciler) Ports() []int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []int32
	for p := range r.open {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Close closes every listener.
func (r *ForwarderReconciler) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for port := range r.open {
		r.Listeners.Close(port)
		delete(r.open, port)
	}
}
