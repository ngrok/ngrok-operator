package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName = "k8s.ngrok.com/finalizer"
)

func isDelete(meta metav1.ObjectMeta) bool {
	return meta.DeletionTimestamp != nil && !meta.DeletionTimestamp.IsZero()
}

func hasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, finalizerName)
}

func addFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, finalizerName)
}

func removeFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, finalizerName)
}

func registerAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if !hasFinalizer(o) {
		addFinalizer(o)
		return c.Update(ctx, o)
	}
	return nil
}
