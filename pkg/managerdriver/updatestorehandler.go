package managerdriver

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

var _ handler.EventHandler = &UpdateStoreHandler{}

// UpdateStoreHandler implements the controller-runtime eventhandler interface
// https://github.com/kubernetes-sigs/controller-runtime/blob/v0.14.1/pkg/handler/eventhandler.go
// This allows it to be used to handle each reconcile event for a watched resource type.
// This handler takes a basic object and updates/deletes the store with it.
// It is used to simply watch some resources and keep their values updated in the store.
// It is used to keep various crds like edges/tunnels/domains, and core resources like ingress classes, updated.
type UpdateStoreHandler struct {
	client client.Client
	driver *Driver
	store  Storer
	log    logr.Logger
}

// NewUpdateStoreHandler creates a new UpdateStoreHandler
func NewUpdateStoreHandler(resourceName string, d *Driver, client client.Client) *UpdateStoreHandler {
	return &UpdateStoreHandler{
		driver: d,
		client: client,
		store:  d.store,
		log:    d.log.WithValues("UpdateStoreHandlerFor", resourceName),
	}
}

// Create is called in response to an create event - e.g. Edge Creation.
func (e *UpdateStoreHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if err := e.store.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in create", "object", evt.Object)
		return
	}
}

// Update is called in response to an update event -  e.g. Edge Updated.
func (e *UpdateStoreHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if err := e.store.Update(evt.ObjectNew); err != nil {
		e.log.Error(err, "error updating object in update", "object", evt.ObjectNew)
		return
	}
	if err := e.driver.updateIngressStatuses(ctx, e.client); err != nil {
		e.log.Error(err, "error syncing after object update", "object", evt.ObjectNew)
		return
	}
}

// Delete is called in response to a delete event - e.g. Edge Deleted.
func (e *UpdateStoreHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if err := e.store.Delete(evt.Object); err != nil {
		e.log.Error(err, "error deleting object", "object", evt.Object)
		return
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request
func (e *UpdateStoreHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	if err := e.store.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in generic", "object", evt.Object)
		return
	}
}
