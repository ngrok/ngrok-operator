// Package privateendpoints reconciles PrivateEndpoint CRs from the ngrok API
// and wires them to in-cluster Services.
package privateendpoints

import (
	"context"
	"errors"
	"fmt"
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

// Poller mirrors the account's private endpoints into PrivateEndpoint CRs.
type Poller struct {
	client.Client
	Log       logr.Logger
	Namespace string
	Endpoints EndpointLister
	Interval  time.Duration
}

func (p *Poller) Start(ctx context.Context) error {
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
func (p *Poller) Sync(ctx context.Context) error {
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
		if _, ok := desired[cr.Name]; ok {
			delete(desired, cr.Name)
			continue
		}
		if err := p.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting PrivateEndpoint %s: %w", cr.Name, err))
		}
	}
	for name, spec := range desired {
		cr := &ngrokv1.PrivateEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: p.Namespace,
				Labels: map[string]string{
					pe.ManagedByLabel: pe.ManagedByValue,
					pe.HostLabel:      pe.HostKey(spec.Hostname),
				},
			},
			Spec: spec,
		}
		if err := p.Create(ctx, cr); err != nil && !apierrors.IsAlreadyExists(err) {
			errs = append(errs, fmt.Errorf("creating PrivateEndpoint for %s: %w", spec.URL, err))
		}
	}
	return errors.Join(errs...)
}

// desiredSpecs keeps private, non-kubernetes-bound endpoints, keyed by CR
// name. Pooled endpoints share a URL and collapse into one entry.
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
		out[pe.CRName(ep.URL)] = spec
	}
	return out
}
