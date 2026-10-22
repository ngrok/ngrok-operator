// Package privateendpoints reconciles PrivateEndpoint CRs from the ngrok API
// and wires them to in-cluster Services.
package privateendpoints

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

type EndpointLister interface {
	List(*ngrok.Paging) ngrok.Iter[*ngrok.Endpoint]
}

// PrivateEndpointPoller mirrors the account's private endpoints into PrivateEndpoint CRs.
type PrivateEndpointPoller struct {
	client.Client
	Log       logr.Logger
	Namespace string
	Endpoints EndpointLister
	Interval  time.Duration
}

func (p *PrivateEndpointPoller) Start(ctx context.Context) error {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		if err := p.Sync(ctx); err != nil {
			p.Log.Error(err, "private endpoint sync failed, keeping current PrivateEndpoints")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Sync converges PrivateEndpoint CRs to the API's private endpoints. On a
// list error it changes nothing, so an API outage never deletes CRs.
func (p *PrivateEndpointPoller) Sync(ctx context.Context) error {
	var eps []*ngrok.Endpoint
	iter := p.Endpoints.List(nil)
	for iter.Next(ctx) {
		eps = append(eps, iter.Item())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("listing endpoints: %w", err)
	}
	desired := desiredSpecs(p.Log, eps)

	var existing ngrokv1.PrivateEndpointList
	if err := p.List(ctx, &existing, client.InNamespace(p.Namespace), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue}); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}

	var errs []error
	for i := range existing.Items {
		cr := &existing.Items[i]
		if spec, ok := desired[cr.Name]; ok {
			delete(desired, cr.Name)
			if err := p.heal(ctx, cr, spec); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := p.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting PrivateEndpoint %s: %w", cr.Name, err))
		}
	}
	for name, spec := range desired {
		cr := &ngrokv1.PrivateEndpoint{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: managedLabels(spec)},
			Spec:       spec,
		}
		err := p.Create(ctx, cr)
		switch {
		case apierrors.IsAlreadyExists(err):
			// Only labeled CRs are ours; a same-named CR without the label may
			// be hand-made, so don't adopt or overwrite it.
			p.Log.Info("PrivateEndpoint exists without managed-by label, leaving it alone", "name", name, "url", spec.URL)
		case err != nil:
			errs = append(errs, fmt.Errorf("creating PrivateEndpoint for %s: %w", spec.URL, err))
		}
	}
	return errors.Join(errs...)
}

// heal puts a managed CR's spec and poller-owned labels back to desired. Other
// labels and status are left alone.
func (p *PrivateEndpointPoller) heal(ctx context.Context, cr *ngrokv1.PrivateEndpoint, spec ngrokv1.PrivateEndpointSpec) error {
	want := managedLabels(spec)
	drifted := cr.Spec != spec
	for k, v := range want {
		if cr.Labels[k] != v {
			drifted = true
		}
	}
	if !drifted {
		return nil
	}
	p.Log.Info("restoring drifted PrivateEndpoint", "name", cr.Name, "url", spec.URL)
	cr.Spec = spec
	maps.Copy(cr.Labels, want)
	if err := p.Update(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("restoring PrivateEndpoint %s: %w", cr.Name, err)
	}
	return nil
}

func managedLabels(spec ngrokv1.PrivateEndpointSpec) map[string]string {
	return map[string]string{
		pe.ManagedByLabel: pe.ManagedByValue,
		pe.HostLabel:      pe.HostKey(spec.Hostname),
	}
}

// desiredSpecs keeps *.ngrok.direct, non-kubernetes-bound endpoints, keyed by
// CR name. Pooled endpoints share a URL and collapse into one entry.
func desiredSpecs(log logr.Logger, eps []*ngrok.Endpoint) map[string]ngrokv1.PrivateEndpointSpec {
	out := map[string]ngrokv1.PrivateEndpointSpec{}
	for _, ep := range eps {
		if slices.Contains(ep.Bindings, "kubernetes") {
			continue
		}
		spec, err := pe.ParseURL(ep.URL)
		if err != nil {
			log.V(1).Info("skipping endpoint", "id", ep.ID, "url", ep.URL, "reason", err.Error())
			continue
		}
		if !pe.IsPrivateHostname(spec.Hostname) {
			continue
		}
		out[pe.CRName(spec)] = spec
	}
	return out
}
