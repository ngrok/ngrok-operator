package controllers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName = "k8s.ngrok.com/finalizer"
)

func IsUpsert(o client.Object) bool {
	return o.GetDeletionTimestamp().IsZero()
}

func IsDelete(o client.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, finalizerName)
}

func AddFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, finalizerName)
}

func RemoveFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, finalizerName)
}

func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if !HasFinalizer(o) {
		AddFinalizer(o)
		return c.Update(ctx, o)
	}
	return nil
}

func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	RemoveFinalizer(o)
	return c.Update(ctx, o)
}
