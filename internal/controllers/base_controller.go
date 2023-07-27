package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ngrokController[T client.Object] interface {
	client() client.Client

	getStatusID(cr T) string
	create(ctx context.Context, cr T) error
	update(ctx context.Context, cr T) error
	delete(ctx context.Context, cr T) error
}

func doReconcile[T client.Object](ctx context.Context, req ctrl.Request, cr T, d ngrokController[T]) (ctrl.Result, error) {
	if err := d.client().Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if isUpsert(cr) {
		if err := registerAndSyncFinalizer(ctx, d.client(), cr); err != nil {
			return ctrl.Result{}, err
		}

		if d.getStatusID(cr) == "" {
			if err := d.create(ctx, cr); err != nil {
				return reconcileResultFromError(err)
			}
		} else {
			if err := d.update(ctx, cr); err != nil {
				return reconcileResultFromError(err)
			}
		}
	} else {
		if hasFinalizer(cr) {
			if d.getStatusID(cr) != "" {
				if err := d.delete(ctx, cr); err != nil {
					return reconcileResultFromError(err)
				}
			}

			if err := removeAndSyncFinalizer(ctx, d.client(), cr); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func reconcileResultFromError(err error) (ctrl.Result, error) {
	// TODO implement this
	return ctrl.Result{}, err
}
