package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Re-export drain types for convenience so consumers can use controller.DrainState
type DrainState = drain.State

var IsDraining = drain.IsDraining

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
	Recorder events.EventRecorder

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
		if err := util.RegisterAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
			return ctrl.Result{}, err
		}

		if self.StatusID != nil && self.StatusID(obj) == "" {
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Creating", "Create", fmt.Sprintf("Creating %s", objName))
			if err := self.Create(ctx, obj); err != nil {
				self.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "CreateError", "Create", fmt.Sprintf("Failed to Create %s: %s", objName, err.Error()))
				return self.handleErr(CreateOp, obj, err)
			}
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Created", "Create", fmt.Sprintf("Created %s", objName))
		} else {
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Updating", "Update", fmt.Sprintf("Updating %s", objName))
			if err := self.Update(ctx, obj); err != nil {
				self.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "UpdateError", "Update", fmt.Sprintf("Failed to update %s: %s", objName, err.Error()))
				return self.handleErr(UpdateOp, obj, err)
			}
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Updated", "Update", fmt.Sprintf("Updated %s", objName))
		}
	} else if util.HasFinalizer(obj) {
		if self.StatusID != nil && self.StatusID(obj) != "" {
			sid := self.StatusID(obj)
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Deleting", "Delete", fmt.Sprintf("Deleting %s", objName))
			if err := self.Delete(ctx, obj); err != nil {
				if !ngrok.IsNotFound(err) {
					self.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "DeleteError", "Delete", fmt.Sprintf("Failed to delete %s: %s", objName, err.Error()))
					return self.handleErr(DeleteOp, obj, err)
				}
				log.Info(fmt.Sprintf("%s not found, assuming it was already deleted", objFullName), "ID", sid)
			}
			self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Deleted", "Delete", fmt.Sprintf("Deleted %s", objName))
		}

		if err := util.RemoveAndSyncFinalizer(ctx, self.Kube, obj); err != nil {
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

// ReconcileStatus reconciles the status of an object, retrying on conflict.
//
// Status update conflicts are common because the object's resourceVersion can
// change between the initial Get() and this call (e.g., from the finalizer Patch
// earlier in the reconcile, or from an external spec mutation). On conflict, this
// method re-fetches the latest resourceVersion and retries.
//
// This is safe for controllers where BaseController is the sole status writer for
// the resource (AgentEndpoint, CloudEndpoint, Domain, IPPolicy, etc.). For
// resources with multiple concurrent status writers (BoundEndpoint, Gateway), the
// callers manage their own retry/conflict logic and should not use this method.
func (self *BaseController[T]) ReconcileStatus(ctx context.Context, obj T, origErr error) error {
	log := ctrl.LoggerFrom(ctx).WithValues("originalError", origErr)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := self.Kube.Status().Update(ctx, obj)
		if apierrors.IsConflict(err) {
			// Re-fetch the latest resourceVersion for the next attempt.
			// Status().Update() only writes the status subresource, so we only
			// need the current resourceVersion — the spec/metadata are irrelevant.
			latest := obj.DeepCopyObject().(T)
			if getErr := self.Kube.Get(ctx, client.ObjectKeyFromObject(obj), latest); getErr != nil {
				return getErr
			}
			obj.SetResourceVersion(latest.GetResourceVersion())
		}
		return err
	})

	if retryErr != nil {
		self.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "StatusError", "UpdateStatus", fmt.Sprintf("Failed to reconcile status: %s", retryErr.Error()))
		log.V(1).Error(retryErr, "Failed to update status")
		return StatusError{err: origErr, cause: retryErr}
	}

	self.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Status", "UpdateStatus", "Successfully reconciled status")
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

// StatusError wraps .Status().*() errors returned from k8s client.
// err is the original reconcile error (may be nil if reconcile succeeded but status update failed).
// cause is the status update error.
type StatusError struct {
	err   error
	cause error
}

func (e StatusError) Error() string {
	if e.err == nil {
		return e.cause.Error()
	}
	return fmt.Sprintf("%s: %s", e.cause, e.err)
}

func (e StatusError) Unwrap() error {
	return e.cause
}
