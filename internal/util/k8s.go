package util

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// FinalizerName is the finalizer used by the ngrok operator.
const FinalizerName = "ngrok.com/finalizer"

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyFinalizerName is the deprecated finalizer name. Retained for
// auto-upgrade: existing objects carrying the legacy finalizer have it
// silently swapped for FinalizerName on the next reconcile. Delete this
// constant and the legacy branches in HasFinalizer / AddFinalizer /
// RemoveFinalizer below in the release immediately before 1.0.
const LegacyFinalizerName = "k8s.ngrok.com/finalizer"

// LEGACY-PREFIX-MIGRATION: END

// HasFinalizer returns true if the object carries either the new or the
// legacy ngrok finalizer.
func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, FinalizerName) ||
		// LEGACY-PREFIX-MIGRATION: drop the LegacyFinalizerName check in 1.0
		controllerutil.ContainsFinalizer(o, LegacyFinalizerName)
}

// AddFinalizer ensures the new ngrok finalizer is present and removes the
// legacy one if it lingers. Returns true if either operation modified the
// object.
func AddFinalizer(o client.Object) bool {
	added := controllerutil.AddFinalizer(o, FinalizerName)
	// LEGACY-PREFIX-MIGRATION: drop the legacy removal + the `removed` |/`added` OR in 1.0
	removed := controllerutil.RemoveFinalizer(o, LegacyFinalizerName)
	return added || removed
}

// RemoveFinalizer removes both the new and legacy ngrok finalizers from the
// object if either is present.
func RemoveFinalizer(o client.Object) bool {
	removedNew := controllerutil.RemoveFinalizer(o, FinalizerName)
	// LEGACY-PREFIX-MIGRATION: drop the legacy removal + the `removedLegacy` OR in 1.0
	removedLegacy := controllerutil.RemoveFinalizer(o, LegacyFinalizerName)
	return removedNew || removedLegacy
}

// RegisterAndSyncFinalizer adds the ngrok finalizer to the object if not already present.
// If it adds the finalizer, it patches the object in the Kubernetes API.
// Uses Patch instead of Update to avoid resourceVersion conflicts by only sending the diff.
func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	patch := client.MergeFrom(o.DeepCopyObject().(client.Object))
	if AddFinalizer(o) {
		return c.Patch(ctx, o, patch)
	}
	return nil
}

// RemoveAndSyncFinalizer removes the ngrok finalizer from the object if present.
// If it removes the finalizer, it patches the object in the Kubernetes API.
// Uses Patch instead of Update to avoid resourceVersion conflicts by only sending the diff.
func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	patch := client.MergeFrom(o.DeepCopyObject().(client.Object))
	if RemoveFinalizer(o) {
		return c.Patch(ctx, o, patch)
	}
	return nil
}

// ToClientObjects converts a slice of objects whose pointer implements client.Object
// to a slice of client.Objects
func ToClientObjects[T any, PT interface {
	*T
	client.Object
}](s []T) []client.Object {
	objs := make([]client.Object, len(s))
	for i, obj := range s {
		var p PT = &obj
		objs[i] = p
	}
	return objs
}

// ObjectsToName converts a client.Object to its name
func ObjToName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return obj.GetName()
}

// ObjToKind converts a client.Object to its kind
func ObjToKind(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind
}

// ObjToGroupVersionKind converts a client.Object to its GVK
func ObjToGVK(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String()
}

// ObjToHumanName converts a client.Object to a human-readable name
func ObjToHumanName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind + "/" + obj.GetName()
}

// ObjToHumanGvkName converts a client.Object to a human-readable full name including GroupVersionKind
func ObjToHumanGvkName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String() + " Name=" + obj.GetName()
}
