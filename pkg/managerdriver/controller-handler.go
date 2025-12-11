package managerdriver

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/store"
)

var _ handler.EventHandler = &ControllerEventHandler{}

// ControllerEventHandler implements the controller-runtime eventhandler interface
// https://github.com/kubernetes-sigs/controller-runtime/blob/v0.14.1/pkg/handler/eventhandler.go
// This allows it to be used to handle each reconcile event for a watched resource type.
// This handler takes a basic object and updates/deletes the store with it.
// It is used to simply watch some resources and keep their values updated in the store.
// It is used to keep various crds like edges/tunnels/domains, and core resources like ingress classes, updated.
type ControllerEventHandler struct {
	client   client.Client
	driver   *Driver
	store    store.Storer
	log      logr.Logger
	recorder record.EventRecorder
}

// ControllerEventHandlerOpt is a functional option for configuring ControllerEventHandler
type ControllerEventHandlerOpt func(*ControllerEventHandler)

// WithEventRecorder configures the handler to propagate error events from child resources
// (like Domain) up to parent resources (like Ingress/Gateway)
func WithEventRecorder(recorder record.EventRecorder) ControllerEventHandlerOpt {
	return func(h *ControllerEventHandler) {
		h.recorder = recorder
	}
}

// NewControllerEventHandler creates a new ControllerEventHandler
func NewControllerEventHandler(resourceName string, d *Driver, c client.Client, opts ...ControllerEventHandlerOpt) *ControllerEventHandler {
	h := &ControllerEventHandler{
		driver: d,
		client: c,
		store:  d.store,
		log:    d.log.WithValues("ControllerEventHandlerFor", resourceName),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Create is called in response to an create event - e.g. Edge Creation.
func (e *ControllerEventHandler) Create(_ context.Context, evt event.CreateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if err := e.store.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in create", "object", evt.Object)
		return
	}
}

// Update is called in response to an update event -  e.g. Edge Updated.
func (e *ControllerEventHandler) Update(ctx context.Context, evt event.UpdateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if err := e.store.Update(evt.ObjectNew); err != nil {
		e.log.Error(err, "error updating object in update", "object", evt.ObjectNew)
		return
	}
	if err := e.driver.updateStatuses(ctx, e.client); err != nil {
		e.log.Error(err, "error syncing after object update", "object", evt.ObjectNew)
		return
	}

	// Propagate errors from downstream resources to source Ingress/Gateway objects
	if e.recorder != nil {
		e.propagateErrorsToSources(evt.ObjectNew)
	}
}

// Delete is called in response to a delete event - e.g. Edge Deleted.
func (e *ControllerEventHandler) Delete(_ context.Context, evt event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if err := e.store.Delete(evt.Object); err != nil {
		e.log.Error(err, "error deleting object", "object", evt.Object)
		return
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request
func (e *ControllerEventHandler) Generic(_ context.Context, evt event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if err := e.store.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in generic", "object", evt.Object)
		return
	}
}

// propagateErrorsToSources checks if the updated object has error conditions and emits
// events on source Ingress/Gateway resources that created this object.
func (e *ControllerEventHandler) propagateErrorsToSources(obj client.Object) {
	domain, ok := obj.(*ingressv1alpha1.Domain)
	if !ok {
		// Future: handle other types like CloudEndpoint, AgentEndpoint
		return
	}

	readyCond := meta.FindStatusCondition(domain.Status.Conditions, "Ready")
	if readyCond == nil {
		return
	}

	// Find and emit events to source Ingresses
	for _, ing := range e.findIngressesForDomain(domain.Spec.Domain) {
		e.recorder.Eventf(ing, corev1.EventTypeWarning, readyCond.Reason,
			"Domain %q: %s", domain.Spec.Domain, readyCond.Message)
	}

	// Find and emit events to source Gateways
	for _, gw := range e.findGatewaysForDomain(domain.Spec.Domain) {
		e.recorder.Eventf(gw, corev1.EventTypeWarning, readyCond.Reason,
			"Domain %q: %s", domain.Spec.Domain, readyCond.Message)
	}
}

// findIngressesForDomain returns all Ingresses that reference the given domain name
func (e *ControllerEventHandler) findIngressesForDomain(domainName string) []*netv1.Ingress {
	var result []*netv1.Ingress
	for _, ing := range e.store.ListNgrokIngressesV1() {
		if ing == nil {
			continue
		}
		for _, rule := range ing.Spec.Rules {
			if rule.Host == domainName {
				result = append(result, ing)
				break
			}
		}
	}
	return result
}

// findGatewaysForDomain returns all Gateways that reference the given domain name
func (e *ControllerEventHandler) findGatewaysForDomain(domainName string) []*gatewayv1.Gateway {
	var result []*gatewayv1.Gateway
	for _, gw := range e.store.ListGateways() {
		if gw == nil {
			continue
		}
		for _, listener := range gw.Spec.Listeners {
			if listener.Hostname != nil && string(*listener.Hostname) == domainName {
				result = append(result, gw)
				break
			}
		}
	}
	return result
}
