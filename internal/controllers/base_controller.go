package controllers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type baseController[T client.Object] struct {
	Kube     client.Client
	Log      logr.Logger
	Recorder record.EventRecorder

	statusID func(ct T) string
	create   func(ctx context.Context, cr T) error
	update   func(ctx context.Context, cr T) error
	delete   func(ctx context.Context, cr T) error
}

func (r *baseController[T]) reconcile(ctx context.Context, req ctrl.Request, cr T) (ctrl.Result, error) {
	if err := r.Kube.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if isUpsert(cr) {
		if err := registerAndSyncFinalizer(ctx, r.Kube, cr); err != nil {
			return ctrl.Result{}, err
		}

		if r.statusID != nil && r.statusID(cr) == "" {
			if err := r.create(ctx, cr); err != nil {
				return reconcileResultFromError(err)
			}
		} else {
			if err := r.update(ctx, cr); err != nil {
				return reconcileResultFromError(err)
			}
		}
	} else {
		if hasFinalizer(cr) {
			if r.statusID == nil || r.statusID(cr) != "" {
				if err := r.delete(ctx, cr); err != nil {
					return reconcileResultFromError(err)
				}
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

	// TODO implement this
	return ctrl.Result{}, err
}
