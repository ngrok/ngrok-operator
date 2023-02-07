package store

import (
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

var _ handler.EventHandler = &UpdateStoreHandler{}

type UpdateStoreHandler struct {
	driver *Driver
	log    logr.Logger
}

func NewUpdateStoreHandler(resourceName string, d *Driver) *UpdateStoreHandler {
	return &UpdateStoreHandler{
		driver: d,
		log:    d.log.WithValues("UpdateStoreHandlerFor", resourceName),
	}
}

func (e *UpdateStoreHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if err := e.driver.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in create", "object", evt.Object)
		return
	}
}

func (e *UpdateStoreHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if err := e.driver.Update(evt.ObjectNew); err != nil {
		e.log.Error(err, "error updating object in update", "object", evt.ObjectNew)
		return
	}
}

func (e *UpdateStoreHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if err := e.driver.Delete(evt.Object); err != nil {
		e.log.Error(err, "error deleting object", "object", evt.Object)
		return
	}
}

func (e *UpdateStoreHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	if err := e.driver.Update(evt.Object); err != nil {
		e.log.Error(err, "error updating object in generic", "object", evt.Object)
		return
	}
}
