package util

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// FinalizerName is the finalizer used by the ngrok operator
	FinalizerName = "k8s.ngrok.com/finalizer"
)

// HasFinalizer returns true if the object has the ngrok finalizer present.
func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, FinalizerName)
}

// AddFinalizer adds the ngrok finalizer to the object if not already present.
// Returns true if the finalizer was added.
func AddFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, FinalizerName)
}

// RemoveFinalizer removes the ngrok finalizer from the object if present.
// Returns true if the finalizer was removed.
func RemoveFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, FinalizerName)
}

// RegisterAndSyncFinalizer adds the ngrok finalizer to the object if not already present.
// If it adds the finalizer, it updates the object in the Kubernetes API.
func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if AddFinalizer(o) {
		return c.Update(ctx, o)
	}
	return nil
}

// RemoveAndSyncFinalizer removes the ngrok finalizer from the object if present.
// If it removes the finalizer, it updates the object in the Kubernetes API.
func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if RemoveFinalizer(o) {
		return c.Update(ctx, o)
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
