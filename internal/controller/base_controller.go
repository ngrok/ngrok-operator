package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/drainstate"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Re-export drainstate types for convenience so consumers can use controller.DrainState
type DrainState = drainstate.State

var IsDraining = drainstate.IsDraining

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

	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState DrainState

	StatusID  func(obj T) string
	Create    func(ctx context.Context, obj T) error
	Update    func(ctx context.Context, obj T) error
	Delete    func(ctx context.Context, obj T) error
	ErrResult func(op BaseControllerOp, obj T, err error) (ctrl.Result, error)
}

// reconcile is the primary function that a manager calls for this controller to reconcile an event for the give client.Object
func (self *BaseController[T]) Reconcile(ctx context.Context, req ctrl.Request, obj T) (ctrl.Result, error) {
	// fill in the obj
	if err := self.Kube.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// obj is filled in and can now be trusted

	objFullName := util.ObjToHumanGvkName(obj)
	objName := util.ObjToHumanName(obj)

	log := self.Log.WithValues("resource", objFullName)
	ctx = ctrl.LoggerInto(ctx, log)

	log.V(1).Info("Reconciling Resource", "ID", self.StatusID(obj))

	// Skip non-delete reconciles during drain to prevent adding new finalizers
	if IsDraining(ctx, self.DrainState) && !IsDelete(obj) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	if IsUpsert(obj) {
		if err := RegisterAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
			return ctrl.Result{}, err
		}

		if self.StatusID != nil && self.StatusID(obj) == "" {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Creating", fmt.Sprintf("Creating %s", objName))
			if err := self.Create(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "CreateError", fmt.Sprintf("Failed to Create %s: %s", objName, err.Error()))
				return self.handleErr(CreateOp, obj, err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Created", fmt.Sprintf("Created %s", objName))
		} else {
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updating", fmt.Sprintf("Updating %s", objName))
			if err := self.Update(ctx, obj); err != nil {
				self.Recorder.Event(obj, v1.EventTypeWarning, "UpdateError", fmt.Sprintf("Failed to update %s: %s", objName, err.Error()))
				return self.handleErr(UpdateOp, obj, err)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated %s", objName))
		}
	} else if HasFinalizer(obj) {
		if self.StatusID != nil && self.StatusID(obj) != "" {
			sid := self.StatusID(obj)
			self.Recorder.Event(obj, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting %s", objName))
			if err := self.Delete(ctx, obj); err != nil {
				if !ngrok.IsNotFound(err) {
					self.Recorder.Event(obj, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to delete %s: %s", objName, err.Error()))
					return self.handleErr(DeleteOp, obj, err)
				}
				log.Info(fmt.Sprintf("%s not found, assuming it was already deleted", objFullName), "ID", sid)
			}
			self.Recorder.Event(obj, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted %s", objName))
		}

		if err := RemoveAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// NewEnqueueRequestForMapFunc wraps a map function to be used as an event handler.
// It also takes care to make sure that the controllers logger is passed through to the map function, so
// that we can use our common pattern of getting the logger from the context.
func (self *BaseController[T]) NewEnqueueRequestForMapFunc(f func(ctx context.Context, obj client.Object) []reconcile.Request) handler.EventHandler {
	wrappedFunc := func(ctx context.Context, obj client.Object) []reconcile.Request {
		ctx = ctrl.LoggerInto(ctx, self.Log)
		return f(ctx, obj)
	}
	return handler.EnqueueRequestsFromMapFunc(wrappedFunc)
}

// handleErr is a helper function to handle errors in the controller. If an ErrResult function is not provided,
// it will use the default CtrlResultForErr function.
func (self *BaseController[T]) handleErr(op BaseControllerOp, obj T, err error) (ctrl.Result, error) {
	if self.ErrResult != nil {
		return self.ErrResult(op, obj, err)
	}
	return CtrlResultForErr(err)
}

// ReconcileStatus is a helper function to reconcile the status of an object and requeue on update errors
// Note: obj must be the latest resource version from k8s api
func (self *BaseController[T]) ReconcileStatus(ctx context.Context, obj T, origErr error) error {
	log := ctrl.LoggerFrom(ctx).WithValues("originalError", origErr)

	if err := self.Kube.Status().Update(ctx, obj); err != nil {
		self.Recorder.Event(obj, v1.EventTypeWarning, "StatusError", fmt.Sprintf("Failed to reconcile status: %s", err.Error()))
		log.V(1).Error(err, "Failed to update status")
		return StatusError{origErr, err}
	}

	self.Recorder.Event(obj, v1.EventTypeNormal, "Status", "Successfully reconciled status")
	log.V(1).Info("Successfully updated status")
	return origErr
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
		case nerr.StatusCode == http.StatusNotFound:
			return ctrl.Result{}, err
		case ngrok.IsErrorCode(nerr, ngrokapi.NgrokOpErrFailedToCreateCSR.Code):
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nerr
		case ngrok.IsErrorCode(nerr, ngrokapi.NgrokOpErrFailedToCreateUpstreamService.Code, ngrokapi.NgrokOpErrFailedToCreateTargetService.Code):
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nerr
		case ngrok.IsErrorCode(nerr, ngrokapi.NgrokOpErrEndpointDenied.Code):
			return ctrl.Result{}, nil // do not retry, endpoint poller will take care of this
		default:
			// the rest are client errors, we don't retry by default
			return ctrl.Result{}, nil
		}
	}

	// if error was because of status update, requeue for 10 seconds
	var serr StatusError
	if errors.As(err, &serr) {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, serr
	}

	return ctrl.Result{}, err
}

// StatusError wraps .Status().*() errors returned from k8s client
type StatusError struct {
	err   error
	cause error
}

func (e StatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.cause, e.err)
}

func (e StatusError) Unwrap() error {
	return e.cause
}
