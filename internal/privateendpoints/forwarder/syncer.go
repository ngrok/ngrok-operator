package forwarder

import (
	"context"
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch

// Syncer rebuilds the routing table and port listeners from all
// PrivateEndpoints on every change.
type Syncer struct {
	client.Client
	Namespace string
	Table     *Table
	Ports     *PortListeners
}

func (s *Syncer) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, s.Sync(ctx)
}

func (s *Syncer) Sync(ctx context.Context) error {
	var list ngrokv1.PrivateEndpointList
	if err := s.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var entries []Entry
	var ports []int32
	for _, cr := range list.Items {
		if cr.Status.ClusterIP == "" || cr.Status.ForwarderPort == 0 {
			continue // not wired up by the controller yet
		}
		entries = append(entries, Entry{
			Hostname:      cr.Spec.Hostname,
			Port:          cr.Spec.Port,
			ForwarderPort: cr.Status.ForwarderPort,
		})
		ports = append(ports, cr.Status.ForwarderPort)
	}
	s.Table.Replace(entries)
	return s.Ports.Sync(ctx, ports)
}

// SetupWithManager syncs once after the cache fills (so the table goes
// Synced even with zero PrivateEndpoints), then on every change.
//
// Listeners opened from Reconcile live on the ctx controller-runtime passes
// to reconcilers, which is the controller's run ctx, not a per-request one.
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
