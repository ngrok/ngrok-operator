package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-operator/internal/controller/controllers"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// baseControllerOp is an enum for the different operations that can be performed by a baseController
type baseControllerOp int

const (
	// createOp is the operation for creating a resource
	createOp baseControllerOp = iota

	// updateOp is the operation for updating a resource (upsert)
	updateOp

	// deleteOp is the operation for deleting a resource (and finalizers)
	deleteOp
)

// baseController is our standard pattern for writing controllers
type baseController[T client.Object] struct {
	// Kube is the base client for interacting with the Kubernetes API
	Kube client.Client

	// Log is the logger for the controller
	Log logr.Logger

	// Recorder is the event recorder for the controller
	Recorder record.EventRecorder

	// Namespace is optional for controllers
	Namespace *string

	statusID  func(ct T) string
	create    func(ctx context.Context, obj T) error
	update    func(ctx context.Context, obj T) error
	delete    func(ctx context.Context, obj T) error
	errResult func(op baseControllerOp, obj T, err error) (ctrl.Result, error)
}

// reconcile is the primary function that a manager calls for this controller to reconcile an event for the give client.Object
func (self *baseController[T]) reconcile(ctx context.Context, req ctrl.Request, obj T) (ctrl.Result, error) {
	objFullName := util.ObjToHumanGvkName(obj)
	objName := util.ObjToHumanName(obj)

	log := self.Log.WithValues("resource", objFullName)
	ctx = ctrl.LoggerInto(ctx, log)

	if err := self.Kube.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if controllers.IsUpsert(obj) {
		if err := controllers.RegisterAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
			return ctrl.Result{}, err
		}

		if self.statusID != nil && self.statusID(obj) == "" {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Creating", fmt.Sprintf("Creating %s", objName))
			if err := self.create(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "CreateError", fmt.Sprintf("Failed to Create %s: %s", objName, err.Error()))
				if self.errResult != nil {
					return self.errResult(createOp, obj, err)
				}
				return self.ctrlResultForErr(err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Created", fmt.Sprintf("Created %s", objName))
		} else {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updating", fmt.Sprintf("Updating %s", objName))
			if err := self.update(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "UpdateError", fmt.Sprintf("Failed to update %s: %s", objName, err.Error()))
				if self.errResult != nil {
					return self.errResult(updateOp, obj, err)
				}
				return self.ctrlResultForErr(err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated %s", objName))
		}
	} else {
		if controllers.HasFinalizer(obj) {
			if self.statusID != nil && self.statusID(obj) != "" {
				sid := self.statusID(obj)
				self.Recorder.Event(obj, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting %s", objName))
				if err := self.delete(ctx, obj); err != nil {
					if !ngrok.IsNotFound(err) {
						self.Recorder.Event(obj, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to delete %s: %s", objName, err.Error()))
						if self.errResult != nil {
							return self.errResult(deleteOp, obj, err)
						}
						return self.ctrlResultForErr(err)
					}
					log.Info(fmt.Sprintf("%s not found, assuming it was already deleted", objFullName), "ID", sid)
				}
				self.Recorder.Event(obj, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted %s", objName))
			}

			if err := controllers.RemoveAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// ctrlResultForErr is a helper function to convert an error into a ctrl.Result passing through ngrok error mappings
func (self *baseController[T]) ctrlResultForErr(err error) (ctrl.Result, error) {
	var nerr *ngrok.Error
	if errors.As(err, &nerr) {
		switch {
		case nerr.StatusCode >= 500:
			return ctrl.Result{}, err
		case nerr.StatusCode == http.StatusTooManyRequests:
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		default:
			// the rest are client errors, we don't retry by default
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, err
}
