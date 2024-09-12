package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BaseControllerOp is an enum for the different operations that can be performed by a BaseController
type BaseControllerOp int

const (
	// createOp is the operation for creating a resource
	CreateOp BaseControllerOp = iota

	// updateOp is the operation for updating a resource (upsert)
	UpdateOp

	// deleteOp is the operation for deleting a resource (and finalizers)
	DeleteOp
)

// BaseController is our standard pattern for writing controllers
//
// Note: Non-provided methods are not called during reconcile
type BaseController[T client.Object] struct {
	// Kube is the base client for interacting with the Kubernetes API
	Kube client.Client

	// Log is the logger for the controller
	Log logr.Logger

	// Recorder is the event recorder for the controller
	Recorder record.EventRecorder

	// Namespace is optional for controllers
	Namespace *string

	StatusID  func(ct T) string
	Create    func(ctx context.Context, obj T) error
	Update    func(ctx context.Context, obj T) error
	Delete    func(ctx context.Context, obj T) error
	ErrResult func(op BaseControllerOp, obj T, err error) (ctrl.Result, error)
}

// reconcile is the primary function that a manager calls for this controller to reconcile an event for the give client.Object
func (self *BaseController[T]) Reconcile(ctx context.Context, req ctrl.Request, obj T) (ctrl.Result, error) {
	objFullName := util.ObjToHumanGvkName(obj)
	objName := util.ObjToHumanName(obj)

	log := self.Log.WithValues("resource", objFullName)
	ctx = ctrl.LoggerInto(ctx, log)

	if err := self.Kube.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if IsUpsert(obj) {
		if err := RegisterAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
			return ctrl.Result{}, err
		}

		if self.StatusID != nil && self.StatusID(obj) == "" {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Creating", fmt.Sprintf("Creating %s", objName))
			if err := self.Create(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "CreateError", fmt.Sprintf("Failed to Create %s: %s", objName, err.Error()))
				if self.ErrResult != nil {
					return self.ErrResult(CreateOp, obj, err)
				}
				return CtrlResultForErr(err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Created", fmt.Sprintf("Created %s", objName))
		} else {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updating", fmt.Sprintf("Updating %s", objName))
			if err := self.Update(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "UpdateError", fmt.Sprintf("Failed to update %s: %s", objName, err.Error()))
				if self.ErrResult != nil {
					return self.ErrResult(UpdateOp, obj, err)
				}
				return CtrlResultForErr(err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated %s", objName))
		}
	} else {
		if HasFinalizer(obj) {
			if self.StatusID != nil && self.StatusID(obj) != "" {
				sid := self.StatusID(obj)
				self.Recorder.Event(obj, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting %s", objName))
				if err := self.Delete(ctx, obj); err != nil {
					if !ngrok.IsNotFound(err) {
						self.Recorder.Event(obj, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to delete %s: %s", objName, err.Error()))
						if self.ErrResult != nil {
							return self.ErrResult(DeleteOp, obj, err)
						}
						return CtrlResultForErr(err)
					}
					log.Info(fmt.Sprintf("%s not found, assuming it was already deleted", objFullName), "ID", sid)
				}
				self.Recorder.Event(obj, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted %s", objName))
			}

			if err := RemoveAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// CtrlResultForErr is a helper function to convert an error into a ctrl.Result passing through ngrok error mappings
func CtrlResultForErr(err error) (ctrl.Result, error) {
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
