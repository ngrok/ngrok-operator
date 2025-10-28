package controller

import (
	"context"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FinalizerName = "k8s.ngrok.com/finalizer"
)

// IsUpsert returns true if the object is being created or updated.
// i.e. the DeletionTimestamp is zero.
func IsUpsert(o client.Object) bool {
	return o.GetDeletionTimestamp().IsZero()
}

// IsDelete returns true if the object is being deleted.
// i.e. the DeletionTimestamp is non-zero.
func IsDelete(o client.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

// HasFinalizer returns true if the object has our operator finalizer set.
func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, FinalizerName)
}

// AddFinalizer adds our operator finalizer to the object.
// If the finalizer was not already present, it returns true.
func AddFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, FinalizerName)
}

// RemoveFinalizer removes our operator finalizer from the object.
// If the finalizer was present and removed, it returns true.
func RemoveFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, FinalizerName)
}

// RegisterAndSyncFinalizer adds our finalizer to the object and updates it in the cluster if not already present.
func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if !HasFinalizer(o) {
		AddFinalizer(o)
		return c.Update(ctx, o)
	}
	return nil
}

// RemoveAndSyncFinalizer removes our finalizer from the object and updates it in the cluster.
func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	RemoveFinalizer(o)
	return c.Update(ctx, o)
}

// AddAnnotations adds the given annotations to the object.
func AddAnnotations(o client.Object, annotations map[string]string) {
	if o == nil || annotations == nil {
		return
	}

	existing := o.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}

	maps.Copy(existing, annotations)
	o.SetAnnotations(existing)
}
