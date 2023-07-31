package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type baseControllerOp int

const (
	createOp baseControllerOp = iota
	updateOp
	deleteOp
)

type baseController[T client.Object] struct {
	Kube     client.Client
	Log      logr.Logger
	Recorder record.EventRecorder

	kubeType  string
	statusID  func(ct T) string
	create    func(ctx context.Context, cr T) error
	update    func(ctx context.Context, cr T) error
	delete    func(ctx context.Context, cr T) error
	errResult func(op baseControllerOp, cr T, err error) (ctrl.Result, error)
}

func (r *baseController[T]) reconcile(ctx context.Context, req ctrl.Request, cr T) (ctrl.Result, error) {
	log := r.Log.WithValues(r.kubeType, req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	if err := r.Kube.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	crName := req.NamespacedName.String()
	if isUpsert(cr) {
		if err := registerAndSyncFinalizer(ctx, r.Kube, cr); err != nil {
			return ctrl.Result{}, err
		}

		if r.statusID != nil && r.statusID(cr) == "" {
			r.Recorder.Event(cr, v1.EventTypeNormal, "Creating", fmt.Sprintf("Creating %s: %s", r.kubeType, crName))
			if err := r.create(ctx, cr); err != nil {
				r.Recorder.Event(cr, v1.EventTypeWarning, "CreateError", fmt.Sprintf("Failed to create %s %s: %s", r.kubeType, crName, err.Error()))
				if r.errResult != nil {
					return r.errResult(createOp, cr, err)
				}
				return reconcileResultFromError(err)
			}
			r.Recorder.Event(cr, v1.EventTypeNormal, "Created", fmt.Sprintf("Created %s: %s", r.kubeType, crName))
		} else {
			r.Recorder.Event(cr, v1.EventTypeNormal, "Updating", fmt.Sprintf("Updating %s: %s", r.kubeType, crName))
			if err := r.update(ctx, cr); err != nil {
				r.Recorder.Event(cr, v1.EventTypeWarning, "UpdateError", fmt.Sprintf("Failed to update %s %s: %s", r.kubeType, crName, err.Error()))
				if r.errResult != nil {
					return r.errResult(updateOp, cr, err)
				}
				return reconcileResultFromError(err)
			}
			r.Recorder.Event(cr, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated %s: %s", r.kubeType, crName))
		}
	} else {
		if hasFinalizer(cr) {
			if r.statusID == nil || r.statusID(cr) != "" {
				sid := r.statusID(cr)
				r.Recorder.Event(cr, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting %s: %s", r.kubeType, crName))
				if err := r.delete(ctx, cr); err != nil {
					if !ngrok.IsNotFound(err) {
						r.Recorder.Event(cr, v1.EventTypeWarning, "DeleteError", fmt.Sprintf("Failed to delete %s %s: %s", r.kubeType, crName, err.Error()))
						if r.errResult != nil {
							return r.errResult(deleteOp, cr, err)
						}
						return reconcileResultFromError(err)
					}
					log.Info(fmt.Sprintf("%s not found, assuming it was already deleted", r.kubeType), "ID", sid)
				}
				r.Recorder.Event(cr, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted %s: %s", r.kubeType, crName))
			}

			if err := removeAndSyncFinalizer(ctx, r.Kube, cr); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func reconcileResultFromError(err error) (ctrl.Result, error) {
	var nerr *ngrok.Error
	if errors.As(err, &nerr) {
		switch {
		case nerr.StatusCode >= 500:
			return ctrl.Result{}, err
		case nerr.StatusCode == http.StatusTooManyRequests:
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		// case ngrok.IsErrorCode(err, retryCodes...):
		// 	return ctrl.Result{Requeue: true}, nil
		default:
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, err
}
